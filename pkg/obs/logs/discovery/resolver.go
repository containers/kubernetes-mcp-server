package discovery

import (
	"context"

	"k8s.io/client-go/dynamic"
)

// GatewayResolver resolves base URLs for LokiStack gateway endpoints.
// Implementations may use OpenShift Routes, Ingress, or other mechanisms.
// A nil GatewayResolver is valid; callers fall back to HTTP service DNS.
type GatewayResolver interface {
	ResolveGatewayURL(ctx context.Context, client dynamic.Interface, namespace, stackName, tenantsMode string) (string, error)
}
