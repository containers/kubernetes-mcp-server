package prompts

import (
	"context"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

var configPrompts []api.ServerPrompt

// Clear removes all registered prompts
func Clear() {
	configPrompts = []api.ServerPrompt{}
}

// Register registers prompts to be available in the MCP server
func Register(prompts ...api.ServerPrompt) {
	configPrompts = append(configPrompts, prompts...)
}

// ConfigPrompts returns all prompts loaded from configuration
func ConfigPrompts() []api.ServerPrompt {
	return configPrompts
}

// LoadFromToml parses prompts from TOML configuration data and registers them
func LoadFromToml(ctx context.Context, primitive toml.Primitive, md toml.MetaData) error {
	var prompts []api.Prompt
	if err := md.PrimitiveDecode(primitive, &prompts); err != nil {
		return fmt.Errorf("failed to parse prompts from TOML: %w", err)
	}

	serverPrompts := createServerPrompts(prompts)
	Register(serverPrompts...)
	return nil
}

// createServerPrompts converts Prompt definitions to ServerPrompts with handlers
func createServerPrompts(prompts []api.Prompt) []api.ServerPrompt {
	serverPrompts := make([]api.ServerPrompt, 0, len(prompts))
	for _, prompt := range prompts {
		serverPrompts = append(serverPrompts, api.ServerPrompt{
			Prompt:  prompt,
			Handler: createPromptHandler(prompt),
		})
	}
	return serverPrompts
}

// createPromptHandler creates a handler function for a prompt
func createPromptHandler(prompt api.Prompt) api.PromptHandlerFunc {
	return func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
		args := params.GetArguments()

		// Validate required arguments
		for _, arg := range prompt.Arguments {
			if arg.Required {
				if _, exists := args[arg.Name]; !exists {
					return nil, fmt.Errorf("required argument '%s' is missing", arg.Name)
				}
			}
		}

		// Render messages with argument substitution
		messages := make([]api.PromptMessage, 0, len(prompt.Templates))
		for _, template := range prompt.Templates {
			content := substituteArguments(template.Content, args)
			messages = append(messages, api.PromptMessage{
				Role: template.Role,
				Content: api.PromptContent{
					Type: "text",
					Text: content,
				},
			})
		}

		return api.NewPromptCallResult(prompt.Description, messages, nil), nil
	}
}

// substituteArguments replaces {{argument}} placeholders in content with actual values
func substituteArguments(content string, args map[string]string) string {
	result := content
	for key, value := range args {
		placeholder := fmt.Sprintf("{{%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}
