package kiali

import (
	"context"
	"net/http"
)

// ListNamespaces calls the Kiali namespaces API using the provided Authorization header value.
// Returns all namespaces in the mesh that the user has access to.
func (k *Kiali) ListNamespaces(ctx context.Context) (string, error) {
	return k.executeRequest(ctx, http.MethodGet, NamespacesEndpoint, "", nil)
}
