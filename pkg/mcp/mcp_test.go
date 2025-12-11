package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/mark3labs/mcp-go/client"
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
	s.mockServer.Handle(&test.DiscoveryClientHandler{})
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
		s.InitMcpClient(transport.WithHTTPHeaders(map[string]string{header: "Bearer a-token-from-mcp-client"}))
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
	mockServer *test.MockServer
}

func (s *ServerInstructionsSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(&test.DiscoveryClientHandler{})
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
}

func (s *ServerInstructionsSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *ServerInstructionsSuite) TestServerInstructions() {
	s.Run("returns empty instructions when not configured", func() {
		s.Cfg.ServerInstructions = ""
		mcpServer, err := NewServer(Configuration{StaticConfig: s.Cfg})
		s.Require().NoError(err)
		s.T().Cleanup(mcpServer.Close)

		testServer := httptest.NewServer(mcpServer.ServeHTTP())
		s.T().Cleanup(testServer.Close)

		mcpClient, err := client.NewStreamableHttpClient(testServer.URL+"/mcp", transport.WithContinuousListening())
		s.Require().NoError(err)
		s.T().Cleanup(func() { _ = mcpClient.Close() })

		err = mcpClient.Start(context.Background())
		s.Require().NoError(err)

		result, err := mcpClient.Initialize(context.Background(), test.McpInitRequest())
		s.Require().NoError(err)
		s.Empty(result.Instructions, "instructions should be empty when not configured")
	})

	s.Run("returns configured instructions", func() {
		expectedInstructions := "Always use YAML output format for kubectl commands."
		s.Cfg.ServerInstructions = expectedInstructions

		mcpServer, err := NewServer(Configuration{StaticConfig: s.Cfg})
		s.Require().NoError(err)
		s.T().Cleanup(mcpServer.Close)

		testServer := httptest.NewServer(mcpServer.ServeHTTP())
		s.T().Cleanup(testServer.Close)

		mcpClient, err := client.NewStreamableHttpClient(testServer.URL+"/mcp", transport.WithContinuousListening())
		s.Require().NoError(err)
		s.T().Cleanup(func() { _ = mcpClient.Close() })

		err = mcpClient.Start(context.Background())
		s.Require().NoError(err)

		result, err := mcpClient.Initialize(context.Background(), test.McpInitRequest())
		s.Require().NoError(err)
		s.Equal(expectedInstructions, result.Instructions, "instructions should match configured value")
	})
}

func TestServerInstructions(t *testing.T) {
	suite.Run(t, new(ServerInstructionsSuite))
}
