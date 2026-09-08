//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type keycloakState struct {
	localURL string
}

var keycloakTS testState[keycloakState]

func TestKeycloakOIDC(t *testing.T) {
	f := features.New("keycloak-oidc").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			keycloakURL := requireKeycloak(ctx, t, cfg)
			return keycloakTS.set(ctx, &keycloakState{localURL: keycloakURL})
		}).
		Assess("OIDC discovery returns correct issuer URL", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := keycloakTS.get(ctx)

			d := discoverOIDC(t, s.localURL, keycloakRealm)
			require.Contains(t, d.Issuer, "keycloak",
				"issuer must reference the Keycloak server")
			require.Contains(t, d.Issuer, keycloakRealm)
			require.NotEmpty(t, d.TokenEndpoint)
			require.NotEmpty(t, d.AuthorizationEndpoint)

			return ctx
		}).
		Assess("token endpoint issues valid tokens", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := keycloakTS.get(ctx)

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
			s := keycloakTS.get(ctx)

			// 1. Get a user token from Keycloak (mcp-client, public client).
			token := mcpUserToken(t, s.localURL, "openid", "mcp-server")

			// 2. Deploy the MCP server with OIDC config and the cert-manager CA
			//    mounted so it can trust Keycloak's TLS. The CA secret is created
			//    by the copyKeycloakCASecret pre-install hook before Helm install.
			dep := deployServer(ctx, t, cfg, "keycloak-oidc",
				withConfig(oidcServerConfig(nil)),
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
			output := requireToolCallSuccess(t, mcpClient, "namespaces_list", map[string]any{})
			require.Contains(t, output, dep.namespace,
				"OIDC-authenticated namespaces_list should include the server's own namespace")

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
