package http

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
)

type httpContext struct {
	klogState       klog.State
	mockServer      *test.MockServer
	LogBuffer       bytes.Buffer
	HttpAddress     string             // HTTP server address
	timeoutCancel   context.CancelFunc // Release resources if test completes before the timeout
	StopServer      context.CancelFunc
	WaitForShutdown func() error
	StaticConfig    *config.StaticConfig
	OidcProvider    *oidc.Provider
}

const tokenReviewSuccessful = `
	{
		"kind": "TokenReview",
		"apiVersion": "authentication.k8s.io/v1",
		"spec": {"token": "valid-token"},
		"status": {
			"authenticated": true,
			"user": {
				"username": "test-user",
				"groups": ["system:authenticated"]
			}
		}
	}`

func (c *httpContext) beforeEach(t *testing.T) {
	t.Helper()
	http.DefaultClient.Timeout = 10 * time.Second
	if c.StaticConfig == nil {
		c.StaticConfig = &config.StaticConfig{}
	}
	c.mockServer = test.NewMockServer()
	// Fake Kubernetes configuration
	mockKubeConfig := c.mockServer.KubeConfig()
	kubeConfig := filepath.Join(t.TempDir(), "config")
	_ = clientcmd.WriteToFile(*mockKubeConfig, kubeConfig)
	c.StaticConfig.KubeConfig = kubeConfig
	// Capture logging
	c.klogState = klog.CaptureState()
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	klog.InitFlags(flags)
	_ = flags.Set("v", "5")
	klog.SetLogger(textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(5), textlogger.Output(&c.LogBuffer))))
	// Start server in random port
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("Failed to find random port for HTTP server: %v", err)
	}
	c.HttpAddress = ln.Addr().String()
	if randomPortErr := ln.Close(); randomPortErr != nil {
		t.Fatalf("Failed to close random port listener: %v", randomPortErr)
	}
	c.StaticConfig.Port = fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)
	mcpServer, err := mcp.NewServer(mcp.Configuration{
		Profile:      mcp.Profiles[0],
		StaticConfig: c.StaticConfig,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}
	var timeoutCtx, cancelCtx context.Context
	timeoutCtx, c.timeoutCancel = context.WithTimeout(t.Context(), 10*time.Second)
	group, gc := errgroup.WithContext(timeoutCtx)
	cancelCtx, c.StopServer = context.WithCancel(gc)
	group.Go(func() error { return Serve(cancelCtx, mcpServer, c.StaticConfig, c.OidcProvider) })
	c.WaitForShutdown = group.Wait
	// Wait for HTTP server to start (using net)
	for i := 0; i < 10; i++ {
		conn, err := net.Dial("tcp", c.HttpAddress)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond) // Wait before retrying
	}
}

func (c *httpContext) afterEach(t *testing.T) {
	t.Helper()
	c.mockServer.Close()
	c.StopServer()
	err := c.WaitForShutdown()
	if err != nil {
		t.Errorf("HTTP server did not shut down gracefully: %v", err)
	}
	c.timeoutCancel()
	c.klogState.Restore()
	_ = os.Setenv("KUBECONFIG", "")
}

func testCase(t *testing.T, test func(c *httpContext)) {
	testCaseWithContext(t, &httpContext{}, test)
}

func testCaseWithContext(t *testing.T, httpCtx *httpContext, test func(c *httpContext)) {
	httpCtx.beforeEach(t)
	t.Cleanup(func() { httpCtx.afterEach(t) })
	test(httpCtx)
}

type OidcTestServer struct {
	*rsa.PrivateKey
	*oidc.Provider
	*httptest.Server
	TokenEndpointHandler http.HandlerFunc
}

