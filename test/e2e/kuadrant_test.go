//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const (
	// toolPrefix is the prefix of the registration with userSpecificList enabled
	// (tools fetched per user; the gateway does not federate its prompts).
	toolPrefix = "k8s_"
	// promptPrefix is the prefix of the registration with userSpecificList
	// disabled, whose tools and prompts the gateway discovers and federates.
	promptPrefix = "k8sp_"
)

var mcpServerRegistrationGVR = schema.GroupVersionResource{
	Group:    "mcp.kuadrant.io",
	Version:  "v1alpha1",
	Resource: "mcpserverregistrations",
}

var httpRouteGVR = schema.GroupVersionResource{
	Group:    "gateway.networking.k8s.io",
	Version:  "v1",
	Resource: "httproutes",
}

type kuadrantState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var kuadrantTS testState[kuadrantState]

// TestKuadrantGateway deploys the server behind the Kuadrant MCP Gateway and
// drives tools and prompts through the gateway.
//
// Two registrations point at the same deployment:
//   - toolPrefix: userSpecificList=Enabled, as a server that enforces per-user
//     auth would be registered. The gateway fetches its tools per request but
//     (as of MCP_GATEWAY_VERSION in build/kuadrant.mk) skips prompt discovery
//     for such registrations entirely, so it never exposes prompts.
//   - promptPrefix: userSpecificList=Disabled, so the broker discovers both
//     tools and prompts and federates them under the prefix.
func TestKuadrantGateway(t *testing.T) {
	gatewayNS := envOrDefault("GATEWAY_NAMESPACE", "gateway-system")
	gatewaySvc := envOrDefault("GATEWAY_SERVICE", "mcp-gateway-istio")
	gatewayHost := envOrDefault("GATEWAY_HOST", "mcp.gateway.local")

	f := features.New("kuadrant-gateway").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			kubeconfig := cfg.KubeconfigFile()
			clientset, err := clientsetFromKubeconfig(kubeconfig)
			require.NoError(t, err)

			restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
			require.NoError(t, err)

			// Check if gateway service exists; skip if not.
			_, err = clientset.CoreV1().Services(gatewayNS).Get(ctx, gatewaySvc, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				t.Skipf("Kuadrant MCP Gateway service %s/%s not found — install with 'make kuadrant-setup'", gatewayNS, gatewaySvc)
			}
			require.NoError(t, err, "check gateway service")

			// 1. Deploy the MCP server with read-only config, configured prompts, RBAC, and HTTPRoute.
			dep := deployServer(ctx, t, cfg, "kuadrant-gw", withConfig(`
				read_only = true
			`+configuredPromptsTOML), withValues(mergeValues(clusterReaderRBACValues(), map[string]any{
				"httpRoute": map[string]any{
					"enabled": true,
					"parentRefs": []map[string]any{
						{
							"name":      "mcp-gateway",
							"namespace": gatewayNS,
						},
					},
					"hostnames": []string{"{{ .Release.Name }}.mcp.local"},
					"rules": []map[string]any{
						{
							"matches": []map[string]any{
								{"path": map[string]any{"type": "PathPrefix", "value": "/"}},
							},
						},
					},
				},
			})))

			dynClient, err := dynamic.NewForConfig(restCfg)
			require.NoError(t, err, "create dynamic client")

			// 2. Register the chart's HTTPRoute with userSpecificList enabled.
			registerWithGateway(ctx, t, dynClient, dep.namespace, dep.name, dep.name+"-reg", toolPrefix, "Enabled")

			// 3. Add a second HTTPRoute to the same Service and register it with
			//    userSpecificList disabled so the gateway federates prompts.
			promptRoute := dep.name + "-prompts"
			createGatewayHTTPRoute(ctx, t, dynClient, dep.namespace, promptRoute, gatewayNS, promptRoute+".mcp.local", dep.name)
			registerWithGateway(ctx, t, dynClient, dep.namespace, promptRoute, promptRoute+"-reg", promptPrefix, "Disabled")

			// 4. Port-forward to the gateway and connect MCP.
			gatewayURL, stopGW := portForwardService(ctx, t, restCfg, clientset, gatewayNS, gatewaySvc, serverPort)
			t.Cleanup(stopGW)

			mcpClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(gatewayURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Host": gatewayHost}),
			)
			t.Cleanup(mcpClient.Close)

			return kuadrantTS.set(ctx, &kuadrantState{dep: dep, mcpClient: mcpClient})
		}).
		Assess("tools accessible through Kuadrant MCP Gateway", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := kuadrantTS.get(ctx)
			clientset, err := clientsetFromKubeconfig(cfg.KubeconfigFile())
			require.NoError(t, err)

			// Verify tools with prefix.
			result, err := s.mcpClient.ListTools()
			require.NoError(t, err, "list tools through gateway")

			names := toolNames(result.Tools)
			var prefixed []string
			for _, tool := range result.Tools {
				if strings.HasPrefix(tool.Name, toolPrefix) {
					prefixed = append(prefixed, tool.Name)
				}
			}
			require.Greater(t, len(prefixed), 0,
				"expected tools with prefix %q, got: %v", toolPrefix, names)

			// Call namespaces_list through the gateway and verify.
			namespacesTool := toolPrefix + "namespaces_list"
			require.Contains(t, names, namespacesTool)

			toolOutput := requireToolCallSuccess(t, s.mcpClient, namespacesTool, map[string]any{})

			// Cross-reference against direct K8s API.
			nsList, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			require.NoError(t, err, "list namespaces via K8s API")
			for _, ns := range nsList.Items {
				require.Contains(t, toolOutput, ns.Name,
					"namespace %q not found in tool output", ns.Name)
			}

			return ctx
		}).
		Assess("prompts accessible through Kuadrant MCP Gateway", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := kuadrantTS.get(ctx)

			// Prompts are federated under the registration prefix. The broker
			// discovers them in its own loop after the registration reports Ready,
			// so allow a short grace period.
			requirePromptsEventually(t, s.mcpClient, 60*time.Second,
				promptPrefix+clusterHealthCheckPrompt,
				promptPrefix+mcpVersionPrompt,
				promptPrefix+checkPodLogsPrompt,
			)

			// The pinned gateway drops the prompt title in transit (its MCP
			// library predates the field), so titles are not asserted here.
			t.Run("configured prompts", func(t *testing.T) {
				assertConfiguredPrompts(t, s.mcpClient, promptPrefix, false)
			})

			t.Run("built-in cluster-health-check", func(t *testing.T) {
				assertClusterHealthCheck(t, s.mcpClient, promptPrefix, false, s.dep.namespace)
			})

			t.Run("gateway returns the same content as the direct path", func(t *testing.T) {
				direct := test.NewMcpClient(t, nil, test.WithEndpoint(s.dep.serverURL+"/mcp"))
				t.Cleanup(direct.Close)

				args := map[string]string{"name": "my-pod", "namespace": "my-ns"}
				viaGateway := requirePromptText(t, s.mcpClient, promptPrefix+checkPodLogsPrompt, args)
				viaDirect := requirePromptText(t, direct, checkPodLogsPrompt, args)
				require.Equal(t, viaDirect, viaGateway)
			})

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

// registerWithGateway creates an MCPServerRegistration targeting the named
// HTTPRoute, registers its cleanup, and waits for it to become Ready.
func registerWithGateway(ctx context.Context, t *testing.T, dynClient dynamic.Interface, namespace, routeName, regName, prefix, userSpecificList string) {
	t.Helper()
	reg := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "mcp.kuadrant.io/v1alpha1",
			"kind":       "MCPServerRegistration",
			"metadata": map[string]any{
				"name":      regName,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"prefix":           prefix,
				"userSpecificList": userSpecificList,
				"targetRef": map[string]any{
					"group": "gateway.networking.k8s.io",
					"kind":  "HTTPRoute",
					"name":  routeName,
				},
			},
		},
	}
	_, err := dynClient.Resource(mcpServerRegistrationGVR).Namespace(namespace).Create(ctx, reg, metav1.CreateOptions{})
	require.NoError(t, err, "create MCPServerRegistration %s", regName)

	t.Cleanup(func() {
		_ = dynClient.Resource(mcpServerRegistrationGVR).Namespace(namespace).Delete(
			context.Background(), regName, metav1.DeleteOptions{},
		)
	})

	waitForRegistrationReady(ctx, t, dynClient, namespace, regName, 120*time.Second)
}

