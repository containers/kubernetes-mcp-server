package mcp

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
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
	kubeConfig := s.Cfg.KubeConfig
	listOutput := s.Cfg.ListOutput
	readOnly := s.Cfg.ReadOnly
	cfg, err := config.ReadToml([]byte(fmt.Sprintf(`
		toolsets = ["%s"]
		[toolset_configs.netobserv]
		url = "%s"
	`, s.toolsetName, s.mockServer.Config().Host)))
	s.Require().NoError(err)
	s.Cfg = cfg
	s.Cfg.KubeConfig = kubeConfig
	s.Cfg.ListOutput = listOutput
	s.Cfg.ReadOnly = readOnly
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
		s.Equal("300", capturedURL.Query().Get("timeRange"))
		s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "result")
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

func (s *NetObservSuite) TestListNamespaces() {
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/api/resources/namespaces", r.URL.Path)
		_, _ = w.Write([]byte(`["default","openshift-netobserv"]`))
	}))
	s.InitMcpClient()

	toolResult, err := s.CallTool(fmt.Sprintf("%s_list_namespaces", s.toolsetName), map[string]interface{}{})
	s.Nilf(err, "call tool failed %v", err)
	s.Falsef(toolResult.IsError, "call tool failed")
	s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "openshift-netobserv")
}

func (s *NetObservSuite) TestListAlerts_fallsBackToPrometheus() {
	prom := test.NewMockServer()
	s.T().Cleanup(prom.Close)
	prom.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/api/v1/rules", r.URL.Path)
		_, _ = w.Write([]byte(`{"status":"success","data":{"groups":[]}}`))
	}))

	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/prometheus/api/v1/rules" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))

	kubeConfig := s.Cfg.KubeConfig
	listOutput := s.Cfg.ListOutput
	readOnly := s.Cfg.ReadOnly
	cfg, err := config.ReadToml([]byte(fmt.Sprintf(`
		toolsets = ["%s"]
		[toolset_configs.netobserv]
		url = "%s"
		prometheus_url = "%s"
		insecure = true
	`, s.toolsetName, s.mockServer.Config().Host, prom.Config().Host)))
	s.Require().NoError(err)
	s.Cfg = cfg
	s.Cfg.KubeConfig = kubeConfig
	s.Cfg.ListOutput = listOutput
	s.Cfg.ReadOnly = readOnly
	s.InitMcpClient()

	toolResult, err := s.CallTool(fmt.Sprintf("%s_list_alerts", s.toolsetName), map[string]interface{}{
		"type": "alert",
	})
	s.Nilf(err, "call tool failed %v", err)
	s.Falsef(toolResult.IsError, "call tool failed")
	s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, `"groups"`)
}

func (s *NetObservSuite) TestListNames() {
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/api/resources/names", r.URL.Path)
		s.Equal("default", r.URL.Query().Get("namespace"))
		s.Equal("Pod", r.URL.Query().Get("kind"))
		_, _ = w.Write([]byte(`["pod-a","pod-b"]`))
	}))
	s.InitMcpClient()

	toolResult, err := s.CallTool(fmt.Sprintf("%s_list_names", s.toolsetName), map[string]interface{}{
		"namespace": "default",
		"kind":      "Pod",
	})
	s.Nilf(err, "call tool failed %v", err)
	s.Falsef(toolResult.IsError, "call tool failed")
	s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "pod-a")
}

func TestNetObservMcp(t *testing.T) {
	suite.Run(t, new(NetObservSuite))
}
