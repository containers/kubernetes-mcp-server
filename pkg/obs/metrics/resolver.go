package metrics

import (
	"context"

	"k8s.io/client-go/dynamic"
)

// BackendResolver resolves URLs for metrics infrastructure endpoints.
// Implementations may use OpenShift Routes, Ingress, or other mechanisms.
// A nil BackendResolver is valid; callers fall back to environment variables or defaults.
type BackendResolver interface {
	ResolveMetricsBackend(ctx context.Context, client dynamic.Interface, backend string) (string, error)
	ResolveAlertmanager(ctx context.Context, client dynamic.Interface) (string, error)
}
