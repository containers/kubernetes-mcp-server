package mcp

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
)

// McpPromptsSuite tests MCP prompts integration
type McpPromptsSuite struct {
	BaseMcpSuite
}

func (s *McpPromptsSuite) TestListPrompts() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		[[prompts]]
		name = "test-prompt"
		title = "Test Prompt"
		description = "A test prompt for integration testing"
		
		[[prompts.arguments]]
		name = "test_arg"
		description = "A test argument"
		required = true
		
		[[prompts.messages]]
		role = "user"
		content = "Test message with {{test_arg}}"
	`), s.Cfg), "Expected to parse prompts config")

	s.InitMcpClient()

	prompts, err := s.ListPrompts(s.T().Context(), mcp.ListPromptsRequest{})

	s.Run("ListPrompts returns prompts", func() {
		s.NoError(err, "call ListPrompts failed")
		s.NotNilf(prompts, "list prompts failed")
	})

	s.Run("config prompt is available with all metadata", func() {
		s.Require().NotNil(prompts)
		var testPrompt *mcp.Prompt
		for _, prompt := range prompts.Prompts {
			if prompt.Name == "test-prompt" {
				testPrompt = &prompt
				break
			}
		}
		s.Require().NotNil(testPrompt, "test-prompt should be found")

		// Verify all metadata fields are returned
		s.Equal("test-prompt", testPrompt.Name)
		s.Equal("A test prompt for integration testing", testPrompt.Description, "description should match")
		s.Require().Len(testPrompt.Arguments, 1)
		s.Equal("test_arg", testPrompt.Arguments[0].Name)
		s.Equal("A test argument", testPrompt.Arguments[0].Description)
		s.True(testPrompt.Arguments[0].Required)
	})
}

func (s *McpPromptsSuite) TestGetPrompt() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		[[prompts]]
		name = "substitution-prompt"
		description = "Test argument substitution"
		
		[[prompts.arguments]]
		name = "name"
		description = "Name to substitute"
		required = true
		
		[[prompts.messages]]
		role = "user"
		content = "Hello {{name}}!"
	`), s.Cfg), "Expected to parse prompts config")

	s.InitMcpClient()

	result, err := s.GetPrompt(s.T().Context(), mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "substitution-prompt",
			Arguments: map[string]string{
				"name": "World",
			},
		},
	})

	s.Run("GetPrompt succeeds", func() {
		s.NoError(err, "call GetPrompt failed")
		s.NotNilf(result, "get prompt failed")
	})

	s.Run("argument substitution works", func() {
		s.Require().NotNil(result)
		s.Equal("Test argument substitution", result.Description)
		s.Require().Len(result.Messages, 1)
		s.Equal("user", string(result.Messages[0].Role))
		textContent, ok := result.Messages[0].Content.(mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("text", textContent.Type)
		s.Equal("Hello World!", textContent.Text)
	})
}

func (s *McpPromptsSuite) TestGetPromptMissingRequiredArgument() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		[[prompts]]
		name = "required-arg-prompt"
		description = "Test required argument validation"
		
		[[prompts.arguments]]
		name = "required_arg"
		description = "A required argument"
		required = true
		
		[[prompts.messages]]
		role = "user"
		content = "Content with {{required_arg}}"
	`), s.Cfg), "Expected to parse prompts config")

	s.InitMcpClient()

	result, err := s.GetPrompt(s.T().Context(), mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name:      "required-arg-prompt",
			Arguments: map[string]string{},
		},
	})

	s.Run("missing required argument returns error", func() {
		s.Error(err, "expected error for missing required argument")
		s.Nil(result)
		s.Contains(err.Error(), "required argument 'required_arg' is missing")
	})
}

func TestMcpPromptsSuite(t *testing.T) {
	suite.Run(t, new(McpPromptsSuite))
}