func NewOidcTestServer(t *testing.T) (oidcTestServer *OidcTestServer) {
	t.Helper()
	var err error
	oidcTestServer = &OidcTestServer{}
	oidcTestServer.PrivateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key for oidc: %v", err)
	}
	oidcServer := &oidctest.Server{
		Algorithms: []string{oidc.RS256, oidc.ES256},
		PublicKeys: []oidctest.PublicKey{
			{
				PublicKey: oidcTestServer.Public(),
				KeyID:     "test-oidc-key-id",
				Algorithm: oidc.RS256,
			},
		},
	}
	oidcTestServer.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" && oidcTestServer.TokenEndpointHandler != nil {
			oidcTestServer.TokenEndpointHandler.ServeHTTP(w, r)
			return
		}
		oidcServer.ServeHTTP(w, r)
	}))
	oidcServer.SetIssuer(oidcTestServer.URL)
	oidcTestServer.Provider, err = oidc.NewProvider(t.Context(), oidcTestServer.URL)
	if err != nil {
		t.Fatalf("failed to create OIDC provider: %v", err)
	}
	return
}

func TestGracefulShutdown(t *testing.T) {
	testCase(t, func(ctx *httpContext) {
		ctx.StopServer()
		err := ctx.WaitForShutdown()
		t.Run("Stops gracefully", func(t *testing.T) {
			if err != nil {
				t.Errorf("Expected graceful shutdown, but got error: %v", err)
			}
		})
		t.Run("Stops on context cancel", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Context cancelled, initiating graceful shutdown") {
				t.Errorf("Context cancelled, initiating graceful shutdown, got: %s", ctx.LogBuffer.String())
			}
		})
		t.Run("Starts server shutdown", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Shutting down HTTP server gracefully") {
				t.Errorf("Expected graceful shutdown log, got: %s", ctx.LogBuffer.String())
			}
		})
		t.Run("Server shutdown completes", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "HTTP server shutdown complete") {
				t.Errorf("Expected HTTP server shutdown completed log, got: %s", ctx.LogBuffer.String())
			}
		})
	})
}

func TestSseTransport(t *testing.T) {
	testCase(t, func(ctx *httpContext) {
		sseResp, sseErr := http.Get(fmt.Sprintf("http://%s/sse", ctx.HttpAddress))
		t.Cleanup(func() { _ = sseResp.Body.Close() })
		t.Run("Exposes SSE endpoint at /sse", func(t *testing.T) {
			if sseErr != nil {
				t.Fatalf("Failed to get SSE endpoint: %v", sseErr)
			}
			if sseResp.StatusCode != http.StatusOK {
				t.Errorf("Expected HTTP 200 OK, got %d", sseResp.StatusCode)
			}
		})
		t.Run("SSE endpoint returns text/event-stream content type", func(t *testing.T) {
			if sseResp.Header.Get("Content-Type") != "text/event-stream" {
				t.Errorf("Expected Content-Type text/event-stream, got %s", sseResp.Header.Get("Content-Type"))
			}
		})
		responseReader := bufio.NewReader(sseResp.Body)
		event, eventErr := responseReader.ReadString('\n')
		endpoint, endpointErr := responseReader.ReadString('\n')
		t.Run("SSE endpoint returns stream with messages endpoint", func(t *testing.T) {
			if eventErr != nil {
				t.Fatalf("Failed to read SSE response body (event): %v", eventErr)
			}
			if event != "event: endpoint\n" {
				t.Errorf("Expected SSE event 'endpoint', got %s", event)
			}
			if endpointErr != nil {
				t.Fatalf("Failed to read SSE response body (endpoint): %v", endpointErr)
			}
			if !strings.HasPrefix(endpoint, "data: /message?sessionId=") {
				t.Errorf("Expected SSE data: '/message', got %s", endpoint)
			}
		})
		messageResp, messageErr := http.Post(
			fmt.Sprintf("http://%s/message?sessionId=%s", ctx.HttpAddress, strings.TrimSpace(endpoint[25:])),
			"application/json",
			bytes.NewBufferString("{}"),
		)
		t.Cleanup(func() { _ = messageResp.Body.Close() })
		t.Run("Exposes message endpoint at /message", func(t *testing.T) {
			if messageErr != nil {
				t.Fatalf("Failed to get message endpoint: %v", messageErr)
			}
			if messageResp.StatusCode != http.StatusAccepted {
				t.Errorf("Expected HTTP 202 OK, got %d", messageResp.StatusCode)
			}
		})
	})
}

