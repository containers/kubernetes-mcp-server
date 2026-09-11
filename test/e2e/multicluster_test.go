//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type multiclusterState struct {
	keycloakURL  string
	hubContext   string
	spokeContext string
	dep          *serverDeployment
}

var multiclusterTS testState[multiclusterState]

func spokeContextName() string {
	return os.Getenv("E2E_SPOKE_CONTEXT")
}

// requireSpokeCluster checks that a spoke cluster context and server URL are
// available. Skips if not configured. Returns the spoke context name.
func requireSpokeCluster(t *testing.T) string {
	t.Helper()
	ctx := spokeContextName()
	if ctx == "" {
		t.Skip("E2E_SPOKE_CONTEXT not set — no spoke cluster available for multicluster tests")
	}
	if os.Getenv("E2E_SPOKE_SERVER_URL") == "" {
		t.Skip("E2E_SPOKE_SERVER_URL not set — spoke cluster URL required for multicluster tests")
	}
	return ctx
}

// hubContextName returns the hub (default) context from the kubeconfig.
func hubContextName(t *testing.T, kubeconfigPath string) string {
	t.Helper()
	cfg, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err, "load kubeconfig")
	require.NotEmpty(t, cfg.CurrentContext, "kubeconfig has no current-context")
	return cfg.CurrentContext
}

// copyKubeconfigSecret creates a Secret containing a pod-friendly kubeconfig.
// Minikube kubeconfigs use localhost URLs (port-forwarded) that aren't reachable
// from inside a pod. This rewrites:
//   - Hub server URL → https://kubernetes.default.svc (in-cluster, always works)
//   - Spoke server URL → E2E_SPOKE_SERVER_URL (the spoke's container IP on the
//     shared network, with TLS verification disabled since the API server cert
//     doesn't include the container IP in its SANs)
func copyKubeconfigSecret(kubeconfigPath, hubContext, spokeContext string) func(ctx context.Context, t *testing.T, clientset kubernetes.Interface, namespace string) {
	return func(ctx context.Context, t *testing.T, clientset kubernetes.Interface, namespace string) {
		t.Helper()
		cfg, err := clientcmd.LoadFromFile(kubeconfigPath)
		require.NoError(t, err, "load kubeconfig %s", kubeconfigPath)

		if ctxEntry, ok := cfg.Contexts[hubContext]; ok {
			if cluster, ok := cfg.Clusters[ctxEntry.Cluster]; ok {
				cluster.Server = "https://kubernetes.default.svc"
			}
		}

		if spokeURL := os.Getenv("E2E_SPOKE_SERVER_URL"); spokeURL != "" {
			if ctxEntry, ok := cfg.Contexts[spokeContext]; ok {
				if cluster, ok := cfg.Clusters[ctxEntry.Cluster]; ok {
					cluster.Server = spokeURL
					cluster.InsecureSkipTLSVerify = true
					cluster.CertificateAuthority = ""
					cluster.CertificateAuthorityData = nil
				}
			}
		}

		data, err := clientcmd.Write(*cfg)
		require.NoError(t, err, "serialize pod-friendly kubeconfig")

		_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "multicluster-kubeconfig"},
			Data:       map[string][]byte{"kubeconfig": data},
		}, metav1.CreateOptions{})
		require.NoError(t, err, "create kubeconfig secret")
	}
}

func kubeconfigVolumeValues() map[string]any {
	return map[string]any{
		"extraVolumes": []map[string]any{
			{
				"name":   "multicluster-kubeconfig",
				"secret": map[string]any{"secretName": "multicluster-kubeconfig"},
			},
		},
		"extraVolumeMounts": []map[string]any{
			{
				"name":      "multicluster-kubeconfig",
				"mountPath": "/etc/multicluster",
				"readOnly":  true,
			},
		},
	}
}

