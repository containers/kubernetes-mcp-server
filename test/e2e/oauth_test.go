//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
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
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "OIDC discovery status")

	var d oidcDiscovery
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&d), "decode OIDC discovery")
	return d
}

// tokenRequest describes an OAuth2 token request against Keycloak. GrantType
// defaults to "password" when empty; Scopes are joined space-delimited into the
// scope parameter. Only the fields relevant to the chosen grant need to be set.
type tokenRequest struct {
	baseURL      string
	realm        string
	grantType    string
	clientID     string
	clientSecret string
	username     string
	password     string
	scopes       []string
}

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

	client := &http.Client{Timeout: 10 * time.Second}
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", req.baseURL, req.realm)
	resp, err := client.PostForm(tokenURL, form)
	require.NoError(t, err, "POST token endpoint")
	defer resp.Body.Close()

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

// requireUnauthorized asserts that the MCP endpoint rejects a request with HTTP
// 401 and a WWW-Authenticate header carrying error="<wantErrorToken>". An empty
// token sends no Authorization header. It bypasses the go-sdk client (which hides
// the HTTP status) by issuing a raw JSON-RPC initialize request.
func requireUnauthorized(t *testing.T, endpoint, token, wantErrorToken string) {
	t.Helper()
	const body = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}}`
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	require.NoError(t, err, "build MCP request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "POST MCP endpoint")
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "expected 401 from MCP endpoint")
	wwwAuth := resp.Header.Get("WWW-Authenticate")
	require.Contains(t, wwwAuth, fmt.Sprintf(`error="%s"`, wantErrorToken),
		"WWW-Authenticate = %q", wwwAuth)
}
