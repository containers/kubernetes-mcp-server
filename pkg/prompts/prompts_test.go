package prompts

import (
	"bytes"
	"context"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

// PromptsTestSuite tests the prompts package
type PromptsTestSuite struct {
	suite.Suite
}

func (s *PromptsTestSuite) SetupTest() {
	// Clear prompts before each test
	Clear()
}

func (s *PromptsTestSuite) TestLoadFromToml_SinglePrompt() {
	s.Run("parses single prompt with all fields", func() {
		tomlData := `
[[prompts]]
name = "test-prompt"
title = "Test Prompt"
description = "A test prompt for validation"

[[prompts.arguments]]
name = "pod_name"
description = "Name of the pod"
required = true

[[prompts.arguments]]
name = "namespace"
description = "Namespace of the pod"
required = false

[[prompts.messages]]
role = "user"
content = "Describe pod {{pod_name}} in namespace {{namespace}}"

[[prompts.messages]]
role = "assistant"
content = "I'll help you with pod {{pod_name}}"
`

		var temp struct {
			Prompts toml.Primitive `toml:"prompts"`
		}
		md, err := toml.NewDecoder(bytes.NewReader([]byte(tomlData))).Decode(&temp)
		s.Require().NoError(err)

		ctx := context.Background()
		err = LoadFromToml(ctx, temp.Prompts, md)
		s.Require().NoError(err)

		serverPrompts := ConfigPrompts()
		s.Require().Len(serverPrompts, 1)

		prompt := serverPrompts[0].Prompt
		s.Equal("test-prompt", prompt.Name)
		s.Equal("Test Prompt", prompt.Title)
		s.Equal("A test prompt for validation", prompt.Description)

		// Verify arguments
		s.Require().Len(prompt.Arguments, 2)
		s.Equal("pod_name", prompt.Arguments[0].Name)
		s.True(prompt.Arguments[0].Required)
		s.Equal("namespace", prompt.Arguments[1].Name)
		s.False(prompt.Arguments[1].Required)

		// Verify templates
		s.Require().Len(prompt.Templates, 2)
		s.Equal("user", prompt.Templates[0].Role)
		s.Contains(prompt.Templates[0].Content, "{{pod_name}}")
		s.Equal("assistant", prompt.Templates[1].Role)

		// Verify handler was created
		s.NotNil(serverPrompts[0].Handler)
	})
}

func (s *PromptsTestSuite) TestLoadFromToml_MultiplePrompts() {
	tomlData := `
[[prompts]]
name = "prompt-1"
description = "First prompt"

[[prompts.messages]]
role = "user"
content = "Message 1"

[[prompts]]
name = "prompt-2"
description = "Second prompt"

[[prompts.arguments]]
name = "arg1"
required = true

[[prompts.messages]]
role = "user"
content = "Message 2 with {{arg1}}"
`

	var temp struct {
		Prompts toml.Primitive `toml:"prompts"`
	}
	md, err := toml.NewDecoder(bytes.NewReader([]byte(tomlData))).Decode(&temp)
	s.Require().NoError(err)

	ctx := context.Background()
	err = LoadFromToml(ctx, temp.Prompts, md)
	s.Require().NoError(err)

	serverPrompts := ConfigPrompts()
	s.Require().Len(serverPrompts, 2)

	s.Equal("prompt-1", serverPrompts[0].Prompt.Name)
	s.Equal("prompt-2", serverPrompts[1].Prompt.Name)

	// Verify second prompt has arguments
	s.Require().Len(serverPrompts[1].Prompt.Arguments, 1)
	s.Equal("arg1", serverPrompts[1].Prompt.Arguments[0].Name)
	s.True(serverPrompts[1].Prompt.Arguments[0].Required)
}

func (s *PromptsTestSuite) TestRegister() {
	s.Run("registers prompts correctly", func() {
		prompt1 := api.ServerPrompt{
			Prompt: api.Prompt{
				Name:        "test-1",
				Description: "Test 1",
			},
		}
		prompt2 := api.ServerPrompt{
			Prompt: api.Prompt{
				Name:        "test-2",
				Description: "Test 2",
			},
		}

		Register(prompt1, prompt2)

		prompts := ConfigPrompts()
		s.Len(prompts, 2)
		s.Equal("test-1", prompts[0].Prompt.Name)
		s.Equal("test-2", prompts[1].Prompt.Name)
	})
}

func (s *PromptsTestSuite) TestClear() {
	s.Run("clears all prompts", func() {
		prompt := api.ServerPrompt{
			Prompt: api.Prompt{
				Name:        "test",
				Description: "Test",
			},
		}
		Register(prompt)
		s.Len(ConfigPrompts(), 1)

		Clear()
		s.Len(ConfigPrompts(), 0)
	})
}

func (s *PromptsTestSuite) TestPromptHandler() {
	s.Run("validates required arguments", func() {
		prompt := api.Prompt{
			Name:        "test",
			Description: "Test",
			Arguments: []api.PromptArgument{
				{Name: "required_arg", Required: true},
				{Name: "optional_arg", Required: false},
			},
			Templates: []api.PromptTemplate{
				{Role: "user", Content: "Hello {{required_arg}}"},
			},
		}

		handler := createPromptHandler(prompt)

		// Test missing required argument
		params := &testPromptRequest{args: map[string]string{}}
		result, err := handler(api.PromptHandlerParams{PromptCallRequest: params})
		s.Error(err)
		s.Contains(err.Error(), "required argument 'required_arg' is missing")
		s.Nil(result)

		// Test with required argument
		params = &testPromptRequest{args: map[string]string{"required_arg": "value"}}
		result, err = handler(api.PromptHandlerParams{PromptCallRequest: params})
		s.NoError(err)
		s.NotNil(result)
		s.Len(result.Messages, 1)
		s.Equal("Hello value", result.Messages[0].Content.Text)
	})
}

func (s *PromptsTestSuite) TestSubstituteArguments() {
	s.Run("replaces placeholders correctly", func() {
		content := "Hello {{name}}, your age is {{age}}"
		args := map[string]string{
			"name": "Alice",
			"age":  "30",
		}

		result := substituteArguments(content, args)
		s.Equal("Hello Alice, your age is 30", result)
	})

	s.Run("handles missing arguments", func() {
		content := "Hello {{name}}"
		args := map[string]string{}

		result := substituteArguments(content, args)
		s.Equal("Hello {{name}}", result)
	})
}

// testPromptRequest is a test implementation of PromptCallRequest
type testPromptRequest struct {
	args map[string]string
}

func (t *testPromptRequest) GetArguments() map[string]string {
	return t.args
}

func TestPrompts(t *testing.T) {
	suite.Run(t, new(PromptsTestSuite))
}