func TestStreamableHttpTransport(t *testing.T) {
	testCase(t, func(ctx *httpContext) {
		mcpGetResp, mcpGetErr := http.Get(fmt.Sprintf("http://%s/mcp", ctx.HttpAddress))
		t.Cleanup(func() { _ = mcpGetResp.Body.Close() })
		t.Run("Exposes MCP GET endpoint at /mcp", func(t *testing.T) {
			if mcpGetErr != nil {
				t.Fatalf("Failed to get MCP endpoint: %v", mcpGetErr)
			}
			if mcpGetResp.StatusCode != http.StatusOK {
				t.Errorf("Expected HTTP 200 OK, got %d", mcpGetResp.StatusCode)
			}
		})
		t.Run("MCP GET endpoint returns text/event-stream content type", func(t *testing.T) {
			if mcpGetResp.Header.Get("Content-Type") != "text/event-stream" {
				t.Errorf("Expected Content-Type text/event-stream (GET), got %s", mcpGetResp.Header.Get("Content-Type"))
			}
		})
		mcpPostResp, mcpPostErr := http.Post(fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), "application/json", bytes.NewBufferString("{}"))
		t.Cleanup(func() { _ = mcpPostResp.Body.Close() })
		t.Run("Exposes MCP POST endpoint at /mcp", func(t *testing.T) {
			if mcpPostErr != nil {
				t.Fatalf("Failed to post to MCP endpoint: %v", mcpPostErr)
			}
			if mcpPostResp.StatusCode != http.StatusOK {
				t.Errorf("Expected HTTP 200 OK, got %d", mcpPostResp.StatusCode)
			}
		})
		t.Run("MCP POST endpoint returns application/json content type", func(t *testing.T) {
			if mcpPostResp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type application/json (POST), got %s", mcpPostResp.Header.Get("Content-Type"))
			}
		})
	})
}

func TestHealthCheck(t *testing.T) {
	testCase(t, func(ctx *httpContext) {
		t.Run("Exposes health check endpoint at /healthz", func(t *testing.T) {
			resp, err := http.Get(fmt.Sprintf("http://%s/healthz", ctx.HttpAddress))
			if err != nil {
				t.Fatalf("Failed to get health check endpoint: %v", err)
			}
			t.Cleanup(func() { _ = resp.Body.Close })
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected HTTP 200 OK, got %d", resp.StatusCode)
			}
		})
	})
	// Health exposed even when require Authorization
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, ValidateToken: true}}, func(ctx *httpContext) {
		resp, err := http.Get(fmt.Sprintf("http://%s/healthz", ctx.HttpAddress))
		if err != nil {
			t.Fatalf("Failed to get health check endpoint with OAuth: %v", err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		t.Run("Health check with OAuth returns HTTP 200 OK", func(t *testing.T) {
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected HTTP 200 OK, got %d", resp.StatusCode)
			}
		})
	})
}