// createGatewayHTTPRoute creates an HTTPRoute attached to the mcp-gateway
// Gateway that routes `hostname` to the named Service on serverPort, mirroring
// the chart's HTTPRoute template, and registers its cleanup.
func createGatewayHTTPRoute(ctx context.Context, t *testing.T, dynClient dynamic.Interface, namespace, name, gatewayNS, hostname, service string) {
	t.Helper()
	route := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"parentRefs": []any{
					map[string]any{"name": "mcp-gateway", "namespace": gatewayNS},
				},
				"hostnames": []any{hostname},
				"rules": []any{
					map[string]any{
						"matches": []any{
							map[string]any{"path": map[string]any{"type": "PathPrefix", "value": "/"}},
						},
						"backendRefs": []any{
							map[string]any{
								"group":  "",
								"kind":   "Service",
								"name":   service,
								"port":   int64(serverPort),
								"weight": int64(1),
							},
						},
					},
				},
			},
		},
	}
	_, err := dynClient.Resource(httpRouteGVR).Namespace(namespace).Create(ctx, route, metav1.CreateOptions{})
	require.NoError(t, err, "create HTTPRoute %s", name)

	t.Cleanup(func() {
		_ = dynClient.Resource(httpRouteGVR).Namespace(namespace).Delete(
			context.Background(), name, metav1.DeleteOptions{},
		)
	})
}

