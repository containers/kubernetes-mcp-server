package mcp

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

// McpConfigProviderSuite tests that Config is accessible from handlers at execution time.
type McpConfigProviderSuite struct {
	BaseMcpSuite
	originalToolsets []api.Toolset
}

func (s *McpConfigProviderSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.originalToolsets = toolsets.Toolsets()
}

func (s *McpConfigProviderSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	toolsets.Clear()
	for _, toolset := range s.originalToolsets {
		toolsets.Register(toolset)
	}
}

func (s *McpConfigProviderSuite) TestToolHandlerReceivesToolsetConfig() {
	// Register a tool that reads its own toolset config from ConfigProvider
	testToolset := &configProviderToolset{
		name: "config-provider-test",
		tools: []api.ServerTool{
			{
				Tool: api.Tool{
					Name:        "get_toolset_config",
					Description: "Returns whether toolset config was found",
				},
				Handler: func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
					_, found := params.Config.GetToolsetConfig("kiali")
					if found {
						return api.NewToolCallResult("found", nil), nil
					}
					return api.NewToolCallResult("not-found", nil), nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)

	// toolset_configs requires the two-phase parsing performed by config.ReadToml,
	// so we replace s.Cfg and restore the runtime fields the suite already set.
	kubeConfig := s.Cfg.KubeConfig.Get()
	listOutput := s.Cfg.ListOutput.Get()
	readOnly := s.Cfg.ReadOnly.Get()
	cfg, err := config.ReadToml(s.T().Context(), []byte(`
		toolsets = ["config-provider-test"]
		[toolset_configs.kiali]
		url = "http://kiali.example/"
	`))
	s.Require().NoError(err, "Expected to parse config")
	s.Cfg = cfg
	s.Cfg.KubeConfig.SetForTest(kubeConfig)
	s.Cfg.ListOutput.SetForTest(listOutput)
	s.Cfg.ReadOnly.SetForTest(readOnly)

	s.InitMcpClient()

	s.Run("tool handler can access toolset config", func() {
		result, err := s.CallTool("get_toolset_config", map[string]interface{}{})
		s.NoError(err)
		s.Require().NotNil(result)
		s.Require().Len(result.Content, 1)
		text := result.Content[0].(*mcp.TextContent).Text
		s.Equal("found", text)
	})
}

// configProviderToolset is a mock toolset for testing ConfigProvider access
type configProviderToolset struct {
	name    string
	tools   []api.ServerTool
	prompts []api.ServerPrompt
}

func (t *configProviderToolset) GetName() string        { return t.name }
func (t *configProviderToolset) GetDescription() string { return "Test toolset for ConfigProvider" }
func (t *configProviderToolset) GetTools(_ api.FilteringProvider) []api.ServerTool {
	return t.tools
}
func (t *configProviderToolset) GetPrompts() []api.ServerPrompt                     { return t.prompts }
func (t *configProviderToolset) GetResources() []api.ServerResource                 { return nil }
func (t *configProviderToolset) GetResourceTemplates() []api.ServerResourceTemplate { return nil }

func TestMcpConfigProvider(t *testing.T) {
	suite.Run(t, new(McpConfigProviderSuite))
}
