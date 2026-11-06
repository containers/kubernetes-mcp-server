package mcp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/korrel8r"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type Korrel8rSuite struct {
	BaseMcpSuite
	backend *httptest.Server
}

func TestKorrel8rMCP(t *testing.T) { suite.Run(t, new(Korrel8rSuite)) }

func (s *Korrel8rSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/domains" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[{"name":"k8s"}]`))
	}))
	kubeConfig := s.Cfg.KubeConfig.Get()
	listOutput := s.Cfg.ListOutput.Get()
	readOnly := s.Cfg.ReadOnly.Get()
	toolsetNames := s.Cfg.Toolsets.Get()
	cfg, err := config.ReadToml(s.T().Context(), []byte(fmt.Sprintf(`
[toolset_configs."%s"]
url = %q
`, korrel8r.ToolsetName, s.backend.URL)), config.WithBaseDefault())
	s.Require().NoError(err)
	s.Cfg = cfg
	s.Cfg.KubeConfig.SetForTest(kubeConfig)
	s.Cfg.ListOutput.SetForTest(listOutput)
	s.Cfg.ReadOnly.SetForTest(readOnly)
	s.Cfg.Toolsets.SetForTest(append(toolsetNames, korrel8r.ToolsetName))
}

func (s *Korrel8rSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	s.backend.Close()
}

func (s *Korrel8rSuite) TestToolsAvailableAndExecutable() {
	s.InitMcpClient()
	tools, err := s.ListTools()
	s.Require().NoError(err)
	korrel8rTools := 0
	for _, tool := range tools.Tools {
		if strings.HasPrefix(tool.Name, "korrel8r_") {
			korrel8rTools++
		}
	}
	s.Equal(9, korrel8rTools)
	result, err := s.CallTool("korrel8r_list_domains", nil)
	s.Require().NoError(err)
	s.False(result.IsError)
	s.Contains(result.Content[0].(*mcp.TextContent).Text, "k8s")
}

func (s *Korrel8rSuite) TestReadOnlyFiltersConsoleMutation() {
	s.Cfg.ReadOnly.SetForTest(true)
	s.InitMcpClient()
	tools, err := s.ListTools()
	s.Require().NoError(err)
	for _, tool := range tools.Tools {
		s.NotEqual("korrel8r_show_in_console", tool.Name)
	}
	result, err := s.CallTool("korrel8r_list_domains", nil)
	s.Require().NoError(err)
	s.False(result.IsError)
}

func (s *Korrel8rSuite) TestToolFiltering() {
	s.Cfg.EnabledTools.SetForTest([]string{"korrel8r_list_domains"})
	s.InitMcpClient()
	tools, err := s.ListTools()
	s.Require().NoError(err)
	s.Require().Len(tools.Tools, 1)
	s.Equal("korrel8r_list_domains", tools.Tools[0].Name)
	result, err := s.CallTool("korrel8r_list_domains", nil)
	s.Require().NoError(err)
	s.False(result.IsError)
}

func (s *Korrel8rSuite) TestUnconfiguredInstructions() {
	cfg := config.BaseDefault()
	cfg.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
	cfg.Toolsets.SetForTest(append(cfg.Toolsets.Get(), korrel8r.ToolsetName))
	s.Cfg = cfg
	s.InitMcpClient()
	result, err := s.CallTool("korrel8r_instructions", nil)
	s.Require().NoError(err)
	s.False(result.IsError)
	result, err = s.CallTool("korrel8r_list_domains", nil)
	s.Require().NoError(err)
	s.True(result.IsError)
}
