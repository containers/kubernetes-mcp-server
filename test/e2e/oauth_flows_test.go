//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type oauthFlowsState struct {
	keycloakURL string
	dep         *serverDeployment
}

var oauthFlowsTS testState[oauthFlowsState]

// TestOAuthOIDCFlows exercises the OAuth surface of a server running in OIDC mode
// (require_oauth=true with a real Keycloak authorization server): the rejection
// surface (Group A), the unauthenticated well-known metadata endpoints (Group E),
// and the scope-not-enforced guard (F1). The happy-path chain is covered by
// TestKeycloakOIDC; this test focuses on everything around it.
func TestOAuthOIDCFlows(t *testing.T) {
	f := features.New("oauth-oidc-flows").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			keycloakURL := requireKeycloak(ctx, t, cfg)

			// Deploy the server in OIDC mode; well-known assertions rely on it
			// proxying to Keycloak over the in-cluster service DNS + mounted CA.
			dep := deployServer(ctx, t, cfg, "oauth-oidc",
				withConfig(oidcServerConfig(nil)),
				withValues(mergeValues(viewClusterRoleBindingValues(), keycloakCAVolumeValues())),
				withPreInstall(copyKeycloakCASecret),
			)

			return oauthFlowsTS.set(ctx, &oauthFlowsState{
				keycloakURL: keycloakURL,
				dep:         dep,
			})
		}).
		Assess("rejects missing, malformed, and wrong-audience tokens", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := oauthFlowsTS.get(ctx)
			ep := s.dep.serverURL + "/mcp"

			// A1: no Authorization header at all.
			requireUnauthorized(t, ep, "", "missing_token")
			// A2: an Authorization header that is not a Bearer scheme.
			requireUnauthorizedRaw(t, ep, "Basic dXNlcjpwYXNz", "missing_token")
			// A3 (empty "Bearer " token → invalid_token) is intentionally omitted:
			// HTTP header values are trimmed of trailing whitespace in transit, so
			// "Bearer " arrives as "Bearer", fails the "Bearer " prefix check, and
			// yields missing_token (same path as A2). The server's empty-token
			// branch is defensive code unreachable through a compliant HTTP client.
			// A4: a well-formed header carrying a token that is not a JWT.
			requireUnauthorized(t, ep, "not-a-jwt", "invalid_token")

			// A5: a genuine Keycloak token whose audience does not include the
			// configured oauth_audience. Requesting only the "openid" scope omits
			// the mcp-server audience mapper, so ValidateOffline rejects it.
			wrongAudience := fetchToken(t, tokenRequest{
				baseURL:  s.keycloakURL,
				realm:    keycloakRealm,
				clientID: "mcp-client",
				username: "mcp",
				password: "mcp",
				scopes:   []string{"openid"},
			})
			www := requireUnauthorized(t, ep, wrongAudience, "invalid_token")

			// A8: the challenge advertises the audience the server expects.
			require.Contains(t, www, `audience="mcp-server"`,
				"WWW-Authenticate should advertise the expected audience: %q", www)

			// A7: a JWT with the correct audience and a future expiry, but signed
			// by a throwaway key the provider does not know. It passes offline
			// validation (audience + expiry) and is rejected only when the server
			// verifies the signature against Keycloak's JWKS — a different path
			// than A4 (unparseable) and A5 (offline audience mismatch).
			badlySigned := mintJWT(t, jwt.Claims{
				Issuer:   "https://keycloak.keycloak.svc:8443/realms/openshift",
				Subject:  "e2e-throwaway",
				Audience: jwt.Audience{"mcp-server"},
				Expiry:   jwt.NewNumericDate(time.Now().Add(time.Hour)),
				IssuedAt: jwt.NewNumericDate(time.Now()),
			})
			requireUnauthorized(t, ep, badlySigned, "invalid_token")

			// A6: a JWT with the correct audience but an expiry in the past.
			// ValidateOffline runs before the online provider check and treats a
			// zero Expected.Time as "now", so it rejects the token on expiry
			// (ErrExpired) before the signature is ever verified — a different
			// branch than A7 (future expiry, bad signature → online failure) and
			// A4 (unparseable). The expiry is set well beyond go-jose's default
			// 1-minute leeway.
			expired := mintJWT(t, jwt.Claims{
				Issuer:   "https://keycloak.keycloak.svc:8443/realms/openshift",
				Subject:  "e2e-throwaway",
				Audience: jwt.Audience{"mcp-server"},
				Expiry:   jwt.NewNumericDate(time.Now().Add(-time.Hour)),
				IssuedAt: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			})
			requireUnauthorized(t, ep, expired, "invalid_token")

			return ctx
		}).
		Assess("serves well-known metadata without authentication", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := oauthFlowsTS.get(ctx)
			base := s.dep.serverURL

			// E1: openid-configuration is proxied from Keycloak.
			oidcCfg, _ := requireWellKnown(t, base, "/.well-known/openid-configuration")
			issuer, _ := oidcCfg["issuer"].(string)
			require.Contains(t, issuer, "keycloak.keycloak.svc", "issuer = %q", issuer)
			require.NotEmpty(t, oidcCfg["token_endpoint"], "openid-configuration token_endpoint")

			// E2: oauth-protected-resource (RFC 9728) metadata, plus E8 CORS header.
			prm, hdr := requireWellKnown(t, base, "/.well-known/oauth-protected-resource")
			require.NotEmpty(t, prm["authorization_servers"], "protected-resource authorization_servers")
			require.Equal(t, []any{"header"}, prm["bearer_methods_supported"], "bearer_methods_supported")
			require.Equal(t, "*", hdr.Get("Access-Control-Allow-Origin"), "well-known CORS header")

			// E4: the configured oauth_scopes override the advertised
			// scopes_supported (applyConfigOverrides). oidcServerConfig(nil)
			// configures ["openid", "mcp-server"].
			require.Equal(t, []any{"openid", "mcp-server"}, prm["scopes_supported"],
				"oauth_scopes should override scopes_supported")

			// E3: oauth-authorization-server metadata.
			asm, _ := requireWellKnown(t, base, "/.well-known/oauth-authorization-server")
			require.NotEmpty(t, asm["token_endpoint"], "authorization-server token_endpoint")

			// E7: every fetch above succeeded with no Authorization header even
			// though require_oauth=true, proving well-known endpoints are exempt.
			return ctx
		}).
		Assess("does not enforce scopes beyond audience and signature", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := oauthFlowsTS.get(ctx)

			// F1: advertise a scope the user's token will not carry. The server
			// validates audience + signature, not scope, so a valid token must
			// still be accepted. This guards against a regression that starts
			// gating requests on oauth_scopes.
			dep := deployServer(ctx, t, cfg, "oauth-scope",
				withConfig(oidcServerConfig([]string{"mcp:absent"})),
				withValues(mergeValues(viewClusterRoleBindingValues(), keycloakCAVolumeValues())),
				withPreInstall(copyKeycloakCASecret),
			)

			token := mcpUserToken(t, s.keycloakURL, "openid", "mcp-server")

			mcpClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + token}),
			)
			t.Cleanup(mcpClient.Close)

			requireToolCallSuccess(t, mcpClient, "namespaces_list", map[string]any{})

			return ctx
		}).
		Assess("disable_dynamic_client_registration strips registration metadata", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// E5: with disable_dynamic_client_registration=true, the server must
			// strip registration_endpoint from the proxied metadata and advertise
			// require_request_uri_registration=false (applyConfigOverrides).
			dep := deployServer(ctx, t, cfg, "oauth-nodcr",
				withConfig(oidcServerConfig(nil)+"\ndisable_dynamic_client_registration = true\n"),
				withValues(mergeValues(viewClusterRoleBindingValues(), keycloakCAVolumeValues())),
				withPreInstall(copyKeycloakCASecret),
			)

			asm, _ := requireWellKnown(t, dep.serverURL, "/.well-known/oauth-authorization-server")
			require.NotContains(t, asm, "registration_endpoint",
				"registration_endpoint should be stripped when dynamic client registration is disabled")
			require.Equal(t, false, asm["require_request_uri_registration"],
				"require_request_uri_registration should be false")

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

// TestOAuthServerModes exercises the auth modes that do not need an OIDC provider:
// an unprotected server (require_oauth=false) and token-passthrough mode
// (skip_jwt_verification=true with no authorization_url). No Keycloak required.
func TestOAuthServerModes(t *testing.T) {
	f := features.New("oauth-server-modes").
		Assess("unprotected server allows anonymous tool calls", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// C1: require_oauth=false → requests need no token and run as the
			// pod ServiceAccount (bound to view).
			dep := deployServer(ctx, t, cfg, "oauth-open",
				withConfig("require_oauth = false"),
				withValues(viewClusterRoleBindingValues()),
			)

			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)

			output := requireToolCallSuccess(t, mcpClient, "namespaces_list", map[string]any{})
			require.Contains(t, output, dep.namespace,
				"anonymous namespaces_list should include the server's own namespace")

			return ctx
		}).
		Assess("passthrough mode rejects missing token and hides well-known", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// C4: passthrough forwards tokens unvalidated, but a completely
			// missing token is still rejected up front.
			dep := deployServer(ctx, t, cfg, "oauth-passthrough",
				withConfig("require_oauth = true\nskip_jwt_verification = true"),
				withValues(viewClusterRoleBindingValues()),
			)

			requireUnauthorized(t, dep.serverURL+"/mcp", "", "missing_token")

			// E6: with no authorization_url configured, well-known returns 404.
			requireHTTPStatus(t, dep.serverURL+"/.well-known/oauth-protected-resource", http.StatusNotFound)

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

type oauthIdentityState struct {
	// exchangedToken is a user-derived token with audience "openshift", already
	// exchanged by the test so the apiserver accepts it when the server forwards
	// it without exchanging (require_oauth=false, passthrough).
	exchangedToken string
}

var oauthIdentityTS testState[oauthIdentityState]

// TestOAuthForwardedIdentity proves that in the token-forwarding auth modes the
// tool call acts as the forwarded end-user (realm user "mcp", mapped to
// cluster-admin), not the pod ServiceAccount (bound to view). The discriminator
// is listing Secrets: view cannot, cluster-admin can. Because the kube-apiserver
// enforces audience "openshift" and these modes do NOT run STS, the test forwards
// an already-exchanged openshift-audience token (fetchExchangedToken).
func TestOAuthForwardedIdentity(t *testing.T) {
	f := features.New("oauth-forwarded-identity").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			keycloakURL := requireKeycloak(ctx, t, cfg)

			// User logs in (aud=mcp-server), then the test exchanges the token for
			// an aud=openshift token the apiserver will accept when forwarded.
			userToken := mcpUserToken(t, keycloakURL, "openid", "mcp-server")
			exchanged := fetchExchangedToken(t, keycloakURL, keycloakRealm, userToken)

			return oauthIdentityTS.set(ctx, &oauthIdentityState{exchangedToken: exchanged})
		}).
		Assess("require_oauth=false forwards the user token and acts as that user", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// C2: require_oauth=false forwards a present Bearer header to the
			// cluster. With no token the request runs as the view SA; with the
			// forwarded user token it runs as the cluster-admin user.
			s := oauthIdentityTS.get(ctx)
			dep := deployServer(ctx, t, cfg, "oauth-open-fwd",
				withConfig("require_oauth = false"),
				withValues(viewClusterRoleBindingValues()),
			)

			// SA baseline (no token): view can list namespaces but not Secrets.
			saClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(saClient.Close)
			requireToolCallSuccess(t, saClient, "namespaces_list", map[string]any{})
			requireCannotListSecrets(t, saClient, dep.namespace)

			// Forwarded user token: acts as the cluster-admin user, so Secrets list.
			userClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + s.exchangedToken}),
			)
			t.Cleanup(userClient.Close)
			requireToolCallSuccess(t, userClient, "namespaces_list", map[string]any{})
			requireCanListSecrets(t, userClient, dep.namespace)

			return ctx
		}).
		Assess("passthrough forwards the user token unvalidated and acts as that user", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// C3: passthrough (skip_jwt_verification=true, no authorization_url,
			// no STS) forwards the token unvalidated; the cluster is the sole
			// authority. The forwarded user token acts as the cluster-admin user.
			s := oauthIdentityTS.get(ctx)
			dep := deployServer(ctx, t, cfg, "oauth-passthrough-fwd",
				withConfig("require_oauth = true\nskip_jwt_verification = true"),
				withValues(viewClusterRoleBindingValues()),
			)

			userClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + s.exchangedToken}),
			)
			t.Cleanup(userClient.Close)
			requireToolCallSuccess(t, userClient, "namespaces_list", map[string]any{})
			requireCanListSecrets(t, userClient, dep.namespace)

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

