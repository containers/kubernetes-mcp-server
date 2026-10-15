package mcp

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	netobservToolset "github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type NetObservSuite struct {
	BaseMcpSuite
	mockServer  *test.MockServer
	toolsetName string
}

func (s *NetObservSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.mockServer.Config().BearerToken = "token-xyz"
	s.toolsetName = (&netobservToolset.Toolset{}).GetName()
	kubeConfig := s.Cfg.KubeConfig.Get()
	listOutput := s.Cfg.ListOutput.Get()
	readOnly := s.Cfg.ReadOnly.Get()
	cfg, err := config.ReadToml(s.T().Context(), []byte(fmt.Sprintf(`
		toolsets = ["%s"]
		[toolset_configs.netobserv]
		url = "%s"
	`, s.toolsetName, s.mockServer.Config().Host)))
	s.Require().NoError(err)
	s.Cfg = cfg
	s.Cfg.KubeConfig.SetForTest(kubeConfig)
	s.Cfg.ListOutput.SetForTest(listOutput)
	s.Cfg.ReadOnly.SetForTest(readOnly)
}

func (s *NetObservSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *NetObservSuite) TestListFlows() {
	var capturedURL *url.URL
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		_, _ = w.Write([]byte(`{"result":[],"stats":{}}`))
	}))
	s.InitMcpClient()

	s.Run("list_flows forwards query parameters", func() {
		toolResult, err := s.CallTool(fmt.Sprintf("%s_list_flows", s.toolsetName), map[string]interface{}{
			"namespace": "default",
			"timeRange": 300,
		})
		s.Nilf(err, "call tool failed %v", err)
		s.Falsef(toolResult.IsError, "call tool failed")
		s.Equal("/api/loki/flow/records", capturedURL.Path)
		s.Equal("default", capturedURL.Query().Get("namespace"))
		s.Empty(capturedURL.Query().Get("timeRange"))
		startTime, err := strconv.ParseInt(capturedURL.Query().Get("startTime"), 10, 64)
		s.NoError(err)
		endTime, err := strconv.ParseInt(capturedURL.Query().Get("endTime"), 10, 64)
		s.NoError(err)
		s.Equal(int64(300), endTime-startTime)
		s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "result")
		s.NotNil(toolResult.StructuredContent)
	})
}

func (s *NetObservSuite) TestExportFlows() {
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/api/loki/export", r.URL.Path)
		s.Equal("csv", r.URL.Query().Get("format"))
		_, _ = w.Write([]byte("TimeFlowStartMs,Bytes\n1,2"))
	}))
	s.InitMcpClient()

	toolResult, err := s.CallTool(fmt.Sprintf("%s_export_flows", s.toolsetName), map[string]interface{}{
		"namespace": "default",
	})
	s.Nilf(err, "call tool failed %v", err)
	s.Falsef(toolResult.IsError, "call tool failed")
	s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "TimeFlowStartMs")
}

func (s *NetObservSuite) TestGetFlowMetrics() {
	var capturedURL *url.URL
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		_, _ = w.Write([]byte(`{"data":[],"stats":{}}`))
	}))
	s.InitMcpClient()

	s.Run("get_flow_metrics forwards query parameters", func() {
		toolResult, err := s.CallTool(fmt.Sprintf("%s_get_flow_metrics", s.toolsetName), map[string]interface{}{
			"namespace":   "default",
			"timeRange":   300,
			"aggregateBy": "namespace",
			"type":        "Bytes",
		})
		s.Nilf(err, "call tool failed %v", err)
		s.Falsef(toolResult.IsError, "call tool failed")
		s.Equal("/api/flow/metrics", capturedURL.Path)
		s.Equal("default", capturedURL.Query().Get("namespace"))
		s.Empty(capturedURL.Query().Get("timeRange"))
		startTime, err := strconv.ParseInt(capturedURL.Query().Get("startTime"), 10, 64)
		s.NoError(err)
		endTime, err := strconv.ParseInt(capturedURL.Query().Get("endTime"), 10, 64)
		s.NoError(err)
		s.Equal(int64(300), endTime-startTime)
		s.Equal("namespace", capturedURL.Query().Get("aggregateBy"))
		s.Equal("Bytes", capturedURL.Query().Get("type"))
		s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "data")
		s.NotNil(toolResult.StructuredContent)
	})
}

func (s *NetObservSuite) TestManualBackendConfigUsesStableToolsetKey() {
	kubeConfig := s.Cfg.KubeConfig.Get()
	listOutput := s.Cfg.ListOutput.Get()
	readOnly := s.Cfg.ReadOnly.Get()
	cfg, err := config.ReadToml(s.T().Context(), []byte(fmt.Sprintf(`
		toolsets = ["%s"]
		[toolset_configs.netobserv]
		url = "%s"
		loki_enabled = true
		loki_mode = "LokiStack"
		prometheus_enabled = true
		prometheus_mode = "Manual"
	`, s.toolsetName, s.mockServer.Config().Host)))
	s.Require().NoError(err)
	s.Cfg = cfg
	s.Cfg.KubeConfig.SetForTest(kubeConfig)
	s.Cfg.ListOutput.SetForTest(listOutput)
	s.Cfg.ReadOnly.SetForTest(readOnly)
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NoError(err)
	byName := make(map[string]*mcp.Tool, len(tools.Tools))
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
	}

	listName := s.toolsetName + "_list_flows"
	exportName := s.toolsetName + "_export_flows"
	metricsName := s.toolsetName + "_get_flow_metrics"
	s.Require().Contains(byName, listName)
	s.Require().Contains(byName, exportName)
	s.Require().Contains(byName, metricsName)

	for _, name := range []string{listName, exportName} {
		meta, _ := byName[name].Meta[api.RBACMetadataKey].(map[string]any)
		bounded, _ := meta["bounded"].(map[string]any)
		reqs, _ := bounded["requirements"].([]any)
		s.Require().Len(reqs, 1, name)
		req, _ := reqs[0].(map[string]any)
		s.Equal([]any{"get"}, req["verbs"], name)
		target, _ := req["target"].(map[string]any)
		resource, _ := target["resource"].(map[string]any)
		s.Equal("loki.grafana.com", resource["apiGroup"], name)
		s.Equal("network", resource["resource"], name)
		resourceName, _ := req["resourceName"].(map[string]any)
		s.Equal("logs", resourceName["name"], name)
	}
	_, hasMetricsRBAC := byName[metricsName].Meta[api.RBACMetadataKey]
	s.False(hasMetricsRBAC)
}

func TestNetObservMcp(t *testing.T) {
	suite.Run(t, new(NetObservSuite))
}