func multiclusterServerConfig(spokeContext string) string {
	return fmt.Sprintf(`
		require_oauth = true
		oauth_audience = "mcp-server"
		oauth_scopes = ["openid", "mcp-server"]
		validate_token = false
		authorization_url = "%s"
		cluster_provider_strategy = "kubeconfig"
		cluster_auth_mode = "passthrough"
		kubeconfig = "/etc/multicluster/kubeconfig"
		certificate_authority = "%s/ca.crt"
		denied_resources = []

		[token_exchange]
		strategy = "keycloak-v2"
		audience = "openshift"
		scopes = ["mcp:openshift"]

		[token_exchange.client_auth]
		method = "client_secret_basic"
		client_id = "mcp-server"
		client_secret = "mcp-server-dev-secret"

		[cluster_provider_configs.kubeconfig]
		strategy = "keycloak-v2"

		[cluster_provider_configs.kubeconfig.targets.%s]
		audience = "spoke"
		scopes = ["mcp:spoke"]
	`, keycloakIssuerURL(), caMountPath, spokeContext)
}

// TestMulticlusterTokenExchange verifies that the MCP server can exchange tokens
// with different audiences per kubeconfig context. The hub context uses the
// global token exchange config (audience=openshift), while the spoke context
// uses a per-target override (audience=spoke).
func TestMulticlusterTokenExchange(t *testing.T) {
	f := features.New("multicluster-token-exchange").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			spokeCtx := requireSpokeCluster(t)
			keycloakURL := requireKeycloak(ctx, t, cfg)
			hubCtx := hubContextName(t, cfg.KubeconfigFile())

			dep := deployServer(ctx, t, cfg, "multicluster",
				withConfig(multiclusterServerConfig(spokeCtx)),
				withValues(mergeValues(
					viewClusterRoleBindingValues(),
					keycloakCAVolumeValues(),
					kubeconfigVolumeValues(),
				)),
				withPreInstall(func(ctx context.Context, t *testing.T, clientset kubernetes.Interface, namespace string) {
					copyKeycloakCASecret(ctx, t, clientset, namespace)
					copyKubeconfigSecret(cfg.KubeconfigFile(), hubCtx, spokeCtx)(ctx, t, clientset, namespace)
				}),
			)

			return multiclusterTS.set(ctx, &multiclusterState{
				keycloakURL:  keycloakURL,
				hubContext:   hubCtx,
				spokeContext: spokeCtx,
				dep:          dep,
			})
		}).
		Assess("context_list shows both hub and spoke clusters", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := multiclusterTS.get(ctx)
			token := mcpUserToken(t, s.keycloakURL, "openid", "mcp-server")

			mcpClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(s.dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + token}),
			)
			t.Cleanup(mcpClient.Close)

			output := requireToolCallSuccess(t, mcpClient, "configuration_contexts_list", map[string]any{})
			require.Contains(t, output, s.hubContext, "context_list should include the hub context")
			require.Contains(t, output, s.spokeContext, "context_list should include the spoke context")

			return ctx
		}).
		Assess("tool call against hub context succeeds with global STS exchange", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := multiclusterTS.get(ctx)
			token := mcpUserToken(t, s.keycloakURL, "openid", "mcp-server")

			mcpClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(s.dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + token}),
			)
			t.Cleanup(mcpClient.Close)

			output := requireToolCallSuccess(t, mcpClient, "namespaces_list", map[string]any{
				"context": s.hubContext,
			})
			require.Contains(t, output, s.dep.namespace,
				"hub namespaces_list should include the server's own namespace")

			return ctx
		}).
		Assess("tool call against spoke context succeeds with per-target exchange", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := multiclusterTS.get(ctx)
			token := mcpUserToken(t, s.keycloakURL, "openid", "mcp-server")

			mcpClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(s.dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + token}),
			)
			t.Cleanup(mcpClient.Close)

			output := requireToolCallSuccess(t, mcpClient, "namespaces_list", map[string]any{
				"context": s.spokeContext,
			})
			require.Contains(t, output, "kube-system",
				"spoke namespaces_list should include kube-system")

			return ctx
		}).
		Assess("RBAC is enforced on spoke — viewer cannot list secrets", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := multiclusterTS.get(ctx)

			viewerToken := mcpViewerToken(t, s.keycloakURL, "openid", "mcp-server")
			viewerClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(s.dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + viewerToken}),
			)
			t.Cleanup(viewerClient.Close)

			requireToolCallSuccess(t, viewerClient, "namespaces_list", map[string]any{
				"context": s.spokeContext,
			})
			requireToolCallError(t, viewerClient, "resources_list", map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"namespace":  "default",
				"context":    s.spokeContext,
			})

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
