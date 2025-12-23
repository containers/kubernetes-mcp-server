package kiali

import (
	"context"
	"net/http"
	"net/url"
)

// MeshStatus calls the Kiali mesh graph API to get the status of mesh components.
// This returns information about mesh components like Istio, Kiali, Grafana, Prometheus
// and their interactions, versions, and health status.
func (k *Kiali) MeshStatus(ctx context.Context) (string, error) {
	u, err := url.Parse(MeshGraphEndpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("includeGateways", DefaultIncludeGateways)
	q.Set("includeWaypoints", DefaultIncludeWaypoints)
	u.RawQuery = q.Encode()
	return k.executeRequest(ctx, http.MethodGet, u.String(), "", nil)
}