type oauthSTSState struct {
	keycloakURL string
}

var oauthSTSTS testState[oauthSTSState]

// TestOAuthSTSVariants exercises a token-exchange variant that differs from the
// built-in STS path covered by TestKeycloakOIDC (C5): an explicit
// token_exchange_strategy="rfc8693" with sts_auth_style="header" (client
// credentials sent as an HTTP Basic header rather than form params). This routes
// through the pluggable strategy exchanger (strategyBasedTokenExchange +
// injectClientAuth). A successful tool call proves the exchanged token is
// accepted by the apiserver, and listing Secrets proves the cluster-admin user
// identity is preserved through the strategy path.
func TestOAuthSTSVariants(t *testing.T) {
	f := features.New("oauth-sts-variants").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			keycloakURL := requireKeycloak(ctx, t, cfg)
			return oauthSTSTS.set(ctx, &oauthSTSState{keycloakURL: keycloakURL})
		}).
		Assess("rfc8693 strategy with header auth style completes the exchange chain", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := oauthSTSTS.get(ctx)

			// User token (aud=mcp-server); the server validates it and performs the
			// exchange itself via the rfc8693 strategy with Basic-header client auth.
			userToken := mcpUserToken(t, s.keycloakURL, "openid", "mcp-server")

			dep := deployServer(ctx, t, cfg, "oauth-sts-header",
				withConfig(oidcServerConfig(nil)+"\ntoken_exchange_strategy = \"rfc8693\"\nsts_auth_style = \"header\"\n"),
				withValues(mergeValues(viewClusterRoleBindingValues(), keycloakCAVolumeValues())),
				withPreInstall(copyKeycloakCASecret),
			)

			userClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + userToken}),
			)
			t.Cleanup(userClient.Close)

			requireToolCallSuccess(t, userClient, "namespaces_list", map[string]any{})
			requireCanListSecrets(t, userClient, dep.namespace)

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

