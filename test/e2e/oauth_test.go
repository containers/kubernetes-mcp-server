//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

const (
	keycloakRealm = "openshift"

	// caSecretName is the secret holding the CA cert the MCP server trusts when
	// talking to Keycloak; caMountPath is where the chart mounts it in the pod.
	caSecretName = "keycloak-ca"
	caMountPath  = "/etc/keycloak-ca"
)

// oidcDiscovery is the subset of the OIDC discovery document the tests use.
type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	TokenEndpoint         string `json:"token_endpoint"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
}

// discoverOIDC fetches the realm's OIDC discovery document and asserts 200.
func discoverOIDC(t *testing.T, baseURL, realm string) oidcDiscovery {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	discoveryURL := fmt.Sprintf("%s/realms/%s/.well-known/openid-configuration", baseURL, realm)

	resp, err := client.Get(discoveryURL)
	require.NoError(t, err, "GET OIDC discovery")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "OIDC discovery status")

	var d oidcDiscovery
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&d), "decode OIDC discovery")
	return d
}

// tokenRequest describes an OAuth2 token request against Keycloak. GrantType
// defaults to "password" when empty; Scopes are joined space-delimited into the
// scope parameter. Only the fields relevant to the chosen grant need to be set.
// For the token-exchange grant, set subjectToken (and usually audience); the
// subject/requested token types default to access_token.
type tokenRequest struct {
	baseURL      string
	realm        string
	grantType    string
	clientID     string
	clientSecret string
	username     string
	password     string
	scopes       []string
	// subjectToken is the token being exchanged (token-exchange grant only).
	subjectToken string
	// audience is the requested audience of the exchanged token (token-exchange
	// grant only).
	audience string
}

const (
	// grantTypeTokenExchange is the RFC 8693 token-exchange grant type.
	grantTypeTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"
	// tokenTypeAccessToken is the RFC 8693 access-token type URN.
	tokenTypeAccessToken = "urn:ietf:params:oauth:token-type:access_token"
)

// tokenResponse is the subset of the token endpoint response the tests use.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// requestToken performs the token request and returns the full parsed response,
// asserting a 200. Use fetchToken when only the access token is needed.
func requestToken(t *testing.T, req tokenRequest) tokenResponse {
	t.Helper()
	grantType := req.grantType
	if grantType == "" {
		grantType = "password"
	}

	form := url.Values{
		"grant_type": {grantType},
		"client_id":  {req.clientID},
	}
	if req.clientSecret != "" {
		form.Set("client_secret", req.clientSecret)
	}
	if req.username != "" {
		form.Set("username", req.username)
	}
	if req.password != "" {
		form.Set("password", req.password)
	}
	if len(req.scopes) > 0 {
		form.Set("scope", strings.Join(req.scopes, " "))
	}
	if req.subjectToken != "" {
		form.Set("subject_token", req.subjectToken)
		form.Set("subject_token_type", tokenTypeAccessToken)
		form.Set("requested_token_type", tokenTypeAccessToken)
	}
	if req.audience != "" {
		form.Set("audience", req.audience)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", req.baseURL, req.realm)
	resp, err := client.PostForm(tokenURL, form)
	require.NoError(t, err, "POST token endpoint")
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"token request failed (grant=%s client=%s): %s", grantType, req.clientID, string(body))

	var tr tokenResponse
	require.NoError(t, json.Unmarshal(body, &tr), "decode token response")
	return tr
}

// fetchToken performs the token request and returns a non-empty access token.
func fetchToken(t *testing.T, req tokenRequest) string {
	t.Helper()
	tr := requestToken(t, req)
	require.NotEmpty(t, tr.AccessToken, "expected non-empty access token")
	return tr.AccessToken
}

// fetchExchangedToken performs an RFC 8693 token exchange against Keycloak using
// the confidential mcp-server client, exchanging subjectToken for an access token
// with audience "openshift" (the audience the kube-apiserver requires via
// --oidc-client-id). This is the same exchange the server performs internally in
// OIDC+STS mode; the forwarded-token tests use it to obtain a cluster-accepted
// token to send in modes where the server does NOT exchange (require_oauth=false,
// passthrough), so the tool call can act as the user rather than being rejected
// by the apiserver for a wrong audience.
func fetchExchangedToken(t *testing.T, keycloakURL, realm, subjectToken string) string {
	t.Helper()
	return fetchToken(t, tokenRequest{
		baseURL:      keycloakURL,
		realm:        realm,
		grantType:    grantTypeTokenExchange,
		clientID:     "mcp-server",
		clientSecret: "mcp-server-dev-secret",
		subjectToken: subjectToken,
		audience:     "openshift",
		scopes:       []string{"mcp:openshift"},
	})
}

// requireCanListSecrets asserts that the MCP client can list Secrets in namespace
// (cluster-admin identity). Listing Secrets is the identity discriminator: the
// built-in "view" ClusterRole excludes Secrets while cluster-admin allows them.
func requireCanListSecrets(t *testing.T, mcpClient *test.McpClient, namespace string) {
	t.Helper()
	requireToolCallSuccess(t, mcpClient, "resources_list", map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"namespace":  namespace,
	})
}

// requireCannotListSecrets asserts that the MCP client cannot list Secrets in
// namespace (view identity — forbidden).
func requireCannotListSecrets(t *testing.T, mcpClient *test.McpClient, namespace string) {
	t.Helper()
	requireToolCallError(t, mcpClient, "resources_list", map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"namespace":  namespace,
	})
}

// requireKeycloak checks that Keycloak is installed, skips if not, and returns
// a port-forwarded URL to the Keycloak service. Registers cleanup automatically.
func requireKeycloak(ctx context.Context, t *testing.T, cfg *envconf.Config) string {
	t.Helper()
	clientset, err := clientsetFromKubeconfig(cfg.KubeconfigFile())
	require.NoError(t, err)

	_, err = clientset.CoreV1().Services("keycloak").Get(ctx, "keycloak", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		t.Skip("Keycloak not installed — run 'make keycloak-install' first")
	}
	require.NoError(t, err, "check keycloak service")

	restCfg, err := clientcmd.BuildConfigFromFlags("", cfg.KubeconfigFile())
	require.NoError(t, err, "build rest config")

	keycloakURL, stopPF := portForwardService(ctx, t, restCfg, clientset, "keycloak", "keycloak", 8080)
	t.Cleanup(stopPF)
	return keycloakURL
}

// mcpUserToken fetches an access token for the standard "mcp" user (cluster-admin).
func mcpUserToken(t *testing.T, keycloakURL string, scopes ...string) string {
	t.Helper()
	return fetchToken(t, tokenRequest{
		baseURL:  keycloakURL,
		realm:    keycloakRealm,
		clientID: "mcp-client",
		username: "mcp",
		password: "mcp",
		scopes:   scopes,
	})
}

// mcpViewerToken fetches an access token for the "mcp-viewer" user (view role via group).
func mcpViewerToken(t *testing.T, keycloakURL string, scopes ...string) string {
	t.Helper()
	return fetchToken(t, tokenRequest{
		baseURL:  keycloakURL,
		realm:    keycloakRealm,
		clientID: "mcp-client",
		username: "mcp-viewer",
		password: "mcp-viewer",
		scopes:   scopes,
	})
}

// copyKeycloakCASecret copies the cert-manager self-signed CA into the test
// namespace as the caSecretName secret so the MCP server pod can mount it and
// trust Keycloak's TLS. Intended as a deployServer preInstall hook.
func copyKeycloakCASecret(ctx context.Context, t *testing.T, clientset kubernetes.Interface, namespace string) {
	t.Helper()
	caSecret, err := clientset.CoreV1().Secrets("cert-manager").Get(ctx, "selfsigned-ca-secret", metav1.GetOptions{})
	require.NoError(t, err, "get cert-manager CA secret")

	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: caSecretName},
		Data:       map[string][]byte{"ca.crt": caSecret.Data["ca.crt"]},
	}, metav1.CreateOptions{})
	require.NoError(t, err, "create CA secret in test namespace")
}

// keycloakCAVolumeValues returns Helm values mounting the keycloak-ca secret at
// caMountPath. Reference caMountPath/ca.crt as certificate_authority in config.
func keycloakCAVolumeValues() map[string]any {
	return map[string]any{
		"extraVolumes": []map[string]any{
			{
				"name":   caSecretName,
				"secret": map[string]any{"secretName": caSecretName},
			},
		},
		"extraVolumeMounts": []map[string]any{
			{
				"name":      caSecretName,
				"mountPath": caMountPath,
				"readOnly":  true,
			},
		},
	}
}

const (
	// stsAssertionSecretName is the secret holding the client cert+key the server
	// signs assertions with; stsAssertionMountPath is where the chart mounts it.
	stsAssertionSecretName = "sts-assertion"
	stsAssertionMountPath  = "/etc/sts-assertion"
	// stsAssertionCertPath/KeyPath are the in-pod paths to reference as
	// sts_client_cert_file / sts_client_key_file in config.
	stsAssertionCertPath = stsAssertionMountPath + "/tls.crt"
	stsAssertionKeyPath  = stsAssertionMountPath + "/tls.key"

	// stsAssertionCertFile/KeyFile are the on-disk paths (relative to this package,
	// which is `go test`'s working directory) of the throwaway RSA keypair used by
	// TestOAuthSTSAssertion for RFC 7523 private-key-JWT client authentication. The
	// private key is NEVER committed: it is generated on demand into a gitignored
	// folder by `make keycloak-gen-sts-keypair` (also run by `make keycloak-install`,
	// which renders the matching cert into Keycloak's mcp-server-jwt client). The
	// server signs client assertions with the key; Keycloak trusts the matching cert.
	stsAssertionCertFile = "testdata/generated/sts-assertion.crt"
	stsAssertionKeyFile  = "testdata/generated/sts-assertion.key"
)

// requireStsAssertionKeypair skips the test when the generated D4 keypair is
// absent, keeping the suite convention of skipping (not failing) on a missing
// optional fixture. The keypair is generated on demand and never committed.
func requireStsAssertionKeypair(t *testing.T) {
	t.Helper()
	for _, f := range []string{stsAssertionCertFile, stsAssertionKeyFile} {
		if _, err := os.Stat(f); err != nil {
			t.Skipf("STS assertion keypair not found (%s); run 'make keycloak-gen-sts-keypair' to generate it", f)
		}
	}
}

// copyStsAssertionSecret creates a Secret in the test namespace holding the
// generated throwaway client cert+key so the server pod can mount them and sign
// RFC 7523 client assertions. Intended as a deployServer preInstall hook; call
// requireStsAssertionKeypair in Setup first so a missing keypair skips cleanly.
func copyStsAssertionSecret(ctx context.Context, t *testing.T, clientset kubernetes.Interface, namespace string) {
	t.Helper()
	certPEM, err := os.ReadFile(stsAssertionCertFile)
	require.NoError(t, err, "read STS assertion cert %s", stsAssertionCertFile)
	keyPEM, err := os.ReadFile(stsAssertionKeyFile)
	require.NoError(t, err, "read STS assertion key %s", stsAssertionKeyFile)
	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: stsAssertionSecretName},
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err, "create STS assertion secret in test namespace")
}

// copyKeycloakCAAndAssertionSecrets is a preInstall hook creating both the
// Keycloak CA secret (TLS trust) and the STS assertion keypair secret. The
// assertion test needs both mounted before the pod starts.
func copyKeycloakCAAndAssertionSecrets(ctx context.Context, t *testing.T, clientset kubernetes.Interface, namespace string) {
	t.Helper()
	copyKeycloakCASecret(ctx, t, clientset, namespace)
	copyStsAssertionSecret(ctx, t, clientset, namespace)
}

// stsAssertionVolumeValues returns Helm values mounting the STS assertion
// keypair secret. Combine with keycloakCAVolumeValues via mergeValues when both
// are needed — the deep merge concatenates the extraVolumes/extraVolumeMounts slices.
func stsAssertionVolumeValues() map[string]any {
	return map[string]any{
		"extraVolumes": []map[string]any{
			{"name": stsAssertionSecretName, "secret": map[string]any{"secretName": stsAssertionSecretName}},
		},
		"extraVolumeMounts": []map[string]any{
			{"name": stsAssertionSecretName, "mountPath": stsAssertionMountPath, "readOnly": true},
		},
	}
}

// mcpInitProbe issues a raw JSON-RPC initialize request to the MCP endpoint and
// returns the HTTP status and response headers. It bypasses the go-sdk client,
// which hides the HTTP status. An empty authHeader sends no Authorization header;
// otherwise it is sent verbatim (e.g. "Bearer <token>" or "Basic <creds>").
func mcpInitProbe(t *testing.T, endpoint, authHeader string) (int, http.Header) {
	t.Helper()
	const body = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}}`
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	require.NoError(t, err, "build MCP request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "POST MCP endpoint")
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, resp.Header
}

