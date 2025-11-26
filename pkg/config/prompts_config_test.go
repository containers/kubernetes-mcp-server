package config

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type PromptsConfigSuite struct {
	suite.Suite
}

func (s *PromptsConfigSuite) TestReadConfigWithDisableEmbeddedPrompts() {
	configData := `
disable_embedded_prompts = true
`
	cfg, err := ReadToml([]byte(configData))
	s.Run("no error", func() {
		s.Nilf(err, "failed to read config: %v", err)
	})
	s.Run("disable_embedded_prompts parsed correctly", func() {
		s.Truef(cfg.DisableEmbeddedPrompts, "expected disable_embedded_prompts to be true")
	})
}

func (s *PromptsConfigSuite) TestReadConfigWithInlinePrompts() {
	configData := `
[[prompts]]
name = "test-prompt"
description = "A test prompt"

[[prompts.arguments]]
name = "arg1"
description = "First argument"
required = true

[[prompts.messages]]
role = "user"
content = "Test message with {{arg1}}"
`
	cfg, err := ReadToml([]byte(configData))
	s.Run("no error", func() {
		s.Nilf(err, "failed to read config: %v", err)
	})
	s.Run("prompts parsed correctly", func() {
		s.Lenf(cfg.Prompts, 1, "expected 1 prompt, got %d", len(cfg.Prompts))
		prompt := cfg.Prompts[0]
		s.Equalf("test-prompt", prompt.Name, "expected prompt name to be 'test-prompt', got %s", prompt.Name)
		s.Equalf("A test prompt", prompt.Description, "expected description to match")
		s.Lenf(prompt.Arguments, 1, "expected 1 argument")
		s.Equalf("arg1", prompt.Arguments[0].Name, "expected argument name to be 'arg1'")
		s.Truef(prompt.Arguments[0].Required, "expected argument to be required")
		s.Lenf(prompt.Messages, 1, "expected 1 message")
		s.Equalf("user", prompt.Messages[0].Role, "expected role to be 'user'")
		s.Containsf(prompt.Messages[0].Content, "{{arg1}}", "expected content to contain placeholder")
	})
}

func (s *PromptsConfigSuite) TestReadConfigWithMultipleInlinePrompts() {
	configData := `
[[prompts]]
name = "prompt1"
description = "First prompt"

[[prompts.messages]]
role = "user"
content = "Message 1"

[[prompts]]
name = "prompt2"
description = "Second prompt"

[[prompts.messages]]
role = "assistant"
content = "Message 2"
`
	cfg, err := ReadToml([]byte(configData))
	s.Run("no error", func() {
		s.Nilf(err, "failed to read config: %v", err)
	})
	s.Run("multiple prompts parsed correctly", func() {
		s.Lenf(cfg.Prompts, 2, "expected 2 prompts, got %d", len(cfg.Prompts))
		s.Equalf("prompt1", cfg.Prompts[0].Name, "expected first prompt name to be 'prompt1'")
		s.Equalf("prompt2", cfg.Prompts[1].Name, "expected second prompt name to be 'prompt2'")
	})
}

func TestPromptsConfig(t *testing.T) {
	suite.Run(t, new(PromptsConfigSuite))
}