func TestWellKnownReverseProxy(t *testing.T) {
	cases := []string{
		".well-known/oauth-authorization-server",
		".well-known/oauth-protected-resource",
		".well-known/openid-configuration",
	}
	// With No Authorization URL configured
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, ValidateToken: true}}, func(ctx *httpContext) {
		for _, path := range cases {
			resp, err := http.Get(fmt.Sprintf("http://%s/%s", ctx.HttpAddress, path))
			t.Cleanup(func() { _ = resp.Body.Close() })
			t.Run("Protected resource '"+path+"' without Authorization URL returns 404 - Not Found", func(t *testing.T) {
				if err != nil {
					t.Fatalf("Failed to get %s endpoint: %v", path, err)
				}
				if resp.StatusCode != http.StatusNotFound {
					t.Errorf("Expected HTTP 404 Not Found, got %d", resp.StatusCode)
				}
			})
		}
	})
	// With Authorization URL configured but invalid payload
	invalidPayloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`NOT A JSON PAYLOAD`))
	}))
	t.Cleanup(invalidPayloadServer.Close)
	invalidPayloadConfig := &config.StaticConfig{AuthorizationURL: invalidPayloadServer.URL, RequireOAuth: true, ValidateToken: true}
	testCaseWithContext(t, &httpContext{StaticConfig: invalidPayloadConfig}, func(ctx *httpContext) {
		for _, path := range cases {
			resp, err := http.Get(fmt.Sprintf("http://%s/%s", ctx.HttpAddress, path))
			t.Cleanup(func() { _ = resp.Body.Close() })
			t.Run("Protected resource '"+path+"' with invalid Authorization URL payload returns 500 - Internal Server Error", func(t *testing.T) {
				if err != nil {
					t.Fatalf("Failed to get %s endpoint: %v", path, err)
				}
				if resp.StatusCode != http.StatusInternalServerError {
					t.Errorf("Expected HTTP 500 Internal Server Error, got %d", resp.StatusCode)
				}
			})
		}
	})
	// With Authorization URL configured and valid payload
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.EscapedPath(), "/.well-known/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer": "https://example.com","scopes_supported":["mcp-server"]}`))
	}))
	t.Cleanup(testServer.Close)
	staticConfig := &config.StaticConfig{AuthorizationURL: testServer.URL, RequireOAuth: true, ValidateToken: true}
	testCaseWithContext(t, &httpContext{StaticConfig: staticConfig}, func(ctx *httpContext) {
		for _, path := range cases {
			resp, err := http.Get(fmt.Sprintf("http://%s/%s", ctx.HttpAddress, path))
			t.Cleanup(func() { _ = resp.Body.Close() })
			t.Run("Exposes "+path+" endpoint", func(t *testing.T) {
				if err != nil {
					t.Fatalf("Failed to get %s endpoint: %v", path, err)
				}
				if resp.StatusCode != http.StatusOK {
					t.Errorf("Expected HTTP 200 OK, got %d", resp.StatusCode)
				}
			})
			t.Run(path+" returns application/json content type", func(t *testing.T) {
				if resp.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type application/json, got %s", resp.Header.Get("Content-Type"))
				}
			})
		}
	})
}

func TestWellKnownOverrides(t *testing.T) {
	cases := []string{
		".well-known/oauth-authorization-server",
		".well-known/oauth-protected-resource",
		".well-known/openid-configuration",
	}
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.EscapedPath(), "/.well-known/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`
			{
				"issuer": "https://localhost",
				"registration_endpoint": "https://localhost/clients-registrations/openid-connect",
				"require_request_uri_registration": true,
				"scopes_supported":["scope-1", "scope-2"]
			}`))
	}))
	t.Cleanup(testServer.Close)
	baseConfig := config.StaticConfig{AuthorizationURL: testServer.URL, RequireOAuth: true, ValidateToken: true}
	// With Dynamic Client Registration disabled
	disableDynamicRegistrationConfig := baseConfig
	disableDynamicRegistrationConfig.DisableDynamicClientRegistration = true
	testCaseWithContext(t, &httpContext{StaticConfig: &disableDynamicRegistrationConfig}, func(ctx *httpContext) {
		for _, path := range cases {
			resp, _ := http.Get(fmt.Sprintf("http://%s/%s", ctx.HttpAddress, path))
			t.Cleanup(func() { _ = resp.Body.Close() })
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}
			t.Run("DisableDynamicClientRegistration removes registration_endpoint field", func(t *testing.T) {
				if strings.Contains(string(body), "registration_endpoint") {
					t.Error("Expected registration_endpoint to be removed, but it was found in the response")
				}
			})
			t.Run("DisableDynamicClientRegistration sets require_request_uri_registration = false", func(t *testing.T) {
				if !strings.Contains(string(body), `"require_request_uri_registration":false`) {
					t.Error("Expected require_request_uri_registration to be false, but it was not found in the response")
				}
			})
			t.Run("DisableDynamicClientRegistration includes/preserves scopes_supported", func(t *testing.T) {
				if !strings.Contains(string(body), `"scopes_supported":["scope-1","scope-2"]`) {
					t.Error("Expected scopes_supported to be present, but it was not found in the response")
				}
			})
		}
	})
	// With overrides for OAuth scopes (client/frontend)
	oAuthScopesConfig := baseConfig
	oAuthScopesConfig.OAuthScopes = []string{"openid", "mcp-server"}
	testCaseWithContext(t, &httpContext{StaticConfig: &oAuthScopesConfig}, func(ctx *httpContext) {
		for _, path := range cases {
			resp, _ := http.Get(fmt.Sprintf("http://%s/%s", ctx.HttpAddress, path))
			t.Cleanup(func() { _ = resp.Body.Close() })
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}
			t.Run("OAuthScopes overrides scopes_supported", func(t *testing.T) {
				if !strings.Contains(string(body), `"scopes_supported":["openid","mcp-server"]`) {
					t.Errorf("Expected scopes_supported to be overridden, but original was preserved, response: %s", string(body))
				}
			})
			t.Run("OAuthScopes preserves other fields", func(t *testing.T) {
				if !strings.Contains(string(body), `"issuer":"https://localhost"`) {
					t.Errorf("Expected issuer to be preserved, but got: %s", string(body))
				}
				if !strings.Contains(string(body), `"registration_endpoint":"https://localhost`) {
					t.Errorf("Expected registration_endpoint to be preserved, but got: %s", string(body))
				}
				if !strings.Contains(string(body), `"require_request_uri_registration":true`) {
					t.Error("Expected require_request_uri_registration to be true, but it was not found in the response")
				}
			})
		}
	})
}

