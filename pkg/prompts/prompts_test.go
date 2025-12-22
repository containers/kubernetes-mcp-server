package prompts

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

// PromptsTestSuite tests the prompts package
type PromptsTestSuite struct {
	suite.Suite
}

func (s *PromptsTestSuite) TestToServerPrompts() {
	s.Run("handles empty prompt list", func() {
		var prompts []api.Prompt
		serverPrompts := ToServerPrompts(prompts)
		s.Empty(serverPrompts)
	})

	s.Run("converts Prompt to ServerPrompt correctly", func() {
		prompts := []api.Prompt{{
			Name:        "test-prompt",
			Description: "A test prompt",
			Arguments: []api.PromptArgument{
				{Name: "arg1", Required: true},
			},
			Templates: []api.PromptTemplate{
				{Role: "user", Content: "Hello {{arg1}}"},
			},
		}}

		serverPrompts := ToServerPrompts(prompts)

		s.Len(serverPrompts, 1)
		s.Equal("test-prompt", serverPrompts[0].Prompt.Name)
		s.Equal("A test prompt", serverPrompts[0].Prompt.Description)
		s.Len(serverPrompts[0].Prompt.Arguments, 1)
		s.Equal("arg1", serverPrompts[0].Prompt.Arguments[0].Name)
		s.True(serverPrompts[0].Prompt.Arguments[0].Required)
		s.Len(serverPrompts[0].Prompt.Templates, 1)
		s.Equal("user", serverPrompts[0].Prompt.Templates[0].Role)
		s.Equal("Hello {{arg1}}", serverPrompts[0].Prompt.Templates[0].Content)
	})
}

func (s *PromptsTestSuite) TestPromptHandler() {
	prompts := []api.Prompt{{
		Name:        "test",
		Description: "Test",
		Arguments: []api.PromptArgument{
			{Name: "required_arg", Required: true},
			{Name: "optional_arg", Required: false},
		},
		Templates: []api.PromptTemplate{
			{Role: "user", Content: "Hello {{required_arg}}{{optional_arg}}!"},
		},
	}}
	serverPrompts := ToServerPrompts(prompts)
	handler := serverPrompts[0].Handler
	s.Run("renders messages with argument substitution", func() {
		params := &testPromptRequest{args: map[string]string{
			"required_arg": "World",
			"optional_arg": " of Go",
		}}
		result, err := handler(api.PromptHandlerParams{PromptCallRequest: params})
		s.NoError(err)
		s.NotNil(result)
		s.Len(result.Messages, 1)
		s.Equal("Hello World of Go!", result.Messages[0].Content.Text)
	})
	s.Run("handles optional arguments", func() {
		params := &testPromptRequest{args: map[string]string{
			"required_arg": "Universe",
		}}
		result, err := handler(api.PromptHandlerParams{PromptCallRequest: params})
		s.NoError(err)
		s.NotNil(result)
		s.Len(result.Messages, 1)
		s.Equal("Hello Universe!", result.Messages[0].Content.Text)
	})
	s.Run("validates missing arguments", func() {
		params := &testPromptRequest{args: map[string]string{
			"optional_arg": " of Go",
		}}
		result, err := handler(api.PromptHandlerParams{PromptCallRequest: params})
		s.Error(err)
		s.Contains(err.Error(), "required argument 'required_arg' is missing")
		s.Nil(result)
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
