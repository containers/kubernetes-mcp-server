package mcp

import (
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/internal/test"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/stretchr/testify/suite"
)

type McpHeadersSuite struct {
	BaseMcpSuite
	mockServer     *test.MockServer
	pathHeaders    map[string]http.Header
	pathHeadersMux sync.Mutex
}

func (s *McpHeadersSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
	s.pathHeaders = make(map[string]http.Header)
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		s.pathHeadersMux.Lock()
		s.pathHeaders[req.URL.Path] = req.Header.Clone()
		s.pathHeadersMux.Unlock()
	}))
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Request Performed by DynamicClient
		if req.URL.Path == "/api/v1/namespaces/default/pods" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"kind":"PodList","apiVersion":"v1","items":[]}`))
			return
		}
		// Request Performed by kubernetes.Interface
		if req.URL.Path == "/api/v1/namespaces/default/pods/a-pod-to-delete" {
			w.WriteHeader(200)
			return
		}
	}))
}

func (s *McpHeadersSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *McpHeadersSuite) TestAuthorizationHeaderPropagation() {
	cases := []string{"kubernetes-authorization", "Authorization"}
	for _, header := range cases {
		s.InitMcpClient(test.WithTransport(transport.WithHTTPHeaders(map[string]string{header: "Bearer a-token-from-mcp-client"})))
		_, _ = s.CallTool("pods_list", map[string]interface{}{})
		s.pathHeadersMux.Lock()
		pathHeadersLen := len(s.pathHeaders)
		s.pathHeadersMux.Unlock()
		s.Require().Greater(pathHeadersLen, 0, "No requests were made to Kube API")
		s.Run("DiscoveryClient propagates "+header+" header to Kube API", func() {
			s.pathHeadersMux.Lock()
			apiHeaders := s.pathHeaders["/api"]
			apisHeaders := s.pathHeaders["/apis"]
			apiV1Headers := s.pathHeaders["/api/v1"]
			s.pathHeadersMux.Unlock()

			s.Require().NotNil(apiHeaders, "No requests were made to /api")
			s.Equal("Bearer a-token-from-mcp-client", apiHeaders.Get("Authorization"), "Overridden header Authorization not found in request to /api")
			s.Require().NotNil(apisHeaders, "No requests were made to /apis")
			s.Equal("Bearer a-token-from-mcp-client", apisHeaders.Get("Authorization"), "Overridden header Authorization not found in request to /apis")
			s.Require().NotNil(apiV1Headers, "No requests were made to /api/v1")
			s.Equal("Bearer a-token-from-mcp-client", apiV1Headers.Get("Authorization"), "Overridden header Authorization not found in request to /api/v1")
		})
		s.Run("DynamicClient propagates "+header+" header to Kube API", func() {
			s.pathHeadersMux.Lock()
			podsHeaders := s.pathHeaders["/api/v1/namespaces/default/pods"]
			s.pathHeadersMux.Unlock()

			s.Require().NotNil(podsHeaders, "No requests were made to /api/v1/namespaces/default/pods")
			s.Equal("Bearer a-token-from-mcp-client", podsHeaders.Get("Authorization"), "Overridden header Authorization not found in request to /api/v1/namespaces/default/pods")
		})
		_, _ = s.CallTool("pods_delete", map[string]interface{}{"name": "a-pod-to-delete"})
		s.Run("kubernetes.Interface propagates "+header+" header to Kube API", func() {
			s.pathHeadersMux.Lock()
			podDeleteHeaders := s.pathHeaders["/api/v1/namespaces/default/pods/a-pod-to-delete"]
			s.pathHeadersMux.Unlock()

			s.Require().NotNil(podDeleteHeaders, "No requests were made to /api/v1/namespaces/default/pods/a-pod-to-delete")
			s.Equal("Bearer a-token-from-mcp-client", podDeleteHeaders.Get("Authorization"), "Overridden header Authorization not found in request to /api/v1/namespaces/default/pods/a-pod-to-delete")
		})

	}
}

func TestMcpHeaders(t *testing.T) {
	suite.Run(t, new(McpHeadersSuite))
}

type ServerInstructionsSuite struct {
	BaseMcpSuite
}

func (s *ServerInstructionsSuite) TestServerInstructionsEmpty() {
	s.InitMcpClient()
	s.Run("returns empty instructions when not configured", func() {
		s.Require().NotNil(s.InitializeResult)
		s.Empty(s.InitializeResult.Instructions, "instructions should be empty when not configured")
	})
}

func (s *ServerInstructionsSuite) TestServerInstructionsFromConfiguration() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		server_instructions = "Always use YAML output format for kubectl commands."
	`), s.Cfg), "Expected to parse server instructions config")
	s.InitMcpClient()
	s.Run("returns configured instructions", func() {
		s.Require().NotNil(s.InitializeResult)
		s.Equal("Always use YAML output format for kubectl commands.", s.InitializeResult.Instructions,
			"instructions should match configured value")
	})
}

func TestServerInstructions(t *testing.T) {
	suite.Run(t, new(ServerInstructionsSuite))
}

type UserAgentPropagationSuite struct {
	BaseMcpSuite
	mockServer     *test.MockServer
	pathHeaders    map[string]http.Header
	pathHeadersMux sync.Mutex
}

