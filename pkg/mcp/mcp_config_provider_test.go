package mcp

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

// McpConfigProviderSuite tests that ConfigProvider is accessible from tool and prompt handlers at execution time.
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

func (s *McpConfigProviderSuite) TestToolHandlerReceivesClusterProviderStrategy() {
	// Register a tool whose handler reads the cluster provider strategy from ConfigProvider
	testToolset := &configProviderToolset{
		name: "config-provider-test",
		tools: []api.ServerTool{
			{
				Tool: api.Tool{
					Name:        "get_strategy",
					Description: "Returns the cluster provider strategy from ConfigProvider",
				},
				Handler: func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
					strategy := params.GetClusterProviderStrategy()
					return api.NewToolCallResult(strategy, nil), nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Require().NoError(toml.Unmarshal([]byte(`
		toolsets = ["config-provider-test"]
		cluster_provider_strategy = "kubeconfig"
	`), s.Cfg), "Expected to parse config")
	s.InitMcpClient()

	s.Run("tool handler can access cluster provider strategy", func() {
		result, err := s.CallTool("get_strategy", map[string]interface{}{})
		s.NoError(err)
		s.Require().NotNil(result)
		s.Require().Len(result.Content, 1)
		text := result.Content[0].(*mcp.TextContent).Text
		s.Equal("kubeconfig", text)
	})
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
					_, found := params.GetToolsetConfig("kiali")
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
	kubeConfig := s.Cfg.KubeConfig
	listOutput := s.Cfg.ListOutput
	readOnly := s.Cfg.ReadOnly
	cfg, err := config.ReadToml([]byte(`
		toolsets = ["config-provider-test"]
		[toolset_configs.kiali]
		url = "http://kiali.example/"
	`))
	s.Require().NoError(err, "Expected to parse config")
	s.Cfg = cfg
	s.Cfg.KubeConfig = kubeConfig
	s.Cfg.ListOutput = listOutput
	s.Cfg.ReadOnly = readOnly

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

func (s *McpConfigProviderSuite) TestStrategyReflectsConfigReload() {
	// Register a tool that returns the strategy
	testToolset := &configProviderToolset{
		name: "config-provider-test",
		tools: []api.ServerTool{
			{
				Tool: api.Tool{
					Name:        "get_strategy",
					Description: "Returns the cluster provider strategy from ConfigProvider",
				},
				Handler: func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
					return api.NewToolCallResult(params.GetClusterProviderStrategy(), nil), nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Require().NoError(toml.Unmarshal([]byte(`
		toolsets = ["config-provider-test"]
		cluster_provider_strategy = "kubeconfig"
	`), s.Cfg), "Expected to parse config")
	s.InitMcpClient()

	s.Run("initial strategy is kubeconfig", func() {
		result, err := s.CallTool("get_strategy", map[string]interface{}{})
		s.Require().NoError(err)
		s.Require().NotNil(result)
		s.Require().Len(result.Content, 1)
		text := result.Content[0].(*mcp.TextContent).Text
		s.Equal("kubeconfig", text)
	})

	// Reload config with different strategy
	newConfig := config.BaseDefault()
	newConfig.KubeConfig = s.Cfg.KubeConfig
	s.Require().NoError(toml.Unmarshal([]byte(`
		toolsets = ["config-provider-test"]
		cluster_provider_strategy = "in-cluster"
	`), newConfig), "Expected to parse reload config")
	err := s.mcpServer.ReloadConfiguration(newConfig)
	s.Require().NoError(err)

	s.Run("strategy reflects config reload", func() {
		result, err := s.CallTool("get_strategy", map[string]interface{}{})
		s.Require().NoError(err)
		s.Require().NotNil(result)
		s.Require().Len(result.Content, 1)
		text := result.Content[0].(*mcp.TextContent).Text
		s.Equal("in-cluster", text)
	})
}

func (s *McpConfigProviderSuite) TestPromptHandlerReceivesClusterProviderStrategy() {
	// Register a prompt whose handler reads the strategy from ConfigProvider
	testToolset := &configProviderToolset{
		name: "config-provider-test",
		prompts: []api.ServerPrompt{
			{
				Prompt: api.Prompt{
					Name:        "get_strategy_prompt",
					Description: "Returns the cluster provider strategy",
				},
				Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
					strategy := params.GetClusterProviderStrategy()
					return api.NewPromptCallResult("strategy", []api.PromptMessage{
						{
							Role: "user",
							Content: api.PromptContent{
								Type: "text",
								Text: strategy,
							},
						},
					}, nil), nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Require().NoError(toml.Unmarshal([]byte(`
		toolsets = ["config-provider-test"]
		cluster_provider_strategy = "kubeconfig"
	`), s.Cfg), "Expected to parse config")
	s.InitMcpClient()

	s.Run("prompt handler can access cluster provider strategy", func() {
		result, err := s.GetPrompt("get_strategy_prompt", nil)
		s.NoError(err)
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 1)
		text := result.Messages[0].Content.(*mcp.TextContent).Text
		s.Equal("kubeconfig", text)
	})
}

// configProviderToolset is a mock toolset for testing ConfigProvider access
type configProviderToolset struct {
	name    string
	tools   []api.ServerTool
	prompts []api.ServerPrompt
}

func (t *configProviderToolset) GetName() string                                    { return t.name }
func (t *configProviderToolset) GetDescription() string                             { return "Test toolset for ConfigProvider" }
func (t *configProviderToolset) GetTools(_ api.Openshift) []api.ServerTool          { return t.tools }
func (t *configProviderToolset) GetPrompts() []api.ServerPrompt                     { return t.prompts }
func (t *configProviderToolset) GetResources() []api.ServerResource                 { return nil }
func (t *configProviderToolset) GetResourceTemplates() []api.ServerResourceTemplate { return nil }

func TestMcpConfigProvider(t *testing.T) {
	suite.Run(t, new(McpConfigProviderSuite))
}
