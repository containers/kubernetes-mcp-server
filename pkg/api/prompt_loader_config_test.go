package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptLoader_LoadFromConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        interface{}
		expectedCount int
		expectedName  string
		expectError   bool
	}{
		{
			name: "load single prompt from config",
			config: []struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
				Arguments   []struct {
					Name        string `yaml:"name"`
					Description string `yaml:"description"`
					Required    bool   `yaml:"required"`
				} `yaml:"arguments,omitempty"`
				Messages []struct {
					Role    string `yaml:"role"`
					Content string `yaml:"content"`
				} `yaml:"messages"`
			}{
				{
					Name:        "test-prompt",
					Description: "Test prompt description",
					Arguments: []struct {
						Name        string `yaml:"name"`
						Description string `yaml:"description"`
						Required    bool   `yaml:"required"`
					}{
						{Name: "arg1", Description: "Argument 1", Required: true},
					},
					Messages: []struct {
						Role    string `yaml:"role"`
						Content string `yaml:"content"`
					}{
						{Role: "user", Content: "Test content with {{arg1}}"},
					},
				},
			},
			expectedCount: 1,
			expectedName:  "test-prompt",
			expectError:   false,
		},
		{
			name: "load multiple prompts from config",
			config: []struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
				Messages    []struct {
					Role    string `yaml:"role"`
					Content string `yaml:"content"`
				} `yaml:"messages"`
			}{
				{
					Name:        "prompt1",
					Description: "First prompt",
					Messages: []struct {
						Role    string `yaml:"role"`
						Content string `yaml:"content"`
					}{
						{Role: "user", Content: "Message 1"},
					},
				},
				{
					Name:        "prompt2",
					Description: "Second prompt",
					Messages: []struct {
						Role    string `yaml:"role"`
						Content string `yaml:"content"`
					}{
						{Role: "assistant", Content: "Message 2"},
					},
				},
			},
			expectedCount: 2,
			expectedName:  "prompt1",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewPromptLoader()
			err := loader.LoadFromConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			prompts := loader.GetServerPrompts()
			assert.Len(t, prompts, tt.expectedCount)

			if tt.expectedCount > 0 {
				assert.Equal(t, tt.expectedName, prompts[0].Prompt.Name)
			}
		})
	}
}

func TestPromptLoader_LoadFromConfigWithArguments(t *testing.T) {
	config := []struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Arguments   []struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			Required    bool   `yaml:"required"`
		} `yaml:"arguments,omitempty"`
		Messages []struct {
			Role    string `yaml:"role"`
			Content string `yaml:"content"`
		} `yaml:"messages"`
	}{
		{
			Name:        "prompt-with-args",
			Description: "Prompt with arguments",
			Arguments: []struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
				Required    bool   `yaml:"required"`
			}{
				{Name: "required_arg", Description: "A required argument", Required: true},
				{Name: "optional_arg", Description: "An optional argument", Required: false},
			},
			Messages: []struct {
				Role    string `yaml:"role"`
				Content string `yaml:"content"`
			}{
				{Role: "user", Content: "Content with {{required_arg}} and {{optional_arg}}"},
			},
		},
	}

	loader := NewPromptLoader()
	err := loader.LoadFromConfig(config)
	require.NoError(t, err)

	prompts := loader.GetServerPrompts()
	require.Len(t, prompts, 1)

	prompt := prompts[0]
	assert.Equal(t, "prompt-with-args", prompt.Prompt.Name)
	assert.Len(t, prompt.Prompt.Arguments, 2)
	assert.Equal(t, "required_arg", prompt.Prompt.Arguments[0].Name)
	assert.True(t, prompt.Prompt.Arguments[0].Required)
	assert.Equal(t, "optional_arg", prompt.Prompt.Arguments[1].Name)
	assert.False(t, prompt.Prompt.Arguments[1].Required)
}

func TestPromptLoader_LoadFromConfigHandlerExecution(t *testing.T) {
	config := []struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Arguments   []struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			Required    bool   `yaml:"required"`
		} `yaml:"arguments,omitempty"`
		Messages []struct {
			Role    string `yaml:"role"`
			Content string `yaml:"content"`
		} `yaml:"messages"`
	}{
		{
			Name:        "substitution-test",
			Description: "Test argument substitution",
			Arguments: []struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
				Required    bool   `yaml:"required"`
			}{
				{Name: "name", Description: "Name to substitute", Required: true},
			},
			Messages: []struct {
				Role    string `yaml:"role"`
				Content string `yaml:"content"`
			}{
				{Role: "user", Content: "Hello {{name}}!"},
			},
		},
	}

	loader := NewPromptLoader()
	err := loader.LoadFromConfig(config)
	require.NoError(t, err)

	prompts := loader.GetServerPrompts()
	require.Len(t, prompts, 1)

	prompt := prompts[0]

	// Execute the handler
	params := PromptHandlerParams{
		PromptCallRequest: &mockPromptCallRequest{
			args: map[string]string{
				"name": "World",
			},
		},
	}

	result, err := prompt.Handler(params)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Messages, 1)
	assert.Equal(t, "Hello World!", result.Messages[0].Content.Text)
}

// Mock implementation for testing
type mockPromptCallRequest struct {
	args map[string]string
}

func (m *mockPromptCallRequest) GetArguments() map[string]string {
	if m.args == nil {
		return make(map[string]string)
	}
	return m.args
}
