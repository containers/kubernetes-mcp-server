package http

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"flag"
	"fmt"
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

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
)

type httpContext struct {
	klogState       klog.State
	LogBuffer       bytes.Buffer
	HttpAddress     string             // HTTP server address
	timeoutCancel   context.CancelFunc // Release resources if test completes before the timeout
	StopServer      context.CancelFunc
	WaitForShutdown func() error
	StaticConfig    *config.StaticConfig
	OidcProvider    *oidc.Provider
}

func (c *httpContext) beforeEach(t *testing.T) {
	t.Helper()
	http.DefaultClient.Timeout = 10 * time.Second
	if c.StaticConfig == nil {
		c.StaticConfig = &config.StaticConfig{}
	}
	// Fake Kubernetes configuration
	fakeConfig := api.NewConfig()
	fakeConfig.Clusters["fake"] = api.NewCluster()
	fakeConfig.Clusters["fake"].Server = "https://example.com"
	fakeConfig.Contexts["fake-context"] = api.NewContext()
	fakeConfig.Contexts["fake-context"].Cluster = "fake"
	fakeConfig.CurrentContext = "fake-context"
	kubeConfig := filepath.Join(t.TempDir(), "config")
	_ = clientcmd.WriteToFile(*fakeConfig, kubeConfig)
	_ = os.Setenv("KUBECONFIG", kubeConfig)
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

func NewOidcTestServer(t *testing.T) (privateKey *rsa.PrivateKey, oidcProvider *oidc.Provider, httpServer *httptest.Server) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key for oidc: %v", err)
	}
	oidcServer := &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{
			{
				PublicKey: privateKey.Public(),
				KeyID:     "test-oidc-key-id",
				Algorithm: oidc.RS256,
			},
		},
	}
	httpServer = httptest.NewServer(oidcServer)
	oidcServer.SetIssuer(httpServer.URL)
	oidcProvider, err = oidc.NewProvider(t.Context(), httpServer.URL)
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
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true}}, func(ctx *httpContext) {
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

func TestWellKnownOAuthProtectedResource(t *testing.T) {
	testCase(t, func(ctx *httpContext) {
		resp, err := http.Get(fmt.Sprintf("http://%s/.well-known/oauth-protected-resource", ctx.HttpAddress))
		t.Cleanup(func() { _ = resp.Body.Close() })
		t.Run("Exposes .well-known/oauth-protected-resource endpoint", func(t *testing.T) {
			if err != nil {
				t.Fatalf("Failed to get .well-known/oauth-protected-resource endpoint: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected HTTP 200 OK, got %d", resp.StatusCode)
			}
		})
		t.Run(".well-known/oauth-protected-resource returns application/json content type", func(t *testing.T) {
			if resp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", resp.Header.Get("Content-Type"))
			}
		})
	})
}

func TestMiddlewareLogging(t *testing.T) {
	testCase(t, func(ctx *httpContext) {
		_, _ = http.Get(fmt.Sprintf("http://%s/.well-known/oauth-protected-resource", ctx.HttpAddress))
		t.Run("Logs HTTP requests and responses", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "GET /.well-known/oauth-protected-resource 200") {
				t.Errorf("Expected log entry for GET /.well-known/oauth-protected-resource, got: %s", ctx.LogBuffer.String())
			}
		})
		t.Run("Logs HTTP request duration", func(t *testing.T) {
			expected := `"GET /.well-known/oauth-protected-resource 200 (.+)"`
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
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true}}, func(ctx *httpContext) {
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
			expected := `Bearer realm="Kubernetes MCP Server", audience="kubernetes-mcp-server", error="missing_token"`
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
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true}}, func(ctx *httpContext) {
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
			expected := `Bearer realm="Kubernetes MCP Server", audience="kubernetes-mcp-server", error="missing_token"`
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
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true}}, func(ctx *httpContext) {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/mcp", ctx.HttpAddress), nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer invalid_base64"+tokenBasicNotExpired)
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
			expected := `Bearer realm="Kubernetes MCP Server", audience="kubernetes-mcp-server", error="invalid_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with INVALID Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - JWT validation error") &&
				!strings.Contains(ctx.LogBuffer.String(), "error: failed to parse JWT token: illegal base64 data") {
				t.Errorf("Expected log entry for JWT validation error, got: %s", ctx.LogBuffer.String())
			}
		})
	})
	// Expired Authorization Bearer token
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true}}, func(ctx *httpContext) {
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
			expected := `Bearer realm="Kubernetes MCP Server", audience="kubernetes-mcp-server", error="invalid_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with EXPIRED Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - JWT validation error") &&
				!strings.Contains(ctx.LogBuffer.String(), "validation failed, token is expired (exp)") {
				t.Errorf("Expected log entry for JWT validation error, got: %s", ctx.LogBuffer.String())
			}
		})
	})
	// Failed OIDC validation
	key, oidcProvider, httpServer := NewOidcTestServer(t)
	t.Cleanup(httpServer.Close)
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true}, OidcProvider: oidcProvider}, func(ctx *httpContext) {
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
			expected := `Bearer realm="Kubernetes MCP Server", audience="kubernetes-mcp-server", error="invalid_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with INVALID OIDC Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - OIDC token validation error") &&
				!strings.Contains(ctx.LogBuffer.String(), "JWT token verification failed: oidc: id token issued by a different provider") {
				t.Errorf("Expected log entry for OIDC validation error, got: %s", ctx.LogBuffer.String())
			}
		})
	})
	// Failed Kubernetes TokenReview
	rawClaims := `{
		"iss": "` + httpServer.URL + `",
		"exp": ` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `,
		"aud": "kubernetes-mcp-server"
	}`
	validOidcToken := oidctest.SignIDToken(key, "test-oidc-key-id", oidc.RS256, rawClaims)
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true}, OidcProvider: oidcProvider}, func(ctx *httpContext) {
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
			expected := `Bearer realm="Kubernetes MCP Server", audience="kubernetes-mcp-server", error="invalid_token"`
			if authHeader != expected {
				t.Errorf("Expected WWW-Authenticate header to be %q, got %q", expected, authHeader)
			}
		})
		t.Run("Protected resource with INVALID KUBERNETES Authorization header logs error", func(t *testing.T) {
			if !strings.Contains(ctx.LogBuffer.String(), "Authentication failed - API Server token validation error") {
				t.Errorf("Expected log entry for Kubernetes TokenReview error, got: %s", ctx.LogBuffer.String())
			}
		})
	})
}