// requirePromptsEventually polls prompts/list until every named prompt is
// listed or the timeout elapses.
func requirePromptsEventually(t *testing.T, mcpClient *test.McpClient, timeout time.Duration, names ...string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var listed []string
	var lastErr error
	for {
		result, err := mcpClient.ListPrompts()
		if err != nil {
			// The broker may briefly reject requests while it reloads its
			// federated catalogue; treat errors as retryable until the deadline.
			lastErr = err
		} else {
			lastErr = nil
			listed = promptNames(result.Prompts)
			missing := false
			for _, name := range names {
				if findPrompt(result.Prompts, name) == nil {
					missing = true
					break
				}
			}
			if !missing {
				return
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(2 * time.Second)
	}
	require.Fail(t, fmt.Sprintf("prompts %v not listed within %v; last listing: %v; last error: %v", names, timeout, listed, lastErr))
}

func waitForRegistrationReady(ctx context.Context, t *testing.T, dynClient dynamic.Interface, namespace, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		obj, err := dynClient.Resource(mcpServerRegistrationGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			lastErr = err
			time.Sleep(2 * time.Second)
			continue
		}

		conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
		if err != nil {
			require.Fail(t, fmt.Sprintf("MCPServerRegistration %s/%s has invalid status.conditions: %v", namespace, name, err))
		}
		if found {
			for _, c := range conditions {
				cond, ok := c.(map[string]any)
				if !ok {
					continue
				}
				if cond["type"] == "Ready" && cond["status"] == "True" {
					return
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	if lastErr != nil {
		require.NoError(t, lastErr, "MCPServerRegistration %s/%s: last API error before timeout", namespace, name)
	}
	require.Fail(t, fmt.Sprintf("MCPServerRegistration %s/%s not ready within %v", namespace, name, timeout))
}
