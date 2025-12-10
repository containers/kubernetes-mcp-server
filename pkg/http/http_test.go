package http

import (
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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/errgroup"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"

	configapi "github.com/containers/kubernetes-mcp-server/pkg/api/config"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
)

type BaseHttpSuite struct {
	suite.Suite
	MockServer      *test.MockServer
	StaticConfig    *config.StaticConfig
	mcpServer       *mcp.Server
	OidcProvider    *oidc.Provider
	timeoutCancel   context.CancelFunc
	StopServer      context.CancelFunc
	WaitForShutdown func() error
}

func (s *BaseHttpSuite) SetupTest() {
	http.DefaultClient.Timeout = 10 * time.Second
	s.MockServer = test.NewMockServer()
	s.MockServer.Handle(&test.DiscoveryClientHandler{})
	s.StaticConfig = config.Default()
	s.StaticConfig.KubeConfig = s.MockServer.KubeconfigFile(s.T())
}

func (s *BaseHttpSuite) StartServer() {

	tcpAddr, err := test.RandomPortAddress()
	s.Require().NoError(err, "Expected no error getting random port address")
	s.StaticConfig.Port = strconv.Itoa(tcpAddr.Port)

	s.mcpServer, err = mcp.NewServer(mcp.Configuration{StaticConfig: s.StaticConfig})
	s.Require().NoError(err, "Expected no error creating MCP server")
	s.Require().NotNil(s.mcpServer, "MCP server should not be nil")
	var timeoutCtx, cancelCtx context.Context
	timeoutCtx, s.timeoutCancel = context.WithTimeout(s.T().Context(), 10*time.Second)
	group, gc := errgroup.WithContext(timeoutCtx)
	cancelCtx, s.StopServer = context.WithCancel(gc)
	group.Go(func() error { return Serve(cancelCtx, s.mcpServer, s.StaticConfig, s.OidcProvider, nil) })
	s.WaitForShutdown = group.Wait
	s.Require().NoError(test.WaitForServer(tcpAddr), "HTTP server did not start in time")
	s.Require().NoError(test.WaitForHealthz(tcpAddr), "HTTP server /healthz endpoint did not respond with non-404 in time")
}

func (s *BaseHttpSuite) TearDownTest() {
	s.MockServer.Close()
	if s.mcpServer != nil {
		s.mcpServer.Close()
	}
	s.StopServer()
	s.Require().NoError(s.WaitForShutdown(), "HTTP server did not shut down gracefully")
	s.timeoutCancel()
}

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