// requireUnauthorized asserts that the MCP endpoint rejects a request with HTTP
// 401 and a WWW-Authenticate header carrying error="<wantErrorToken>". An empty
// token sends no Authorization header. It returns the WWW-Authenticate header so
// callers can make further assertions (e.g. on the audience).
func requireUnauthorized(t *testing.T, endpoint, token, wantErrorToken string) string {
	t.Helper()
	authHeader := ""
	if token != "" {
		authHeader = "Bearer " + token
	}
	return requireUnauthorizedRaw(t, endpoint, authHeader, wantErrorToken)
}

// requireUnauthorizedRaw is like requireUnauthorized but sends authHeader verbatim,
// so it can exercise non-Bearer schemes (e.g. "Basic ...") or a bare "Bearer ".
func requireUnauthorizedRaw(t *testing.T, endpoint, authHeader, wantErrorToken string) string {
	t.Helper()
	status, header := mcpInitProbe(t, endpoint, authHeader)
	require.Equal(t, http.StatusUnauthorized, status, "expected 401 from MCP endpoint")
	wwwAuth := header.Get("WWW-Authenticate")
	require.Contains(t, wwwAuth, fmt.Sprintf(`error="%s"`, wantErrorToken),
		"WWW-Authenticate = %q", wwwAuth)
	return wwwAuth
}

