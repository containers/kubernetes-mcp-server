package discovery

import (
	"context"

	"k8s.io/client-go/dynamic"
)

// EndpointResolver resolves HTTPS URLs for named service endpoints.
// Implementations may use OpenShift Routes, Ingress, or other mechanisms.
// A nil EndpointResolver is valid; callers fall back to service DNS.
type EndpointResolver interface {
	ResolveEndpoint(ctx context.Context, client dynamic.Interface, namespace, routeName string) (string, error)
}