func TestMiddlewareLogging(t *testing.T) {
	testCase(t, func(ctx *httpContext) {
		_, _ = http.Get(fmt.Sprintf("http://%s/.well-known/oauth-protected-resource", ctx.HttpAddress))
		t.Run("Logs HTTP requests and responses", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "GET /.well-known/oauth-protected-resource 404") {
				t.Errorf("Expected log entry for GET /.well-known/oauth-protected-resource, got: %s", ctx.LogBuffer.String())
			}
		})
		t.Run("Logs HTTP request duration", func(t *testing.T) {
			expected := `"GET /.well-known/oauth-protected-resource 404 (.+)"`
			m := regexp.MustCompile(expected).FindStringSubmatch(ctx.LogBuffer.String())
			if len(m) != 2 {
				t.Fatalf("Expected log entry to contain duration, got %s", ctx.LogBuffer.String())
			}
			duration, err := time.ParseDuration(m[1])
			if err != nil {
				t.Fatalf("Failed to parse duration from log entry: %v", err)
			}
			if duration < 0 {
				t.Errorf("Expected duration to be non-negative, got %v", duration)
			}
		})
	})
}

func TestAuthorizationUnauthorized(t *testing.T) {
	// Missing Authorization header
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, ValidateToken: true}}, func(ctx *httpContext) {
		resp, err := http.Get(fmt.Sprintf("http://%s/mcp", ctx.HttpAddress))
		if err != nil {
			t.Fatalf("Failed to get protected endpoint: %v", err)
		}
		t.Cleanup(func() { _ = resp.Body.Close })
		t.Run("Protected resource with MISSING Authorization header returns 401 - Unauthorized", func(t *testing.T) {
			if resp.StatusCode != 401 {
				t.Errorf("Expected HTTP 401, got %d", resp.StatusCode)
			}
		})
		t.Run("Protected resource with MISSING Authorization header returns WWW-Authenticate header", func(t *testing.T) {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", error="missing_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with MISSING Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - missing or invalid bearer token") {
				t.Errorf("Expected log entry for missing or invalid bearer token, got: %s", ctx.LogBuffer.String())
			}
		})
	})
	// Authorization header without Bearer prefix
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, ValidateToken: true}}, func(ctx *httpContext) {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Basic YWxhZGRpbjpvcGVuc2VzYW1l")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to get protected endpoint: %v", err)
		}
		t.Cleanup(func() { _ = resp.Body.Close })
		t.Run("Protected resource with INCOMPATIBLE Authorization header returns WWW-Authenticate header", func(t *testing.T) {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", error="missing_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with INCOMPATIBLE Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - missing or invalid bearer token") {
				t.Errorf("Expected log entry for missing or invalid bearer token, got: %s", ctx.LogBuffer.String())
			}
		})
	})
	// Invalid Authorization header
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, ValidateToken: true}}, func(ctx *httpContext) {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+strings.ReplaceAll(tokenBasicNotExpired, ".", ".invalid"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to get protected endpoint: %v", err)
		}
		t.Cleanup(func() { _ = resp.Body.Close })
		t.Run("Protected resource with INVALID Authorization header returns 401 - Unauthorized", func(t *testing.T) {
			if resp.StatusCode != 401 {
				t.Errorf("Expected HTTP 401, got %d", resp.StatusCode)
			}
		})
		t.Run("Protected resource with INVALID Authorization header returns WWW-Authenticate header", func(t *testing.T) {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", error="invalid_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with INVALID Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - JWT validation error") ||
				!strings.Contains(ctx.LogBuffer.String(), "error: failed to parse JWT token: illegal base64 data") {
				t.Errorf("Expected log entry for JWT validation error, got: %s", ctx.LogBuffer.String())
			}
		})
	})
	// Expired Authorization Bearer token
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, ValidateToken: true}}, func(ctx *httpContext) {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+tokenBasicExpired)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to get protected endpoint: %v", err)
		}
		t.Cleanup(func() { _ = resp.Body.Close })
		t.Run("Protected resource with EXPIRED Authorization header returns 401 - Unauthorized", func(t *testing.T) {
			if resp.StatusCode != 401 {
				t.Errorf("Expected HTTP 401, got %d", resp.StatusCode)
			}
		})
		t.Run("Protected resource with EXPIRED Authorization header returns WWW-Authenticate header", func(t *testing.T) {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", error="invalid_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with EXPIRED Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - JWT validation error") ||
				!strings.Contains(ctx.LogBuffer.String(), "validation failed, token is expired (exp)") {
				t.Errorf("Expected log entry for JWT validation error, got: %s", ctx.LogBuffer.String())
			}
		})
	})
	// Invalid audience claim Bearer token
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, OAuthAudience: "expected-audience", ValidateToken: true}}, func(ctx *httpContext) {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+tokenBasicExpired)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to get protected endpoint: %v", err)
		}
		t.Cleanup(func() { _ = resp.Body.Close })
		t.Run("Protected resource with INVALID AUDIENCE Authorization header returns 401 - Unauthorized", func(t *testing.T) {
			if resp.StatusCode != 401 {
				t.Errorf("Expected HTTP 401, got %d", resp.StatusCode)
			}
		})
		t.Run("Protected resource with INVALID AUDIENCE Authorization header returns WWW-Authenticate header", func(t *testing.T) {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", audience="expected-audience", error="invalid_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with INVALID AUDIENCE Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - JWT validation error") ||
				!strings.Contains(ctx.LogBuffer.String(), "invalid audience claim (aud)") {
				t.Errorf("Expected log entry for JWT validation error, got: %s", ctx.LogBuffer.String())
			}
		})
	})
	// Failed OIDC validation
	oidcTestServer := NewOidcTestServer(t)
	t.Cleanup(oidcTestServer.Close)
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, OAuthAudience: "mcp-server", ValidateToken: true}, OidcProvider: oidcTestServer.Provider}, func(ctx *httpContext) {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+tokenBasicNotExpired)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to get protected endpoint: %v", err)
		}
		t.Cleanup(func() { _ = resp.Body.Close })
		t.Run("Protected resource with INVALID OIDC Authorization header returns 401 - Unauthorized", func(t *testing.T) {
			if resp.StatusCode != 401 {
				t.Errorf("Expected HTTP 401, got %d", resp.StatusCode)
			}
		})
		t.Run("Protected resource with INVALID OIDC Authorization header returns WWW-Authenticate header", func(t *testing.T) {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", audience="mcp-server", error="invalid_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with INVALID OIDC Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - JWT validation error") ||
				!strings.Contains(ctx.LogBuffer.String(), "OIDC token validation error: failed to verify signature") {
				t.Errorf("Expected log entry for OIDC validation error, got: %s", ctx.LogBuffer.String())
			}
		})
	})
	// Failed Kubernetes TokenReview
	rawClaims := `{
		"iss": "` + oidcTestServer.URL + `",
		"exp": ` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `,
		"aud": "mcp-server"
	}`
	validOidcToken := oidctest.SignIDToken(oidcTestServer.PrivateKey, "test-oidc-key-id", oidc.RS256, rawClaims)
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, OAuthAudience: "mcp-server", ValidateToken: true}, OidcProvider: oidcTestServer.Provider}, func(ctx *httpContext) {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+validOidcToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to get protected endpoint: %v", err)
		}
		t.Cleanup(func() { _ = resp.Body.Close })
		t.Run("Protected resource with INVALID KUBERNETES Authorization header returns 401 - Unauthorized", func(t *testing.T) {
			if resp.StatusCode != 401 {
				t.Errorf("Expected HTTP 401, got %d", resp.StatusCode)
			}
		})
		t.Run("Protected resource with INVALID KUBERNETES Authorization header returns WWW-Authenticate header", func(t *testing.T) {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", audience="mcp-server", error="invalid_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with INVALID KUBERNETES Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - JWT validation error") ||
				!strings.Contains(ctx.LogBuffer.String(), "kubernetes API token validation error: failed to create token review") {
				t.Errorf("Expected log entry for Kubernetes TokenReview error, got: %s", ctx.LogBuffer.String())
			}
		})
	})
}