type oauthGroupState struct {
	keycloakURL string
	dep         *serverDeployment
}

var oauthGroupTS testState[oauthGroupState]

// TestOAuthGroupRBAC proves that the OIDC groups claim drives Kubernetes RBAC.
// Two realm users authenticate through the SAME OIDC+STS server: "mcp" (no
// group, bound directly to cluster-admin) and "mcp-viewer" (member of group
// "mcp-viewers", bound to the built-in "view" ClusterRole via a Group subject).
// The only variable between them is the group->role mapping, so contrasting
// their ability to list Secrets (allowed for cluster-admin, forbidden for view)
// isolates the groups claim as the thing that grants the permissions. This also
// exercises a real non-admin OIDC identity, distinct from the pod ServiceAccount.
func TestOAuthGroupRBAC(t *testing.T) {
	f := features.New("oauth-group-rbac").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			keycloakURL := requireKeycloak(ctx, t, cfg)

			// One OIDC+STS server; both users' tokens are exchanged by it to the
			// openshift audience the apiserver requires (same path as C5).
			dep := deployServer(ctx, t, cfg, "oauth-group-rbac",
				withConfig(oidcServerConfig(nil)),
				withValues(mergeValues(viewClusterRoleBindingValues(), keycloakCAVolumeValues())),
				withPreInstall(copyKeycloakCASecret),
			)

			return oauthGroupTS.set(ctx, &oauthGroupState{
				keycloakURL: keycloakURL,
				dep:         dep,
			})
		}).
		Assess("the OIDC groups claim maps the user to the view ClusterRole", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := oauthGroupTS.get(ctx)

			// mcp-viewer is a member of group "mcp-viewers", bound to "view".
			// After the server exchanges the token to aud=openshift, the apiserver
			// authenticates it as user "...#mcp-viewer" in group "mcp-viewers".
			viewerToken := mcpViewerToken(t, s.keycloakURL, "openid", "mcp-server")
			viewerClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(s.dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + viewerToken}),
			)
			t.Cleanup(viewerClient.Close)

			// view can list namespaces...
			requireToolCallSuccess(t, viewerClient, "namespaces_list", map[string]any{})
			// ...but not read Secrets. If the group->view binding did not resolve,
			// mcp-viewer would have no permissions and namespaces_list would fail
			// too; if it resolved to more than view, Secrets would succeed.
			requireCannotListSecrets(t, viewerClient, s.dep.namespace)

			// Contrast: the mcp user (cluster-admin, no group) CAN list Secrets
			// through the same server and config — the only difference is the
			// group->role mapping.
			adminToken := mcpUserToken(t, s.keycloakURL, "openid", "mcp-server")
			adminClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(s.dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + adminToken}),
			)
			t.Cleanup(adminClient.Close)
			requireCanListSecrets(t, adminClient, s.dep.namespace)

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

