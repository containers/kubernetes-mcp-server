package mcp

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func TestPromptCallRequestAdapter_GetArguments(t *testing.T) {
	tests := []struct {
		name    string
		request *mcp.GetPromptRequest
		want    map[string]string
	}{
		{
			name:    "nil request",
			request: nil,
			want:    map[string]string{},
		},
		{
			name: "nil params",
			request: &mcp.GetPromptRequest{
				Params: nil,
			},
			want: map[string]string{},
		},
		{
			name: "nil arguments",
			request: &mcp.GetPromptRequest{
				Params: &mcp.GetPromptParams{
					Arguments: nil,
				},
			},
			want: map[string]string{},
		},
		{
			name: "with arguments",
			request: &mcp.GetPromptRequest{
				Params: &mcp.GetPromptParams{
					Arguments: map[string]string{
						"namespace": "default",
						"pod_name":  "test-pod",
					},
				},
			},
			want: map[string]string{
				"namespace": "default",
				"pod_name":  "test-pod",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &promptCallRequestAdapter{request: tt.request}
			got := adapter.GetArguments()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServerPromptToGoSdkPrompt_Conversion(t *testing.T) {
	serverPrompt := api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        "test-prompt",
			Description: "Test description",
			Arguments: []api.PromptArgument{
				{
					Name:        "arg1",
					Description: "First argument",
					Required:    true,
				},
				{
					Name:        "arg2",
					Description: "Second argument",
					Required:    false,
				},
			},
		},
		Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
			return api.NewPromptCallResult("Test result", []api.PromptMessage{
				{
					Role: "user",
					Content: api.PromptContent{
						Type: "text",
						Text: "Test message",
					},
				},
			}, nil), nil
		},
	}

	mockServer := &Server{}

	mcpPrompt, handler, err := ServerPromptToGoSdkPrompt(mockServer, serverPrompt)

	require.NoError(t, err)
	require.NotNil(t, mcpPrompt)
	require.NotNil(t, handler)

	assert.Equal(t, "test-prompt", mcpPrompt.Name)
	assert.Equal(t, "Test description", mcpPrompt.Description)
	require.Len(t, mcpPrompt.Arguments, 2)

	assert.Equal(t, "arg1", mcpPrompt.Arguments[0].Name)
	assert.Equal(t, "First argument", mcpPrompt.Arguments[0].Description)
	assert.True(t, mcpPrompt.Arguments[0].Required)

	assert.Equal(t, "arg2", mcpPrompt.Arguments[1].Name)
	assert.Equal(t, "Second argument", mcpPrompt.Arguments[1].Description)
	assert.False(t, mcpPrompt.Arguments[1].Required)
}

func TestServerPromptToGoSdkPrompt_EmptyArguments(t *testing.T) {
	serverPrompt := api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        "no-args-prompt",
			Description: "Prompt with no arguments",
			Arguments:   []api.PromptArgument{},
		},
		Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
			return api.NewPromptCallResult("Result", []api.PromptMessage{}, nil), nil
		},
	}

	mockServer := &Server{}

	mcpPrompt, handler, err := ServerPromptToGoSdkPrompt(mockServer, serverPrompt)

	require.NoError(t, err)
	require.NotNil(t, mcpPrompt)
	require.NotNil(t, handler)

	assert.Equal(t, "no-args-prompt", mcpPrompt.Name)
	assert.Len(t, mcpPrompt.Arguments, 0)
}