func TestAuthorizationRequireOAuthFalse(t *testing.T) {
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: false}}, func(ctx *httpContext) {
		resp, err := http.Get(fmt.Sprintf("http://%s/mcp", ctx.HttpAddress))
		if err != nil {
			t.Fatalf("Failed to get protected endpoint: %v", err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		t.Run("Protected resource with MISSING Authorization header returns 200 - OK)", func(t *testing.T) {
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected HTTP 200 OK, got %d", resp.StatusCode)
			}
		})
	})
}

func TestAuthorizationRawToken(t *testing.T) {
	cases := []struct {
		audience      string
		validateToken bool
	}{
		{"", false},           // No audience, no validation
		{"", true},            // No audience, validation enabled
		{"mcp-server", false}, // Audience set, no validation
		{"mcp-server", true},  // Audience set, validation enabled
	}
	for _, c := range cases {
		testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, OAuthAudience: c.audience, ValidateToken: c.validateToken}}, func(ctx *httpContext) {
			tokenReviewed := false
			ctx.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.EscapedPath() == "/apis/authentication.k8s.io/v1/tokenreviews" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tokenReviewSuccessful))
					tokenReviewed = true
					return
				}
			}))
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+tokenBasicNotExpired)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to get protected endpoint: %v", err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			t.Run(fmt.Sprintf("Protected resource with audience = '%s' and validate-token = '%t', with VALID Authorization header returns 200 - OK", c.audience, c.validateToken), func(t *testing.T) {
				if resp.StatusCode != http.StatusOK {
					t.Errorf("Expected HTTP 200 OK, got %d", resp.StatusCode)
				}
			})
			t.Run(fmt.Sprintf("Protected resource with audience = '%s' and validate-token = '%t', with VALID Authorization header performs token validation accordingly", c.audience, c.validateToken), func(t *testing.T) {
				if tokenReviewed == true && !c.validateToken {
					t.Errorf("Expected token review to be skipped when validate-token is false, but it was performed")
				}
				if tokenReviewed == false && c.validateToken {
					t.Errorf("Expected token review to be performed when validate-token is true, but it was skipped")
				}
			})
		})
	}

}

