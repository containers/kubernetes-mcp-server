package prompts

import (
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// ToServerPrompts converts Prompt definitions to ServerPrompts with handlers
func ToServerPrompts(prompts []api.Prompt) []api.ServerPrompt {
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
			content := substituteArguments(template.Content, prompt.Arguments, args)
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

// substituteArguments replaces {{argument}} placeholders in content with actual values.
// For optional arguments not provided, their placeholders are removed.
func substituteArguments(content string, promptArgs []api.PromptArgument, args map[string]string) string {
	result := content
	for _, promptArg := range promptArgs {
		placeholder := fmt.Sprintf("{{%s}}", promptArg.Name)
		if value, exists := args[promptArg.Name]; exists {
			result = strings.ReplaceAll(result, placeholder, value)
		} else if !promptArg.Required {
			// Remove placeholder for optional arguments not provided
			result = strings.ReplaceAll(result, placeholder, "")
		}
	}
	return result
}

// MergePrompts merges two slices of prompts, with prompts in override taking precedence
// over prompts in base when they have the same name
func MergePrompts(base, override []api.ServerPrompt) []api.ServerPrompt {
	// Create a map of override prompts by name for quick lookup
	overrideMap := make(map[string]api.ServerPrompt)
	for _, prompt := range override {
		overrideMap[prompt.Prompt.Name] = prompt
	}

	// Build result: start with base prompts, skipping any that are overridden
	result := make([]api.ServerPrompt, 0, len(base)+len(override))
	for _, prompt := range base {
		if _, exists := overrideMap[prompt.Prompt.Name]; !exists {
			result = append(result, prompt)
		}
	}

	// Add all override prompts
	result = append(result, override...)

	return result
}
