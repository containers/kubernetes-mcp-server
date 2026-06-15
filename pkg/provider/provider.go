package provider

import (
	"context"
)

// TargetManager represents a manager for a single cluster target.
// It provides target-specific discovery capabilities for things
// like tool filtering.
type TargetManager interface {
	// IsOpenShift reports whether the target is an OpenShift cluster.
	IsOpenShift(ctx context.Context) bool
}

// ManagerProvider manages one or more cluster targets.
// This interface provides the minimal surface needed by toolsets
// for target compatibility checking and tool filtering.
type ManagerProvider interface {
	// GetTargetManagers returns managers for all targets.
	// Returns an error if managers for any target cannot be retrieved.
	GetTargetManagers(ctx context.Context) ([]TargetManager, error)
}