func TestAuthorizationOidcToken(t *testing.T) {
	oidcTestServer := NewOidcTestServer(t)
	t.Cleanup(oidcTestServer.Close)
	rawClaims := `{
		"iss": "` + oidcTestServer.URL + `",
		"exp": ` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `,
		"aud": "mcp-server"
	}`
	validOidcToken := oidctest.SignIDToken(oidcTestServer.PrivateKey, "test-oidc-key-id", oidc.RS256, rawClaims)
	cases := []bool{false, true}
	for _, validateToken := range cases {
		testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, OAuthAudience: "mcp-server", ValidateToken: validateToken}, OidcProvider: oidcTestServer.Provider}, func(ctx *httpContext) {
			tokenReviewed := false
			ctx.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.EscapedPath() == "/apis/authentication.k8s.io/v1/tokenreviews" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tokenReviewSuccessful))
					tokenReviewed = true
					return
				}
			}))
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+validOidcToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to get protected endpoint: %v", err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			t.Run(fmt.Sprintf("Protected resource with validate-token='%t' with VALID OIDC Authorization header returns 200 - OK", validateToken), func(t *testing.T) {
				if resp.StatusCode != http.StatusOK {
					t.Errorf("Expected HTTP 200 OK, got %d", resp.StatusCode)
				}
			})
			t.Run(fmt.Sprintf("Protected resource with validate-token='%t' with VALID OIDC Authorization header performs token validation accordingly", validateToken), func(t *testing.T) {
				if tokenReviewed == true && !validateToken {
					t.Errorf("Expected token review to be skipped when validate-token is false, but it was performed")
				}
				if tokenReviewed == false && validateToken {
					t.Errorf("Expected token review to be performed when validate-token is true, but it was skipped")
				}
			})
		})
	}
}

