package api

import (
	"context"
)

// TargetProvider represents a cluster provider that can manage one or more cluster targets.
// This is the primary interface used by the MCP server.
type TargetProvider interface {
	// IsMultiTarget reports whether the provider is configured for multiple targets.
	// Unlike GetTargets, it does not require a user-scoped context and should be
	// implementable without expensive lookups.
	// Note that GetTargets may return fewer targets than the provider is configured for
	// (e.g. due to user-scoped access restrictions).
	IsMultiTarget() bool
	GetTargets(ctx context.Context) ([]string, error)
	GetDefaultTarget() string
	GetTargetParameterName() string
}
