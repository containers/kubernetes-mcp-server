package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// McpToolsetPromptsSuite tests toolset prompts integration
type McpToolsetPromptsSuite struct {
	BaseMcpSuite
	originalToolsets []api.Toolset
}

func (s *McpToolsetPromptsSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.originalToolsets = toolsets.Toolsets()
}

func (s *McpToolsetPromptsSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	// Restore original toolsets
	toolsets.Clear()
	for _, toolset := range s.originalToolsets {
		toolsets.Register(toolset)
	}
}

func (s *McpToolsetPromptsSuite) TestToolsetReturningPrompts() {
	testToolset := &mockToolsetWithPrompts{
		name:        "test-toolset",
		description: "Test toolset with prompts",
		prompts: []api.ServerPrompt{
			{
				Prompt: api.Prompt{
					Name:        "toolset-prompt",
					Description: "A prompt from a toolset",
					Arguments: []api.PromptArgument{
						{Name: "arg1", Description: "Test argument", Required: true},
					},
				},
				Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
					args := params.GetArguments()
					messages := []api.PromptMessage{
						{
							Role: "user",
							Content: api.PromptContent{
								Type: "text",
								Text: "Toolset prompt with " + args["arg1"],
							},
						},
					}
					return api.NewPromptCallResult("Toolset prompt result", messages, nil), nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"test-toolset"}

	s.InitMcpClient()

	prompts, err := s.ListPrompts(s.T().Context(), mcp.ListPromptsRequest{})

	s.Run("ListPrompts returns toolset prompts", func() {
		s.NoError(err)
		s.NotNil(prompts)
	})

	s.Run("toolset prompt is available", func() {
		s.Require().NotNil(prompts)
		var found bool
		for _, prompt := range prompts.Prompts {
			if prompt.Name == "toolset-prompt" {
				found = true
				s.Equal("A prompt from a toolset", prompt.Description)
				s.Require().Len(prompt.Arguments, 1)
				s.Equal("arg1", prompt.Arguments[0].Name)
				s.True(prompt.Arguments[0].Required)
				break
			}
		}
		s.True(found, "expected toolset prompt to be available")
	})

	s.Run("toolset prompt handler executes correctly", func() {
		result, err := s.GetPrompt(s.T().Context(), mcp.GetPromptRequest{
			Params: mcp.GetPromptParams{
				Name: "toolset-prompt",
				Arguments: map[string]string{
					"arg1": "test-value",
				},
			},
		})

		s.NoError(err)
		s.Require().NotNil(result)
		s.Equal("Toolset prompt result", result.Description)
		s.Require().Len(result.Messages, 1)
		s.Equal("user", string(result.Messages[0].Role))

		textContent, ok := result.Messages[0].Content.(mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("Toolset prompt with test-value", textContent.Text)
	})
}

func (s *McpToolsetPromptsSuite) TestToolsetReturningNilPrompts() {
	testToolset := &mockToolsetWithPrompts{
		name:        "empty-toolset",
		description: "Toolset with no prompts",
		prompts:     nil,
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"empty-toolset"}

	s.InitMcpClient()

	prompts, err := s.ListPrompts(s.T().Context(), mcp.ListPromptsRequest{})

	s.Run("ListPrompts succeeds with nil toolset prompts", func() {
		s.NoError(err)
		s.NotNil(prompts)
	})

	s.Run("no prompts returned from nil toolset", func() {
		s.Require().NotNil(prompts)
		s.Empty(prompts.Prompts)
	})
}

func (s *McpToolsetPromptsSuite) TestToolsetReturningEmptyPrompts() {
	testToolset := &mockToolsetWithPrompts{
		name:        "empty-slice-toolset",
		description: "Toolset with empty prompts slice",
		prompts:     []api.ServerPrompt{},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"empty-slice-toolset"}

	s.InitMcpClient()

	prompts, err := s.ListPrompts(s.T().Context(), mcp.ListPromptsRequest{})

	s.Run("ListPrompts succeeds with empty toolset prompts", func() {
		s.NoError(err)
		s.NotNil(prompts)
	})

	s.Run("no prompts returned from empty slice toolset", func() {
		s.Require().NotNil(prompts)
		s.Empty(prompts.Prompts)
	})
}

func (s *McpToolsetPromptsSuite) TestMultipleToolsetsPromptCollection() {
	toolset1 := &mockToolsetWithPrompts{
		name:        "toolset1",
		description: "First toolset",
		prompts: []api.ServerPrompt{
			{
				Prompt: api.Prompt{
					Name:        "prompt1",
					Description: "Prompt from toolset1",
				},
				Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
					return api.NewPromptCallResult("Prompt1", []api.PromptMessage{}, nil), nil
				},
			},
		},
	}

	toolset2 := &mockToolsetWithPrompts{
		name:        "toolset2",
		description: "Second toolset",
		prompts: []api.ServerPrompt{
			{
				Prompt: api.Prompt{
					Name:        "prompt2",
					Description: "Prompt from toolset2",
				},
				Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
					return api.NewPromptCallResult("Prompt2", []api.PromptMessage{}, nil), nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(toolset1)
	toolsets.Register(toolset2)
	s.Cfg.Toolsets = []string{"toolset1", "toolset2"}

	s.InitMcpClient()

	prompts, err := s.ListPrompts(s.T().Context(), mcp.ListPromptsRequest{})

	s.Run("ListPrompts collects from multiple toolsets", func() {
		s.NoError(err)
		s.Require().NotNil(prompts)
		s.Require().Len(prompts.Prompts, 2)
	})

	s.Run("prompts from both toolsets are available", func() {
		s.Require().NotNil(prompts)
		promptNames := make(map[string]bool)
		for _, prompt := range prompts.Prompts {
			promptNames[prompt.Name] = true
		}
		s.True(promptNames["prompt1"], "expected prompt1 from toolset1")
		s.True(promptNames["prompt2"], "expected prompt2 from toolset2")
	})
}

func (s *McpToolsetPromptsSuite) TestConfigPromptsOverrideToolsetPrompts() {
	testToolset := &mockToolsetWithPrompts{
		name:        "test-toolset",
		description: "Test toolset",
		prompts: []api.ServerPrompt{
			{
				Prompt: api.Prompt{
					Name:        "shared-prompt",
					Description: "Toolset version",
				},
				Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
					return api.NewPromptCallResult("Toolset", []api.PromptMessage{
						{
							Role: "user",
							Content: api.PromptContent{
								Type: "text",
								Text: "From toolset",
							},
						},
					}, nil), nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)

	// Add config prompt with same name
	cfg, err := config.ReadToml([]byte(`
toolsets = ["test-toolset"]

[[prompts]]
name = "shared-prompt"
description = "Config version"

[[prompts.messages]]
role = "user"
content = "From config"
	`))
	s.Require().NoError(err)
	// Preserve kubeconfig from SetupTest
	cfg.KubeConfig = s.Cfg.KubeConfig
	s.Cfg = cfg

	s.InitMcpClient()

	prompts, err := s.ListPrompts(s.T().Context(), mcp.ListPromptsRequest{})

	s.Run("ListPrompts returns prompts", func() {
		s.NoError(err)
		s.Require().NotNil(prompts)
		s.Require().Len(prompts.Prompts, 1)
	})

	s.Run("config prompt overrides toolset prompt", func() {
		s.Require().NotNil(prompts)
		s.Equal("shared-prompt", prompts.Prompts[0].Name)
		s.Equal("Config version", prompts.Prompts[0].Description)
	})

	s.Run("config prompt handler is used", func() {
		result, err := s.GetPrompt(s.T().Context(), mcp.GetPromptRequest{
			Params: mcp.GetPromptParams{
				Name: "shared-prompt",
			},
		})

		s.NoError(err)
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 1)

		textContent, ok := result.Messages[0].Content.(mcp.TextContent)
		s.Require().True(ok)
		s.Equal("From config", textContent.Text)
	})
}

func (s *McpToolsetPromptsSuite) TestPromptsNotExposedWhenToolsetDisabled() {
	enabledToolset := &mockToolsetWithPrompts{
		name:        "enabled-toolset",
		description: "Enabled toolset",
		prompts: []api.ServerPrompt{
			{
				Prompt: api.Prompt{
					Name:        "enabled-prompt",
					Description: "From enabled toolset",
				},
				Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
					return api.NewPromptCallResult("Enabled", []api.PromptMessage{}, nil), nil
				},
			},
		},
	}

	disabledToolset := &mockToolsetWithPrompts{
		name:        "disabled-toolset",
		description: "Disabled toolset",
		prompts: []api.ServerPrompt{
			{
				Prompt: api.Prompt{
					Name:        "disabled-prompt",
					Description: "From disabled toolset",
				},
				Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
					return api.NewPromptCallResult("Disabled", []api.PromptMessage{}, nil), nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(enabledToolset)
	toolsets.Register(disabledToolset)
	// Only enable one toolset
	s.Cfg.Toolsets = []string{"enabled-toolset"}

	s.InitMcpClient()

	prompts, err := s.ListPrompts(s.T().Context(), mcp.ListPromptsRequest{})

	s.Run("ListPrompts returns prompts", func() {
		s.NoError(err)
		s.Require().NotNil(prompts)
	})

	s.Run("only enabled toolset prompts are available", func() {
		s.Require().NotNil(prompts)
		s.Require().Len(prompts.Prompts, 1)
		s.Equal("enabled-prompt", prompts.Prompts[0].Name)
	})

	s.Run("disabled toolset prompts are not available", func() {
		s.Require().NotNil(prompts)
		for _, prompt := range prompts.Prompts {
			s.NotEqual("disabled-prompt", prompt.Name)
		}
	})
}

// Mock toolset for testing
type mockToolsetWithPrompts struct {
	name        string
	description string
	prompts     []api.ServerPrompt
}

func (m *mockToolsetWithPrompts) GetName() string {
	return m.name
}

func (m *mockToolsetWithPrompts) GetDescription() string {
	return m.description
}

func (m *mockToolsetWithPrompts) GetTools(_ api.Openshift) []api.ServerTool {
	return nil
}

func (m *mockToolsetWithPrompts) GetPrompts() []api.ServerPrompt {
	return m.prompts
}

func TestMcpToolsetPromptsSuite(t *testing.T) {
	suite.Run(t, new(McpToolsetPromptsSuite))
}
