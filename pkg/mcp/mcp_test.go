package mcp

import (
	"net/http"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/stretchr/testify/suite"
)

type McpHeadersSuite struct {
	BaseMcpSuite
	mockServer  *test.MockServer
	pathHeaders map[string]http.Header
}

func (s *McpHeadersSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
	s.pathHeaders = make(map[string]http.Header)
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		s.pathHeaders[req.URL.Path] = req.Header.Clone()
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
		w.WriteHeader(404)
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
		s.Require().Greater(len(s.pathHeaders), 0, "No requests were made to Kube API")
		s.Run("DiscoveryClient propagates "+header+" header to Kube API", func() {
			s.Require().NotNil(s.pathHeaders["/api"], "No requests were made to /api")
			s.Equal("Bearer a-token-from-mcp-client", s.pathHeaders["/api"].Get("Authorization"), "Overridden header Authorization not found in request to /api")
			s.Require().NotNil(s.pathHeaders["/apis"], "No requests were made to /apis")
			s.Equal("Bearer a-token-from-mcp-client", s.pathHeaders["/apis"].Get("Authorization"), "Overridden header Authorization not found in request to /apis")
			s.Require().NotNil(s.pathHeaders["/api/v1"], "No requests were made to /api/v1")
			s.Equal("Bearer a-token-from-mcp-client", s.pathHeaders["/api/v1"].Get("Authorization"), "Overridden header Authorization not found in request to /api/v1")
		})
		s.Run("DynamicClient propagates "+header+" header to Kube API", func() {
			s.Require().NotNil(s.pathHeaders["/api/v1/namespaces/default/pods"], "No requests were made to /api/v1/namespaces/default/pods")
			s.Equal("Bearer a-token-from-mcp-client", s.pathHeaders["/api/v1/namespaces/default/pods"].Get("Authorization"), "Overridden header Authorization not found in request to /api/v1/namespaces/default/pods")
		})
		_, _ = s.CallTool("pods_delete", map[string]interface{}{"name": "a-pod-to-delete"})
		s.Run("kubernetes.Interface propagates "+header+" header to Kube API", func() {
			s.Require().NotNil(s.pathHeaders["/api/v1/namespaces/default/pods/a-pod-to-delete"], "No requests were made to /api/v1/namespaces/default/pods/a-pod-to-delete")
			s.Equal("Bearer a-token-from-mcp-client", s.pathHeaders["/api/v1/namespaces/default/pods/a-pod-to-delete"].Get("Authorization"), "Overridden header Authorization not found in request to /api/v1/namespaces/default/pods/a-pod-to-delete")
		})

	}
}

func TestMcpHeaders(t *testing.T) {
	suite.Run(t, new(McpHeadersSuite))
}
