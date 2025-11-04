package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func TestServerPromptToGoSdkPrompt(t *testing.T) {
	t.Run("Converts empty prompt list", func(t *testing.T) {
		// Given
		prompts := []api.ServerPrompt{}

		// When
		resultPrompts, resultHandlers, err := ServerPromptToGoSdkPrompt(nil, prompts)

		// Then
		require.NoError(t, err)
		assert.Empty(t, resultPrompts)
		assert.Empty(t, resultHandlers)
	})

	t.Run("Converts single prompt correctly", func(t *testing.T) {
		// Given
		prompts := []api.ServerPrompt{
			{
				Name:        "test_prompt",
				Description: "Test prompt description",
				Arguments: []api.PromptArgument{
					{
						Name:        "arg1",
						Description: "Argument 1",
						Required:    true,
					},
				},
				GetMessages: func(arguments map[string]string) []api.PromptMessage {
					return []api.PromptMessage{
						{Role: "user", Content: "Hello"},
						{Role: "assistant", Content: "Hi there"},
					}
				},
			},
		}

		// When
		resultPrompts, resultHandlers, err := ServerPromptToGoSdkPrompt(nil, prompts)

		// Then
		require.NoError(t, err)
		require.Len(t, resultPrompts, 1)
		require.Len(t, resultHandlers, 1)

		prompt := resultPrompts["test_prompt"]
		require.NotNil(t, prompt)
		assert.Equal(t, "test_prompt", prompt.Name)
		assert.Equal(t, "Test prompt description", prompt.Description)
		require.Len(t, prompt.Arguments, 1)

		arg := prompt.Arguments[0]
		assert.Equal(t, "arg1", arg.Name)
		assert.Equal(t, "Argument 1", arg.Description)
		assert.True(t, arg.Required)
	})

	t.Run("Converts multiple prompts correctly", func(t *testing.T) {
		// Given
		prompts := []api.ServerPrompt{
			{
				Name:        "prompt1",
				Description: "First prompt",
				Arguments:   []api.PromptArgument{},
				GetMessages: func(arguments map[string]string) []api.PromptMessage {
					return []api.PromptMessage{{Role: "user", Content: "test1"}}
				},
			},
			{
				Name:        "prompt2",
				Description: "Second prompt",
				Arguments:   []api.PromptArgument{},
				GetMessages: func(arguments map[string]string) []api.PromptMessage {
					return []api.PromptMessage{{Role: "user", Content: "test2"}}
				},
			},
		}

		// When
		resultPrompts, resultHandlers, err := ServerPromptToGoSdkPrompt(nil, prompts)

		// Then
		require.NoError(t, err)
		assert.Len(t, resultPrompts, 2)
		assert.Len(t, resultHandlers, 2)
		assert.NotNil(t, resultPrompts["prompt1"])
		assert.NotNil(t, resultPrompts["prompt2"])
	})
}

func TestCreatePromptHandler(t *testing.T) {
	t.Run("Handler returns correct messages", func(t *testing.T) {
		// Given
		prompt := api.ServerPrompt{
			Name:        "test",
			Description: "Test prompt",
			Arguments:   []api.PromptArgument{},
			GetMessages: func(arguments map[string]string) []api.PromptMessage {
				return []api.PromptMessage{
					{Role: "user", Content: "Test message"},
					{Role: "assistant", Content: "Test response"},
				}
			},
		}

		handler := createPromptHandler(nil, prompt)

		// Create request with empty arguments
		request := &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Name:      "test",
				Arguments: map[string]string{},
			},
		}

		// When
		result, err := handler(context.Background(), request)

		// Then
		require.NoError(t, err)
		assert.Equal(t, "Test prompt", result.Description)
		require.Len(t, result.Messages, 2)
		assert.Equal(t, mcp.Role("user"), result.Messages[0].Role)
		textContent := result.Messages[0].Content.(*mcp.TextContent)
		assert.Equal(t, "Test message", textContent.Text)
		assert.Equal(t, mcp.Role("assistant"), result.Messages[1].Role)
		textContent2 := result.Messages[1].Content.(*mcp.TextContent)
		assert.Equal(t, "Test response", textContent2.Text)
	})

	t.Run("Handler uses provided arguments", func(t *testing.T) {
		// Given
		prompt := api.ServerPrompt{
			Name:        "test",
			Description: "Test prompt",
			Arguments: []api.PromptArgument{
				{Name: "param1", Description: "Parameter 1", Required: false},
			},
			GetMessages: func(arguments map[string]string) []api.PromptMessage {
				value := arguments["param1"]
				return []api.PromptMessage{
					{Role: "user", Content: "Value is: " + value},
				}
			},
		}

		handler := createPromptHandler(nil, prompt)

		// Create request with arguments
		request := &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Name:      "test",
				Arguments: map[string]string{"param1": "test_value"},
			},
		}

		// When
		result, err := handler(context.Background(), request)

		// Then
		require.NoError(t, err)
		require.Len(t, result.Messages, 1)
		textContent := result.Messages[0].Content.(*mcp.TextContent)
		assert.Equal(t, "Value is: test_value", textContent.Text)
	})

	t.Run("Handler handles nil arguments", func(t *testing.T) {
		// Given
		prompt := api.ServerPrompt{
			Name:        "test",
			Description: "Test prompt",
			Arguments:   []api.PromptArgument{},
			GetMessages: func(arguments map[string]string) []api.PromptMessage {
				return []api.PromptMessage{{Role: "user", Content: "test"}}
			},
		}

		handler := createPromptHandler(nil, prompt)

		// Create request with no params
		request := &mcp.GetPromptRequest{}

		// When
		result, err := handler(context.Background(), request)

		// Then
		require.NoError(t, err)
		require.Len(t, result.Messages, 1)
	})
}
