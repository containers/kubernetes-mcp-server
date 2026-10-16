package api

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

// HandlerParams contains the environment shared by every MCP handler and the
// request type specific to that handler.
type HandlerParams[T any] struct {
	context.Context
	Config *config.Config
	KubernetesClient
	Request    T
	ListOutput output.Output
	Elicitor
	// Target is the cluster target the call runs against (the provider's
	// target name; empty for a single-cluster provider).
	Target string
}

// GetToolsetConfig returns the parsed configuration for a toolset.
func (p HandlerParams[T]) GetToolsetConfig(name string) (config.ExtendedConfig, bool) {
	return p.Config.GetToolsetConfig(name)
}
