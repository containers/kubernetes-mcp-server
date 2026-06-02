package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

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

			rt, err := BuildRoundTripper(ctx, tt.restConfig, tt.authMode, true, true)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			req, err := http.NewRequest("GET", server.URL+"/test", http.NoBody)
			require.NoError(t, err)

			resp, err := rt.RoundTrip(req)
			require.NoError(t, err)
			resp.Body.Close()

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
	rt, err := BuildRoundTripper(ctx, &rest.Config{}, AuthModeHeader, true, true)
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
		resp.Body.Close() // Mainly to make the linter happy.
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
