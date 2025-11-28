package api

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// PromptDefinition represents a prompt definition loaded from config
type PromptDefinition struct {
	Name        string                  `yaml:"name"`
	Description string                  `yaml:"description"`
	Arguments   []PromptArgumentDef     `yaml:"arguments,omitempty"`
	Messages    []PromptMessageTemplate `yaml:"messages"`
}

// PromptArgumentDef represents an argument definition
type PromptArgumentDef struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

// PromptMessageTemplate represents a message template
type PromptMessageTemplate struct {
	Role    string `yaml:"role"`
	Content string `yaml:"content"`
}

// PromptLoader loads prompt definitions from TOML config
type PromptLoader struct {
	definitions []PromptDefinition
}

// NewPromptLoader creates a new prompt loader
func NewPromptLoader() *PromptLoader {
	return &PromptLoader{
		definitions: make([]PromptDefinition, 0),
	}
}

// GetServerPrompts converts loaded definitions to ServerPrompt instances
func (l *PromptLoader) GetServerPrompts() []ServerPrompt {
	prompts := make([]ServerPrompt, 0, len(l.definitions))
	for _, def := range l.definitions {
		prompts = append(prompts, l.convertToServerPrompt(def))
	}
	return prompts
}

// convertToServerPrompt converts a PromptDefinition to a ServerPrompt
func (l *PromptLoader) convertToServerPrompt(def PromptDefinition) ServerPrompt {
	arguments := make([]PromptArgument, 0, len(def.Arguments))
	for _, arg := range def.Arguments {
		arguments = append(arguments, PromptArgument(arg))
	}

	return ServerPrompt{
		Prompt: Prompt{
			Name:        def.Name,
			Description: def.Description,
			Arguments:   arguments,
		},
		Handler: l.createHandler(def),
	}
}

// createHandler creates a prompt handler function for a prompt definition
func (l *PromptLoader) createHandler(def PromptDefinition) PromptHandlerFunc {
	return func(params PromptHandlerParams) (*PromptCallResult, error) {
		args := params.GetArguments()

		// Validate required arguments
		for _, argDef := range def.Arguments {
			if argDef.Required {
				if _, exists := args[argDef.Name]; !exists {
					return nil, fmt.Errorf("required argument '%s' is missing", argDef.Name)
				}
			}
		}

		// Render messages with argument substitution
		messages := make([]PromptMessage, 0, len(def.Messages))
		for _, msgTemplate := range def.Messages {
			content := l.substituteArguments(msgTemplate.Content, args)
			messages = append(messages, PromptMessage{
				Role: msgTemplate.Role,
				Content: PromptContent{
					Type: "text",
					Text: content,
				},
			})
		}

		return NewPromptCallResult(def.Description, messages, nil), nil
	}
}

// substituteArguments replaces {{argument}} placeholders in content with actual values
func (l *PromptLoader) substituteArguments(content string, args map[string]string) string {
	result := content
	for key, value := range args {
		placeholder := fmt.Sprintf("{{%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// LoadFromConfig loads prompts from TOML config structures
func (l *PromptLoader) LoadFromConfig(configs interface{}) error {
	// Type assertion to handle the config package types
	// We use interface{} here to avoid circular dependency with config package
	var defs []PromptDefinition

	// Use reflection or type switching to convert config types to PromptDefinition
	// This is a simple implementation that works with the expected structure
	data, err := convertToYAML(configs)
	if err != nil {
		return fmt.Errorf("failed to convert config to YAML: %w", err)
	}

	if err := yaml.Unmarshal(data, &defs); err != nil {
		return fmt.Errorf("failed to parse prompt config: %w", err)
	}

	l.definitions = append(l.definitions, defs...)
	return nil
}

// convertToYAML converts config structs to YAML bytes for uniform processing
func convertToYAML(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}
