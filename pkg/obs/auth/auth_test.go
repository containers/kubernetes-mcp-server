package auth

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	promapi "github.com/prometheus/client_golang/api"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestBuildRoundTripper(t *testing.T) {
	// Mock HTTP server which returns the same auth header value
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Received-Auth", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	caData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})

	tests := []struct {
		name           string
		authMode       AuthMode
		ctxValue       string
		restConfig     *rest.Config
		wantErr        bool
		wantAuthHeader string
	}{
		{
			name:           "header mode forwards bearer token",
			authMode:       AuthModeHeader,
			ctxValue:       "Bearer header-token",
			restConfig:     &rest.Config{},
			wantAuthHeader: "Bearer header-token",
		},
		{
			name:           "header mode forwards bearer token without prefix",
			authMode:       AuthModeHeader,
			ctxValue:       "header-token",
			restConfig:     &rest.Config{},
			wantAuthHeader: "Bearer header-token",
		},
		{
			name:           "header auth mode without token in context succeeds without auth",
			authMode:       AuthModeHeader,
			ctxValue:       "",
			restConfig:     &rest.Config{BearerToken: "kubeconfig-token"},
			wantAuthHeader: "",
		},
		{
			name:     "kubeconfig mode forwards restconfig bearer token",
			authMode: AuthModeKubeConfig,
			ctxValue: "",
			restConfig: &rest.Config{
				BearerToken: "kubeconfig-token",
			},
			wantAuthHeader: "Bearer kubeconfig-token",
		},
		{
			name:     "kubeconfig auth mode ignores context token",
			authMode: AuthModeKubeConfig,
			ctxValue: "Bearer header-token",
			restConfig: &rest.Config{
				BearerToken: "kubeconfig-token",
			},
			wantAuthHeader: "Bearer kubeconfig-token",
		},
		{
			name:           "kubeconfig auth mode without bearer token succeeds without auth",
			authMode:       AuthModeKubeConfig,
			ctxValue:       "",
			restConfig:     &rest.Config{},
			wantAuthHeader: "",
		},
		{
			name:       "nil REST config fails",
			authMode:   AuthModeHeader,
			ctxValue:   "Bearer header-token",
			restConfig: nil,
			wantErr:    true,
		},
		{
			name:       "unsupported auth mode fails",
			authMode:   AuthMode("invalid"),
			ctxValue:   "Bearer token",
			restConfig: &rest.Config{},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.ctxValue != "" {
				ctx = context.WithValue(ctx, kubernetes.OAuthAuthorizationHeader, tt.ctxValue)
			}
			if tt.restConfig != nil {
				tt.restConfig.CAData = caData
			}

			rt, err := BuildRoundTripper(ctx, config.BaseDefault(), tt.restConfig, tt.authMode, true, false)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			req, err := http.NewRequest("GET", server.URL+"/test", http.NoBody)
			require.NoError(t, err)

			resp, err := rt.RoundTrip(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())

			require.Equal(t, tt.wantAuthHeader, resp.Header.Get("X-Received-Auth"))
		})
	}
}

func TestCreateHeaderAPIConfig(t *testing.T) {
	// This test validates the complete flow: context -> token extraction -> RoundTripper adds Authorization header
	token := "test-bearer-token-12345"

	// Step 1: Create a mock transport to capture the request
	var capturedRequest *http.Request
	mockTransport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			// Capture the request before it's sent
			capturedRequest = req.Clone(req.Context())
			return nil, nil
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Prevent actual network call
			return nil, fmt.Errorf("network call prevented in test")
		},
	}

	// Step 2: Temporarily replace the default RoundTripper
	originalTransport := promapi.DefaultRoundTripper
	promapi.DefaultRoundTripper = mockTransport
	defer func() {
		promapi.DefaultRoundTripper = originalTransport
	}()

	// Step 3: Create an HTTP request with Bearer token and extract token into context
	req, err := http.NewRequest("GET", "/test", http.NoBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set(string(kubernetes.OAuthAuthorizationHeader), "Bearer "+token)
	ctx := context.Background()
	ctx = ContextWithAuthFromRequest(ctx, req)

	// Step 4: Create round tripper using the complete production code path
	rt, err := BuildRoundTripper(ctx, config.BaseDefault(), &rest.Config{}, AuthModeHeader, true, false)
	if err != nil {
		t.Fatalf("failed to create round tripper: %v", err)
	}

	// Step 5: Create a test request
	testReq, err := http.NewRequest("GET", "https://prometheus.example.com/api/v1/query", http.NoBody)
	if err != nil {
		t.Fatalf("failed to create test request: %v", err)
	}
	testReq.Header.Set("X-Test", "test-value")

	// Step 6: Make the request using the RoundTripper
	// The Proxy function captures the request before DialContext prevents the actual network call
	resp, _ := rt.RoundTrip(testReq)
	// We ignore the error from DialContext since we only care about the captured request
	if resp != nil && resp.Body != nil {
		require.NoError(t, resp.Body.Close())
	}

	// Step 7: Verify the Authorization header was added to the captured request
	authHeader := capturedRequest.Header.Get("Authorization")
	expectedAuthHeader := "Bearer " + token
	if authHeader != expectedAuthHeader {
		t.Errorf("expected Authorization header %q, got %q", expectedAuthHeader, authHeader)
	}

	// Verify the test header is still present
	if capturedRequest.Header.Get("X-Test") != "test-value" {
		t.Error("expected X-Test header to be preserved")
	}
}

func TestBuildRoundTripperTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	caData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})

	request := func(t *testing.T, rt http.RoundTripper, address string) error {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address, http.NoBody)
		require.NoError(t, err)
		resp, err := rt.RoundTrip(req)
		if err == nil {
			require.NoError(t, resp.Body.Close())
		}
		return err
	}

	t.Run("untrusted certificate is rejected", func(t *testing.T) {
		rt, err := BuildRoundTripper(t.Context(), config.BaseDefault(), &rest.Config{}, AuthModeHeader, true, false)
		require.NoError(t, err)
		require.Error(t, request(t, rt, server.URL))
	})
	t.Run("REST CA data is trusted with hostname verification", func(t *testing.T) {
		rt, err := BuildRoundTripper(t.Context(), config.BaseDefault(), &rest.Config{TLSClientConfig: rest.TLSClientConfig{CAData: caData}}, AuthModeHeader, true, false)
		require.NoError(t, err)
		require.NoError(t, request(t, rt, server.URL))
		require.Error(t, request(t, rt, strings.Replace(server.URL, "127.0.0.1", "localhost", 1)))
	})
	t.Run("REST CA file is trusted", func(t *testing.T) {
		caFile := t.TempDir() + "/ca.pem"
		require.NoError(t, os.WriteFile(caFile, caData, 0600))
		rt, err := BuildRoundTripper(t.Context(), config.BaseDefault(), &rest.Config{TLSClientConfig: rest.TLSClientConfig{CAFile: caFile}}, AuthModeHeader, true, false)
		require.NoError(t, err)
		require.NoError(t, request(t, rt, server.URL))
	})
	t.Run("invalid CA fails closed", func(t *testing.T) {
		_, err := BuildRoundTripper(t.Context(), config.BaseDefault(), &rest.Config{TLSClientConfig: rest.TLSClientConfig{CAData: []byte("invalid")}}, AuthModeHeader, true, false)
		require.ErrorContains(t, err, "CAData")
	})
	t.Run("insecure mode is rejected", func(t *testing.T) {
		_, err := BuildRoundTripper(t.Context(), config.BaseDefault(), &rest.Config{}, AuthModeHeader, true, true)
		require.ErrorContains(t, err, "insecure TLS is not supported")
	})
	t.Run("missing server config is rejected", func(t *testing.T) {
		_, err := BuildRoundTripper(t.Context(), nil, &rest.Config{}, AuthModeHeader, true, false)
		require.ErrorContains(t, err, "no server config available")
	})
	t.Run("require TLS rejects HTTP", func(t *testing.T) {
		cfg := config.BaseDefault()
		cfg.RequireTLS.SetForTest(true)
		rt, err := BuildRoundTripper(t.Context(), cfg, &rest.Config{}, AuthModeHeader, false, false)
		require.NoError(t, err)
		require.ErrorContains(t, request(t, rt, "http://example.com"), "require_tls")
	})
	t.Run("TLS minimum version is applied", func(t *testing.T) {
		tls12Server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		tls12Server.TLS = &tls.Config{MaxVersion: tls.VersionTLS12}
		tls12Server.StartTLS()
		defer tls12Server.Close()
		cfg := config.BaseDefault()
		cfg.TLSMinVersion.SetForTest("1.3")
		ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tls12Server.Certificate().Raw})
		rt, err := BuildRoundTripper(t.Context(), cfg, &rest.Config{TLSClientConfig: rest.TLSClientConfig{CAData: ca}}, AuthModeHeader, true, false)
		require.NoError(t, err)
		require.Error(t, request(t, rt, tls12Server.URL))
	})
	t.Run("invalid cipher suite is rejected", func(t *testing.T) {
		cfg := config.BaseDefault()
		cfg.TLSCipherSuites.SetForTest([]string{"UNKNOWN_CIPHER"})
		_, err := BuildRoundTripper(t.Context(), cfg, &rest.Config{}, AuthModeHeader, true, false)
		require.ErrorContains(t, err, "UNKNOWN_CIPHER")
	})
	t.Run("HTTP still forwards the bearer token", func(t *testing.T) {
		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Received-Auth", r.Header.Get("Authorization"))
		}))
		defer httpServer.Close()
		rt, err := BuildRoundTripper(t.Context(), config.BaseDefault(), &rest.Config{BearerToken: "my-token"}, AuthModeKubeConfig, false, false)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, httpServer.URL, http.NoBody)
		require.NoError(t, err)
		resp, err := rt.RoundTrip(req)
		require.NoError(t, err)
		defer func() { require.NoError(t, resp.Body.Close()) }()
		require.Equal(t, "Bearer my-token", resp.Header.Get("X-Received-Auth"))
	})
}

func TestContextWithAuthFromRequest(t *testing.T) {
	tests := []struct {
		name         string
		authValue    string
		wantCtxValue string
	}{
		{
			name:         "bearer token is stored in context",
			authValue:    "Bearer my-token",
			wantCtxValue: "Bearer my-token",
		},
		{
			name:      "missing header returns unmodified context",
			authValue: "",
		},
		{
			name:      "non-bearer value is ignored",
			authValue: "Basic dXNlcjpwYXNz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/test", http.NoBody)
			require.NoError(t, err)
			if tt.authValue != "" {
				req.Header.Set(string(kubernetes.OAuthAuthorizationHeader), tt.authValue)
			}

			ctx := ContextWithAuthFromRequest(t.Context(), req)
			got, _ := ctx.Value(kubernetes.OAuthAuthorizationHeader).(string)
			require.Equal(t, tt.wantCtxValue, got)
		})
	}
}
