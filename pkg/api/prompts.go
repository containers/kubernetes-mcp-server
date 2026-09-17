package api

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// ServerPrompt represents a prompt that can be registered with the MCP server.
// Prompts provide pre-defined workflow templates and guidance to AI assistants.
type ServerPrompt struct {
	Prompt         config.Prompt
	RBAC           *RBACMetadata
	Handler        PromptHandlerFunc
	ClusterAware   *bool
	ArgumentSchema map[string]config.PromptArgument
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

// PromptMessage represents a single message in a prompt response.
// See MCP specification: https://spec.modelcontextprotocol.io/specification/server/prompts/
type PromptMessage struct {
	Role    string        `json:"role" toml:"role"`
	Content PromptContent `json:"content" toml:"content"`
}

// PromptContent represents the content of a prompt message.
// See MCP specification: https://spec.modelcontextprotocol.io/specification/server/prompts/
type PromptContent struct {
	Type string `json:"type" toml:"type"`
	Text string `json:"text,omitempty" toml:"text,omitempty"`
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
	Config                  *config.Config
	ClusterProviderStrategy string
	KubernetesClient
	PromptCallRequest
	Elicitor
}

// PromptHandlerFunc is a function that handles prompt execution
type PromptHandlerFunc func(params PromptHandlerParams) (*PromptCallResult, error)