// TestOAuthSTSAssertion exercises D4: STS token exchange with
// sts_auth_style="assertion" — private-key-JWT client authentication (RFC 7523),
// distinct from the client-secret auth used everywhere else (C5/D3). Instead of a
// shared secret, the server signs a client_assertion JWT with a committed throwaway
// cert/key (mounted from a Secret) and presents it to Keycloak's mcp-server-jwt
// client, which is registered (clientAuthenticatorType=client-jwt) to trust the
// matching certificate. Because a Keycloak client has exactly one authenticator
// type, this uses a dedicated client rather than flipping mcp-server. A successful
// tool call proves the BuildClientAssertion + injectClientAuth(AuthStyleAssertion)
// path completes the exchange end-to-end; listing Secrets proves the exchanged
// token still carries the cluster-admin identity of the mcp user.
func TestOAuthSTSAssertion(t *testing.T) {
	f := features.New("oauth-sts-assertion").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			keycloakURL := requireKeycloak(ctx, t, cfg)

			// The D4 keypair is generated on demand, not committed; skip if absent.
			requireStsAssertionKeypair(t)

			return oauthSTSTS.set(ctx, &oauthSTSState{keycloakURL: keycloakURL})
		}).
		Assess("assertion auth style signs a client_assertion and completes the exchange chain", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := oauthSTSTS.get(ctx)

			// Raw user token with aud=mcp-server-jwt (the exchanging client's own
			// resource-server audience). Keycloak's standard token exchange requires
			// the requesting client to be within the subject token's audience, so the
			// mcp-server-jwt client can only exchange a token minted for it. The
			// server validates this token and performs the exchange itself,
			// authenticating to Keycloak with a signed JWT assertion (mcp-server-jwt
			// client) rather than a client secret.
			userToken := mcpUserToken(t, s.keycloakURL, "openid", "mcp-server-jwt")

			// Dedicated config: unlike oidcServerConfig, this authenticates the
			// exchange as mcp-server-jwt with a signed assertion (cert/key files),
			// not the mcp-server client secret, and validates incoming tokens against
			// its own audience. Built as its own literal because TOML rejects the
			// duplicate sts_client_id that appending would create.
			assertionConfig := fmt.Sprintf(`
				require_oauth = true
				oauth_audience = "mcp-server-jwt"
				oauth_scopes = ["openid", "mcp-server-jwt"]
				validate_token = false
				authorization_url = "https://keycloak.keycloak.svc:8443/realms/openshift"
				sts_client_id = "mcp-server-jwt"
				sts_audience = "openshift"
				sts_scopes = ["mcp:openshift"]
				token_exchange_strategy = "rfc8693"
				sts_auth_style = "assertion"
				sts_client_cert_file = %q
				sts_client_key_file = %q
				certificate_authority = "%s/ca.crt"
			`, stsAssertionCertPath, stsAssertionKeyPath, caMountPath)

			dep := deployServer(ctx, t, cfg, "oauth-sts-assertion",
				withConfig(assertionConfig),
				withValues(mergeValues(viewClusterRoleBindingValues(), keycloakCAVolumeValues(), stsAssertionVolumeValues())),
				withPreInstall(copyKeycloakCAAndAssertionSecrets),
			)

			userClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(dep.serverURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + userToken}),
			)
			t.Cleanup(userClient.Close)

			requireToolCallSuccess(t, userClient, "namespaces_list", map[string]any{})
			requireCanListSecrets(t, userClient, dep.namespace)

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
