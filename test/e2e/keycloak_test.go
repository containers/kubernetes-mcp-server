//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const keycloakRealm = "openshift"

type keycloakCtxKey struct{}

type keycloakState struct {
	localURL string
}

func TestKeycloakOIDC(t *testing.T) {
	f := features.New("keycloak-oidc").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			clientset, err := clientsetFromKubeconfig(cfg.KubeconfigFile())
			require.NoError(t, err)

			_, err = clientset.CoreV1().Services("keycloak").Get(ctx, "keycloak", metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				t.Skip("Keycloak not installed, skipping OIDC tests (run 'make keycloak-install' first)")
			}
			require.NoError(t, err, "check keycloak service")

			restCfg, err := clientcmd.BuildConfigFromFlags("", cfg.KubeconfigFile())
			require.NoError(t, err, "build rest config")

			localURL, stopPF := portForwardService(ctx, t, restCfg, clientset, "keycloak", "keycloak", 8080)
			t.Cleanup(stopPF)

			return context.WithValue(ctx, keycloakCtxKey{}, &keycloakState{localURL: localURL})
		}).
		Assess("OIDC discovery returns correct issuer URL", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := ctx.Value(keycloakCtxKey{}).(*keycloakState)

			d := discoverOIDC(t, s.localURL, keycloakRealm)
			require.Contains(t, d.Issuer, "keycloak.keycloak.svc",
				"issuer must use internal service DNS to match API server --oidc-issuer-url")
			require.Contains(t, d.Issuer, keycloakRealm)
			require.NotEmpty(t, d.TokenEndpoint)
			require.NotEmpty(t, d.AuthorizationEndpoint)

			return ctx
		}).
		Assess("token endpoint issues valid tokens", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := ctx.Value(keycloakCtxKey{}).(*keycloakState)

			tr := requestToken(t, tokenRequest{
				baseURL:  s.localURL,
				realm:    keycloakRealm,
				clientID: "mcp-client",
				username: "mcp",
				password: "mcp",
				scopes:   []string{"openid"},
			})
			require.NotEmpty(t, tr.AccessToken, "expected non-empty access token")
			require.True(t, strings.EqualFold(tr.TokenType, "bearer"), "expected bearer token type")
			require.Greater(t, tr.ExpiresIn, 0, "expected positive token expiry")

			return ctx
		}).
		Assess("OIDC-authenticated MCP tool call succeeds", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := ctx.Value(keycloakCtxKey{}).(*keycloakState)

			// 1. Get a user token from Keycloak (mcp-client, public client).
			token := fetchToken(t, tokenRequest{
				baseURL:  s.localURL,
				realm:    keycloakRealm,
				clientID: "mcp-client",
				username: "mcp",
				password: "mcp",
				scopes:   []string{"openid", "mcp-server"},
			})

			// 2. Deploy the MCP server with OIDC config and the cert-manager CA
			//    mounted so it can trust Keycloak's TLS. The CA secret is created
			//    by the copyKeycloakCASecret pre-install hook before Helm install.
			configTOML := `
				require_oauth = true
				oauth_audience = "mcp-server"
				oauth_scopes = ["openid", "mcp-server"]
				validate_token = false
				authorization_url = "https://keycloak.keycloak.svc:8443/realms/openshift"
				sts_client_id = "mcp-server"
				sts_client_secret = "mcp-server-dev-secret"
				sts_audience = "openshift"
				sts_scopes = ["mcp:openshift"]
				certificate_authority = "/etc/keycloak-ca/ca.crt"
			`
			dep := deployServer(ctx, t, cfg, "keycloak-oidc",
				withConfig(configTOML),
				withValues(mergeValues(viewClusterRoleBindingValues(), keycloakCAVolumeValues())),
				withPreInstall(copyKeycloakCASecret),
			)

			// 3. Sanity-check the negative path: with require_oauth enabled, an
			//    unauthenticated request must be rejected with 401 missing_token.
			requireUnauthorized(t, dep.serverURL+"/mcp", "", "missing_token")

			// 4. Connect to MCP with the OAuth token and call a tool.
			mcpClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{
					"Authorization": "Bearer " + token,
				}),
			)
			t.Cleanup(mcpClient.Close)

			toolResult, err := mcpClient.ListTools()
			require.NoError(t, err, "list tools through OIDC-authenticated MCP server")
			require.Greater(t, len(toolResult.Tools), 0, "expected at least one tool")
			require.Contains(t, toolNames(toolResult.Tools), "namespaces_list")

			// 5. Actually call a tool — this exercises the full chain:
			//    user token → MCP server → STS exchange → kube-apiserver OIDC auth.
			callResult, err := mcpClient.CallTool("namespaces_list", map[string]any{})
			require.NoError(t, err, "call namespaces_list through OIDC")
			require.False(t, callResult.IsError, "namespaces_list returned error: %s", textContent(callResult))
			output := textContent(callResult)
			require.Contains(t, output, dep.namespace,
				"OIDC-authenticated namespaces_list should include the server's own namespace")

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