// requireWellKnown GETs an unauthenticated well-known metadata endpoint on the MCP
// server, asserts HTTP 200, and returns the decoded JSON document and headers.
func requireWellKnown(t *testing.T, baseURL, path string) (map[string]any, http.Header) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(baseURL + path)
	require.NoError(t, err, "GET %s", path)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 from %s", path)

	var doc map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&doc), "decode %s", path)
	return doc, resp.Header
}

// requireHTTPStatus GETs url and asserts the response status equals want.
func requireHTTPStatus(t *testing.T, url string, want int) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	require.NoError(t, err, "GET %s", url)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, want, resp.StatusCode, "GET %s status", url)
}

// oidcServerConfig returns the standard OIDC + STS token-exchange server config
// used by the Keycloak-backed flow tests. When oauthScopes is nil it advertises
// the default ["openid", "mcp-server"] scopes; pass a value to override (e.g. to
// prove scope is not enforced). The config points at the in-cluster Keycloak
// service and trusts the CA mounted by keycloakCAVolumeValues/copyKeycloakCASecret.
func oidcServerConfig(oauthScopes []string) string {
	scopes := `["openid", "mcp-server"]`
	if oauthScopes != nil {
		scopes = tomlStringArray(oauthScopes)
	}
	return fmt.Sprintf(`
		require_oauth = true
		oauth_audience = "mcp-server"
		oauth_scopes = %s
		validate_token = false
		authorization_url = "https://keycloak.keycloak.svc:8443/realms/openshift"
		sts_client_id = "mcp-server"
		sts_client_secret = "mcp-server-dev-secret"
		sts_audience = "openshift"
		sts_scopes = ["mcp:openshift"]
		certificate_authority = "%s/ca.crt"
	`, scopes, caMountPath)
}

// tomlStringArray renders a Go string slice as a TOML array literal.
func tomlStringArray(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// mintJWT signs claims into a compact JWT using a freshly generated, throwaway
// RSA key. The token is structurally valid and — with a correct audience and a
// future expiry — passes the server's offline validation (ParseJWTClaims +
// ValidateOffline), but its signature cannot be verified against any real OIDC
// provider's JWKS. A server in OIDC mode therefore rejects it with
// invalid_token only during online verification, exercising a branch that a
// malformed token (A4) or a wrong-audience token (A5) cannot reach.
func mintJWT(t *testing.T, claims jwt.Claims) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "generate throwaway RSA key")

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	require.NoError(t, err, "create JWT signer")

	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	require.NoError(t, err, "sign JWT")
	return token
}
