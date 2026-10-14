package mcp

import (
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	dynamicclient "k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/dynamic"
)

// RequireTLSSuite tests Layer 2 (runtime) TLS enforcement for dynamic http
// tools: with require_tls enabled, a plain-HTTP URL is refused at request
// time even though the Tool resource declares it.
type RequireTLSSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
}

func (s *RequireTLSSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"traceId":"test-trace-123","spans":[]}`))
	}))
	s.Cfg.Toolsets.SetForTest(append(s.Cfg.Toolsets.Get(), "dynamic"))
	obj := &unstructured.Unstructured{}
	s.Require().NoError(yaml.Unmarshal([]byte(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: plain-http-trace
spec:
  description: Fetch a trace from a plain-HTTP backend
  readOnly: true
  http:
    url: `+s.mockServer.Config().Host+`
    path: /api/mcp/get_trace_details
    response: text
`), &obj.Object))
	_, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).Create(s.T().Context(), obj, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *RequireTLSSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	_ = dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).
		DeleteCollection(s.T().Context(), metav1.DeleteOptions{}, metav1.ListOptions{})
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *RequireTLSSuite) TestRequireTLS_BlocksHTTPRequests() {
	s.Cfg.RequireTLS.SetForTest(true)
	s.InitMcpClient()
	toolResult, err := s.CallTool("plain_http_trace", map[string]interface{}{})
	s.Require().Nilf(err, "MCP protocol error: %v", err)
	s.True(toolResult.IsError, "tool call should return an error result")
	s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "require_tls", "error should mention require_tls")
}

func (s *RequireTLSSuite) TestRequireTLS_AllowsHTTPWhenDisabled() {
	s.Cfg.RequireTLS.SetForTest(false)
	s.InitMcpClient()
	toolResult, err := s.CallTool("plain_http_trace", map[string]interface{}{})
	s.Require().Nilf(err, "MCP protocol error: %v", err)
	s.False(toolResult.IsError, "tool call should succeed without require_tls")
	s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "test-trace-123")
}

func TestRequireTLS(t *testing.T) {
	suite.Run(t, new(RequireTLSSuite))
}