func (s *UserAgentPropagationSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
	s.pathHeaders = make(map[string]http.Header)
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		s.pathHeadersMux.Lock()
		s.pathHeaders[req.URL.Path] = req.Header.Clone()
		s.pathHeadersMux.Unlock()
	}))
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api/v1/namespaces/default/pods" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"kind":"PodList","apiVersion":"v1","items":[]}`))
			return
		}
	}))
}

func (s *UserAgentPropagationSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *UserAgentPropagationSuite) TestPropagatesExplicitUserAgentToKubeAPI() {
	s.InitMcpClient(test.WithTransport(transport.WithHTTPHeaders(map[string]string{
		"User-Agent": "custom-mcp-client/2.0",
	})))
	_, _ = s.CallTool("pods_list", map[string]any{})

	s.pathHeadersMux.Lock()
	podsHeaders := s.pathHeaders["/api/v1/namespaces/default/pods"]
	s.pathHeadersMux.Unlock()

	s.Require().NotNil(podsHeaders, "No requests were made to /api/v1/namespaces/default/pods")
	s.Run("DynamicClient propagates User-Agent with server prefix to Kube API", func() {
		s.Equal(
			fmt.Sprintf("kubernetes-mcp-server/0.0.0 (%s/%s) custom-mcp-client/2.0", runtime.GOOS, runtime.GOARCH),
			podsHeaders.Get("User-Agent"),
		)
	})
}

func (s *UserAgentPropagationSuite) TestPropagatesExplicitUserAgentWithOAuthToKubeAPI() {
	s.InitMcpClient(test.WithTransport(transport.WithHTTPHeaders(map[string]string{
		"Authorization": "Bearer a-token-from-mcp-client",
		"User-Agent":    "custom-mcp-client/2.0",
	})))
	_, _ = s.CallTool("pods_list", map[string]any{})

	s.pathHeadersMux.Lock()
	podsHeaders := s.pathHeaders["/api/v1/namespaces/default/pods"]
	s.pathHeadersMux.Unlock()

	s.Require().NotNil(podsHeaders, "No requests were made to /api/v1/namespaces/default/pods")
	s.Run("Derived client propagates User-Agent with server prefix to Kube API", func() {
		s.Equal(
			fmt.Sprintf("kubernetes-mcp-server/0.0.0 (%s/%s) custom-mcp-client/2.0", runtime.GOOS, runtime.GOARCH),
			podsHeaders.Get("User-Agent"),
		)
	})
}

func (s *UserAgentPropagationSuite) TestFallsBackToMCPClientInfoForUserAgent() {
	// Create MCP client through a handler that strips the User-Agent header,
	// simulating a transport without HTTP User-Agent (like stdio).
	provider, err := internalk8s.NewProvider(s.Cfg)
	s.Require().NoError(err)
	s.mcpServer, err = NewServer(Configuration{StaticConfig: s.Cfg}, provider)
	s.Require().NoError(err)
	handler := s.mcpServer.ServeHTTP()
	strippedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("User-Agent")
		handler.ServeHTTP(w, r)
	})
	s.McpClient = test.NewMcpClient(s.T(), strippedHandler)

	_, _ = s.CallTool("pods_list", map[string]any{})

	s.pathHeadersMux.Lock()
	podsHeaders := s.pathHeaders["/api/v1/namespaces/default/pods"]
	s.pathHeadersMux.Unlock()

	s.Require().NotNil(podsHeaders, "No requests were made to /api/v1/namespaces/default/pods")
	s.Run("User-Agent falls back to MCP client name and version", func() {
		// McpInitRequest sets ClientInfo: {Name: "test", Version: "1.33.7"}
		s.Equal(
			fmt.Sprintf("kubernetes-mcp-server/0.0.0 (%s/%s) test/1.33.7", runtime.GOOS, runtime.GOARCH),
			podsHeaders.Get("User-Agent"),
		)
	})
}

func (s *UserAgentPropagationSuite) TestFallsBackToServerPrefixWhenNoClientInfo() {
	// Create MCP client through a handler that strips the User-Agent header
	// and initialize with empty client info.
	provider, err := internalk8s.NewProvider(s.Cfg)
	s.Require().NoError(err)
	s.mcpServer, err = NewServer(Configuration{StaticConfig: s.Cfg}, provider)
	s.Require().NoError(err)
	handler := s.mcpServer.ServeHTTP()
	strippedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("User-Agent")
		handler.ServeHTTP(w, r)
	})
	s.McpClient = test.NewMcpClient(s.T(), strippedHandler, test.WithEmptyClientInfo())

	_, _ = s.CallTool("pods_list", map[string]any{})

	s.pathHeadersMux.Lock()
	podsHeaders := s.pathHeaders["/api/v1/namespaces/default/pods"]
	s.pathHeadersMux.Unlock()

	s.Require().NotNil(podsHeaders, "No requests were made to /api/v1/namespaces/default/pods")
	s.Run("User-Agent uses server prefix only without trailing space", func() {
		// When no HTTP User-Agent and empty MCP ClientInfo, should use server prefix only
		s.Equal(
			fmt.Sprintf("kubernetes-mcp-server/0.0.0 (%s/%s)", runtime.GOOS, runtime.GOARCH),
			podsHeaders.Get("User-Agent"),
		)
	})
}

func TestUserAgentPropagation(t *testing.T) {
	suite.Run(t, new(UserAgentPropagationSuite))
}
