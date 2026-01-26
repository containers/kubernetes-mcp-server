package http

import (
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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/errgroup"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"

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
	s.MockServer.Handle(test.NewDiscoveryClientHandler())
	s.StaticConfig = config.Default()
	s.StaticConfig.KubeConfig = s.MockServer.KubeconfigFile(s.T())
}

func (s *BaseHttpSuite) StartServer() {

	tcpAddr, err := test.RandomPortAddress()
	s.Require().NoError(err, "Expected no error getting random port address")
	s.StaticConfig.Port = strconv.Itoa(tcpAddr.Port)

	provider, err := kubernetes.NewProvider(s.StaticConfig, kubernetes.WithTokenExchange(s.OidcProvider, nil))
	s.Require().NoError(err, "Expected no error creating kubernetes target provider")
	s.mcpServer, err = mcp.NewServer(mcp.Configuration{StaticConfig: s.StaticConfig}, provider)
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
	provider, err := kubernetes.NewProvider(c.StaticConfig, kubernetes.WithTokenExchange(c.OidcProvider, nil))
	if err != nil {
		t.Fatalf("Failed to create kubernetes target provider: %v", err)
	}
	mcpServer, err := mcp.NewServer(mcp.Configuration{StaticConfig: c.StaticConfig}, provider)
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
	testCaseWithContext(t, &httpContext{StaticConfig: &config.StaticConfig{RequireOAuth: true, ClusterProviderStrategy: api.ClusterProviderKubeConfig}}, func(ctx *httpContext) {
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
