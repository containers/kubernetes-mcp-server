package api

import (
	"context"

	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// ServerPrompt represents a prompt that can be registered with the MCP server.
// Prompts provide pre-defined workflow templates and guidance to AI assistants.
type ServerPrompt struct {
	Prompt         Prompt
	Handler        PromptHandlerFunc
	ClusterAware   *bool
	ArgumentSchema map[string]PromptArgument
}

// IsClusterAware indicates whether the prompt can accept a "cluster" or "context" parameter
// to operate on a specific Kubernetes cluster context.
// Defaults to true if not explicitly set
func (s *ServerPrompt) IsClusterAware() bool {
	if s.ClusterAware != nil {
		return *s.ClusterAware
	}
	return true
}

// Prompt represents the metadata and content of an MCP prompt
type Prompt struct {
	Name        string           `yaml:"name" json:"name"`
	Description string           `yaml:"description" json:"description,omitempty"`
	Arguments   []PromptArgument `yaml:"arguments,omitempty" json:"arguments,omitempty"`
}

// PromptArgument defines a parameter that can be passed to a prompt
type PromptArgument struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description,omitempty"`
	Required    bool   `yaml:"required" json:"required"`
}

// PromptMessage represents a single message in a prompt template
type PromptMessage struct {
	Role    string        `yaml:"role" json:"role"`
	Content PromptContent `yaml:"content" json:"content"`
}

// PromptContent represents the content of a prompt message
type PromptContent struct {
	Type string `yaml:"type" json:"type"`
	Text string `yaml:"text,omitempty" json:"text,omitempty"`
}

// PromptCallRequest interface for accessing prompt call arguments
type PromptCallRequest interface {
	GetArguments() map[string]string
}

// PromptCallResult represents the result of executing a prompt
type PromptCallResult struct {
	Description string
	Messages    []PromptMessage
	Error       error
}

// NewPromptCallResult creates a new PromptCallResult
func NewPromptCallResult(description string, messages []PromptMessage, err error) *PromptCallResult {
	return &PromptCallResult{
		Description: description,
		Messages:    messages,
		Error:       err,
	}
}

// PromptHandlerParams contains the parameters passed to a prompt handler
type PromptHandlerParams struct {
	context.Context
	*internalk8s.Kubernetes
	PromptCallRequest
}

// PromptHandlerFunc is a function that handles prompt execution
type PromptHandlerFunc func(params PromptHandlerParams) (*PromptCallResult, error)