func TestAuthorizationOidcTokenExchange(t *testing.T) {
	oidcTestServer := NewOidcTestServer(t)
	t.Cleanup(oidcTestServer.Close)
	rawClaims := `{
		"iss": "` + oidcTestServer.URL + `",
		"exp": ` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `,
		"aud": "%s"
	}`
	validOidcClientToken := oidctest.SignIDToken(oidcTestServer.PrivateKey, "test-oidc-key-id", oidc.RS256,
		fmt.Sprintf(rawClaims, "mcp-server"))
	validOidcBackendToken := oidctest.SignIDToken(oidcTestServer.PrivateKey, "test-oidc-key-id", oidc.RS256,
		fmt.Sprintf(rawClaims, "backend-audience"))
	oidcTestServer.TokenEndpointHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"%s","token_type":"Bearer","expires_in":253402297199}`, validOidcBackendToken)
	}
	cases := []bool{false, true}
	for _, validateToken := range cases {
		staticConfig := &config.StaticConfig{
			RequireOAuth:    true,
			OAuthAudience:   "mcp-server",
			ValidateToken:   validateToken,
			StsClientId:     "test-sts-client-id",
			StsClientSecret: "test-sts-client-secret",
			StsAudience:     "backend-audience",
			StsScopes:       []string{"backend-scope"},
		}
		testCaseWithContext(t, &httpContext{StaticConfig: staticConfig, OidcProvider: oidcTestServer.Provider}, func(ctx *httpContext) {
			tokenReviewed := false
			ctx.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.EscapedPath() == "/apis/authentication.k8s.io/v1/tokenreviews" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tokenReviewSuccessful))
					tokenReviewed = true
					return
				}
			}))
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+validOidcClientToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to get protected endpoint: %v", err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			t.Run(fmt.Sprintf("Protected resource with validate-token='%t' with VALID OIDC EXCHANGE Authorization header returns 200 - OK", validateToken), func(t *testing.T) {
				if resp.StatusCode != http.StatusOK {
					t.Errorf("Expected HTTP 200 OK, got %d", resp.StatusCode)
				}
			})
			t.Run(fmt.Sprintf("Protected resource with validate-token='%t' with VALID OIDC EXCHANGE Authorization header performs token validation accordingly", validateToken), func(t *testing.T) {
				if tokenReviewed == true && !validateToken {
					t.Errorf("Expected token review to be skipped when validate-token is false, but it was performed")
				}
				if tokenReviewed == false && validateToken {
					t.Errorf("Expected token review to be performed when validate-token is true, but it was skipped")
				}
			})
		})
	}
}
