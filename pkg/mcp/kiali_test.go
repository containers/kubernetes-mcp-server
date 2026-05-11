package mcp

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	kialiToolset "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type KialiSuite struct {
	BaseMcpSuite
	mockServer  *test.MockServer
	toolsetName string
}

func (s *KialiSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.mockServer.Config().BearerToken = "token-xyz"
	s.toolsetName = (&kialiToolset.Toolset{}).GetName()
	// toolset_configs requires the two-phase parsing performed by config.ReadToml,
	// so we replace s.Cfg and restore the runtime fields the suite already set.
	kubeConfig := s.Cfg.KubeConfig
	listOutput := s.Cfg.ListOutput
	readOnly := s.Cfg.ReadOnly
	cfg, err := config.ReadToml([]byte(fmt.Sprintf(`
		toolsets = ["%s"]
		[toolset_configs.kiali]
		url = "%s"
	`, s.toolsetName, s.mockServer.Config().Host)))
	s.Require().NoError(err, "failed to parse kiali toolset config")
	s.Cfg = cfg
	s.Cfg.KubeConfig = kubeConfig
	s.Cfg.ListOutput = listOutput
	s.Cfg.ReadOnly = readOnly
}

func (s *KialiSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *KialiSuite) TestGetTraces() {
	var capturedURL *url.URL
	var capturedBody string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte(`{"traceId":"test-trace-123","spans":[]}`))
	}))
	s.InitMcpClient()

	s.Run("get_trace_details(traceId = 'test-trace-123')", func() {
		traceId := "test-trace-123"
		toolResult, err := s.CallTool(fmt.Sprintf("%s_get_trace_details", s.toolsetName), map[string]interface{}{
			"traceId": traceId,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("path is correct", func() {
			s.Equal("/api/chat/mcp/get_trace_details", capturedURL.Path, "Unexpected path")
		})
		s.Run("request body contains traceId", func() {
			s.Contains(capturedBody, traceId, "Request body should contain trace ID")
		})
		s.Run("response contains trace ID", func() {
			s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, traceId, "Response should contain trace ID")
		})
	})
}

func (s *KialiSuite) TestGetMeshTrafficGraph() {
	var capturedURL *url.URL
	var capturedBody string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte(`{"elements":{}}`))
	}))
	s.InitMcpClient()

	s.Run("get_mesh_traffic_graph with namespaces", func() {
		toolResult, err := s.CallTool(fmt.Sprintf("%s_get_mesh_traffic_graph", s.toolsetName), map[string]interface{}{
			"namespaces": "bookinfo",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("sends single POST to MCP endpoint", func() {
			s.Equal("/api/chat/mcp/get_mesh_traffic_graph", capturedURL.Path, "Unexpected path")
		})
		s.Run("request body contains namespaces", func() {
			s.Contains(capturedBody, "bookinfo", "Request body should contain namespaces")
		})
	})
}

func (s *KialiSuite) TestGetMeshStatus() {
	var capturedURL *url.URL
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	s.InitMcpClient()

	s.Run("get_mesh_status", func() {
		toolResult, err := s.CallTool(fmt.Sprintf("%s_get_mesh_status", s.toolsetName), map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("sends POST to MCP endpoint", func() {
			s.Equal("/api/chat/mcp/get_mesh_status", capturedURL.Path, "Unexpected path")
		})
		s.Run("response contains status", func() {
			s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "healthy", "Response should contain status")
		})
	})
}

func TestKiali(t *testing.T) {
	suite.Run(t, new(KialiSuite))
}
