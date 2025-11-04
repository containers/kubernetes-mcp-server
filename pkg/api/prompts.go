package api

import (
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// ServerPrompt represents a prompt that can be provided to the MCP server
type ServerPrompt struct {
	Name        string
	Description string
	Arguments   []PromptArgument
	GetMessages func(arguments map[string]string) []PromptMessage
}

// PromptArgument defines an argument that can be passed to a prompt
type PromptArgument struct {
	Name        string
	Description string
	Required    bool
}

// PromptMessage represents a message in a prompt
type PromptMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// PromptSet groups related prompts together
type PromptSet interface {
	// GetName returns the name of the prompt set
	GetName() string
	// GetDescription returns a description of what this prompt set provides
	GetDescription() string
	// GetPrompts returns all prompts in this set
	GetPrompts(o internalk8s.Openshift) []ServerPrompt
}