func (c *httpContext) beforeEach(t *testing.T) {
	t.Helper()
	http.DefaultClient.Timeout = 10 * time.Second
	if c.StaticConfig == nil {
		c.StaticConfig = config.Default()
	}
	c.mockServer = test.NewMockServer()
	// Fake Kubernetes configuration
	c.StaticConfig.KubeConfig = c.mockServer.KubeconfigFile(t)
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
	mcpServer, err := mcp.NewServer(mcp.Configuration{StaticConfig: c.StaticConfig})
	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}
	var timeoutCtx, cancelCtx context.Context
	timeoutCtx, c.timeoutCancel = context.WithTimeout(t.Context(), 10*time.Second)
	group, gc := errgroup.WithContext(timeoutCtx)
	cancelCtx, c.StopServer = context.WithCancel(gc)
	group.Go(func() error { return Serve(cancelCtx, mcpServer, c.StaticConfig, c.OidcProvider, nil) })
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
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, ValidateToken: true, ClusterProviderStrategy: configapi.ClusterProviderKubeConfig}}, func(ctx *httpContext) {
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
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, ValidateToken: true, ClusterProviderStrategy: configapi.ClusterProviderKubeConfig}}, func(ctx *httpContext) {
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
	invalidPayloadConfig := &config.StaticConfig{
		AuthorizationURL:        invalidPayloadServer.URL,
		RequireOAuth:            true,
		ValidateToken:           true,
		ClusterProviderStrategy: configapi.ClusterProviderKubeConfig,
	}
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
	staticConfig := &config.StaticConfig{
		AuthorizationURL:        testServer.URL,
		RequireOAuth:            true,
		ValidateToken:           true,
		ClusterProviderStrategy: configapi.ClusterProviderKubeConfig,
	}
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

func TestWellKnownHeaderPropagation(t *testing.T) {
	cases := []string{
		".well-known/oauth-authorization-server",
		".well-known/oauth-protected-resource",
		".well-known/openid-configuration",
	}
	var receivedRequestHeaders http.Header
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.EscapedPath(), "/.well-known/") {
			http.NotFound(w, r)
			return
		}
		// Capture headers received from the proxy
		receivedRequestHeaders = r.Header.Clone()
		// Set response headers that should be propagated back
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "https://example.com")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Custom-Backend-Header", "backend-value")
		_, _ = w.Write([]byte(`{"issuer": "https://example.com"}`))
	}))
	t.Cleanup(testServer.Close)
	staticConfig := &config.StaticConfig{
		AuthorizationURL:        testServer.URL,
		RequireOAuth:            true,
		ValidateToken:           true,
		ClusterProviderStrategy: configapi.ClusterProviderKubeConfig,
	}
	testCaseWithContext(t, &httpContext{StaticConfig: staticConfig}, func(ctx *httpContext) {
		for _, path := range cases {
			receivedRequestHeaders = nil
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/%s", ctx.HttpAddress, path), nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			// Add various headers to test propagation
			req.Header.Set("Origin", "https://example.com")
			req.Header.Set("User-Agent", "Test-Agent/1.0")
			req.Header.Set("Accept", "application/json")
			req.Header.Set("Accept-Language", "en-US")
			req.Header.Set("X-Custom-Header", "custom-value")
			req.Header.Set("Referer", "https://example.com/page")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to get %s endpoint: %v", path, err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })

			t.Run("Well-known proxy propagates Origin header to backend for "+path, func(t *testing.T) {
				if receivedRequestHeaders == nil {
					t.Fatal("Backend did not receive any headers")
				}
				if receivedRequestHeaders.Get("Origin") != "https://example.com" {
					t.Errorf("Expected Origin header 'https://example.com', got '%s'", receivedRequestHeaders.Get("Origin"))
				}
			})

			t.Run("Well-known proxy propagates User-Agent header to backend for "+path, func(t *testing.T) {
				if receivedRequestHeaders.Get("User-Agent") != "Test-Agent/1.0" {
					t.Errorf("Expected User-Agent header 'Test-Agent/1.0', got '%s'", receivedRequestHeaders.Get("User-Agent"))
				}
			})

			t.Run("Well-known proxy propagates Accept header to backend for "+path, func(t *testing.T) {
				if receivedRequestHeaders.Get("Accept") != "application/json" {
					t.Errorf("Expected Accept header 'application/json', got '%s'", receivedRequestHeaders.Get("Accept"))
				}
			})

			t.Run("Well-known proxy propagates Accept-Language header to backend for "+path, func(t *testing.T) {
				if receivedRequestHeaders.Get("Accept-Language") != "en-US" {
					t.Errorf("Expected Accept-Language header 'en-US', got '%s'", receivedRequestHeaders.Get("Accept-Language"))
				}
			})

			t.Run("Well-known proxy propagates custom headers to backend for "+path, func(t *testing.T) {
				if receivedRequestHeaders.Get("X-Custom-Header") != "custom-value" {
					t.Errorf("Expected X-Custom-Header 'custom-value', got '%s'", receivedRequestHeaders.Get("X-Custom-Header"))
				}
			})

			t.Run("Well-known proxy propagates Referer header to backend for "+path, func(t *testing.T) {
				if receivedRequestHeaders.Get("Referer") != "https://example.com/page" {
					t.Errorf("Expected Referer header 'https://example.com/page', got '%s'", receivedRequestHeaders.Get("Referer"))
				}
			})

			t.Run("Well-known proxy returns Access-Control-Allow-Origin from backend for "+path, func(t *testing.T) {
				if resp.Header.Get("Access-Control-Allow-Origin") != "https://example.com" {
					t.Errorf("Expected Access-Control-Allow-Origin header 'https://example.com', got '%s'", resp.Header.Get("Access-Control-Allow-Origin"))
				}
			})

			t.Run("Well-known proxy returns Access-Control-Allow-Methods from backend for "+path, func(t *testing.T) {
				if resp.Header.Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" {
					t.Errorf("Expected Access-Control-Allow-Methods header 'GET, POST, OPTIONS', got '%s'", resp.Header.Get("Access-Control-Allow-Methods"))
				}
			})

			t.Run("Well-known proxy returns Cache-Control from backend for "+path, func(t *testing.T) {
				if resp.Header.Get("Cache-Control") != "no-cache" {
					t.Errorf("Expected Cache-Control header 'no-cache', got '%s'", resp.Header.Get("Cache-Control"))
				}
			})

			t.Run("Well-known proxy returns custom response headers from backend for "+path, func(t *testing.T) {
				if resp.Header.Get("X-Custom-Backend-Header") != "backend-value" {
					t.Errorf("Expected X-Custom-Backend-Header 'backend-value', got '%s'", resp.Header.Get("X-Custom-Backend-Header"))
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
	baseConfig := config.StaticConfig{
		AuthorizationURL:        testServer.URL,
		RequireOAuth:            true,
		ValidateToken:           true,
		ClusterProviderStrategy: configapi.ClusterProviderKubeConfig,
	}
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
