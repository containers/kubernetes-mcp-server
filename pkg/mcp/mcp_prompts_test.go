package mcp

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config/configtest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

// McpPromptsSuite tests MCP prompts integration
type McpPromptsSuite struct {
	BaseMcpSuite
}

func (s *McpPromptsSuite) TestListPrompts() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
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
	`)

	s.InitMcpClient()

	prompts, err := s.ListPrompts()

	s.Run("ListPrompts returns prompts", func() {
		s.NoError(err, "call ListPrompts failed")
		s.NotNilf(prompts, "list prompts failed")
	})

	s.Run("config prompt is available with all metadata", func() {
		s.Require().NotNil(prompts)
		var testPrompt *mcp.Prompt
		for _, p := range prompts.Prompts {
			if p.Name == "test-prompt" {
				testPrompt = p
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
	configtest.OverlayTOML(s.T(), &s.Cfg, `
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
	`)

	s.InitMcpClient()

	result, err := s.GetPrompt("substitution-prompt", map[string]string{
		"name": "World",
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
		textContent, ok := result.Messages[0].Content.(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("Hello World!", textContent.Text)
	})
}

func (s *McpPromptsSuite) TestGetPromptMissingRequiredArgument() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
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
	`)

	s.InitMcpClient()

	result, err := s.GetPrompt("required-arg-prompt", map[string]string{})

	s.Run("missing required argument returns error", func() {
		s.Error(err, "expected error for missing required argument")
		s.Nil(result)
		s.Contains(err.Error(), "required argument 'required_arg' is missing")
	})
}

func (s *McpPromptsSuite) TestGetPromptOptionalArgument() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		[[prompts]]
		name = "check-pod-logs"
		title = "Check Pod Logs"
		description = "Show pod logs"

		[[prompts.arguments]]
		name = "name"
		description = "Pod name"
		required = true

		[[prompts.arguments]]
		name = "namespace"
		description = "Pod namespace"
		required = false

		[[prompts.messages]]
		role = "user"
		content = "Show me the logs for pod {{name}} in {{namespace}}"
	`)

	s.InitMcpClient()

	s.Run("optional argument omitted removes its placeholder", func() {
		result, err := s.GetPrompt("check-pod-logs", map[string]string{"name": "my-pod"})
		s.Require().NoError(err, "call GetPrompt failed")
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 1)
		textContent, ok := result.Messages[0].Content.(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("Show me the logs for pod my-pod in ", textContent.Text)
	})

	s.Run("optional argument provided is substituted", func() {
		result, err := s.GetPrompt("check-pod-logs", map[string]string{"name": "my-pod", "namespace": "my-ns"})
		s.Require().NoError(err, "call GetPrompt failed")
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 1)
		textContent, ok := result.Messages[0].Content.(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("Show me the logs for pod my-pod in my-ns", textContent.Text)
	})

	s.Run("missing required argument errors even when optional is provided", func() {
		result, err := s.GetPrompt("check-pod-logs", map[string]string{"namespace": "my-ns"})
		s.Error(err, "expected error for missing required argument")
		s.Nil(result)
		s.Contains(err.Error(), "required argument 'name' is missing")
	})

	s.Run("unknown extra arguments are ignored", func() {
		result, err := s.GetPrompt("check-pod-logs", map[string]string{"name": "my-pod", "bogus": "value"})
		s.Require().NoError(err, "call GetPrompt failed")
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 1)
		textContent, ok := result.Messages[0].Content.(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("Show me the logs for pod my-pod in ", textContent.Text)
	})

	s.Run("empty string argument value substitutes as empty", func() {
		result, err := s.GetPrompt("check-pod-logs", map[string]string{"name": ""})
		s.Require().NoError(err, "empty value satisfies a required argument")
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 1)
		textContent, ok := result.Messages[0].Content.(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("Show me the logs for pod  in ", textContent.Text)
	})
}

func (s *McpPromptsSuite) TestGetPromptNoArguments() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		[[prompts]]
		name = "mcp-version"
		title = "Give me the version"
		description = "Give me the version"

		[[prompts.messages]]
		role = "user"
		content = "Tell me what version of Kubernetes this cluster is running on"
	`)

	s.InitMcpClient()

	s.Run("prompt without arguments lists with no arguments", func() {
		prompts, err := s.ListPrompts()
		s.Require().NoError(err, "call ListPrompts failed")
		var found *mcp.Prompt
		for _, p := range prompts.Prompts {
			if p.Name == "mcp-version" {
				found = p
				break
			}
		}
		s.Require().NotNil(found, "mcp-version should be listed")
		s.Equal("Give me the version", found.Title)
		s.Empty(found.Arguments, "prompt should expose no arguments")
	})

	s.Run("GetPrompt with nil arguments returns the message verbatim", func() {
		result, err := s.GetPrompt("mcp-version", nil)
		s.Require().NoError(err, "call GetPrompt failed")
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 1)
		textContent, ok := result.Messages[0].Content.(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("Tell me what version of Kubernetes this cluster is running on", textContent.Text)
	})

	s.Run("GetPrompt with unexpected arguments still succeeds", func() {
		result, err := s.GetPrompt("mcp-version", map[string]string{"unexpected": "x"})
		s.Require().NoError(err, "call GetPrompt failed")
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 1)
	})
}

func (s *McpPromptsSuite) TestGetPromptMultipleMessages() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		[[prompts]]
		name = "conversation-prompt"
		description = "Multi-message prompt"

		[[prompts.arguments]]
		name = "topic"
		required = true

		[[prompts.messages]]
		role = "user"
		content = "Tell me about {{topic}}"

		[[prompts.messages]]
		role = "assistant"
		content = "Sure, here is what I know about {{topic}}:"

		[[prompts.messages]]
		role = "user"
		content = "Also mention {{topic}} caveats"
	`)

	s.InitMcpClient()

	result, err := s.GetPrompt("conversation-prompt", map[string]string{"topic": "pods"})

	s.Run("all messages are returned in order with roles preserved", func() {
		s.Require().NoError(err, "call GetPrompt failed")
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 3)
		s.Equal("user", string(result.Messages[0].Role))
		s.Equal("assistant", string(result.Messages[1].Role))
		s.Equal("user", string(result.Messages[2].Role))
	})

	s.Run("argument is substituted in every message", func() {
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 3)
		for i, want := range []string{
			"Tell me about pods",
			"Sure, here is what I know about pods:",
			"Also mention pods caveats",
		} {
			textContent, ok := result.Messages[i].Content.(*mcp.TextContent)
			s.Require().True(ok, "expected TextContent in message %d", i)
			s.Equal(want, textContent.Text, "message %d", i)
		}
	})
}

func (s *McpPromptsSuite) TestGetPromptUnknownPrompt() {
	s.InitMcpClient()

	result, err := s.GetPrompt("does-not-exist", nil)

	s.Run("unknown prompt name returns error", func() {
		s.Error(err, "expected error for unknown prompt")
		s.Nil(result)
		s.Contains(err.Error(), "does-not-exist")
	})
}

func TestMcpPromptsSuite(t *testing.T) {
	suite.Run(t, new(McpPromptsSuite))
}
