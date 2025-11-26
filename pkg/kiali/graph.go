package kiali

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// Graph calls the Kiali graph API using the provided Authorization header value.
// `namespaces` may contain zero, one or many namespaces. If empty, the API may return an empty graph
// or the server default, depending on Kiali configuration.
func (k *Kiali) Graph(ctx context.Context, namespaces []string, queryParams map[string]string) (string, error) {
	u, err := url.Parse(GraphEndpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	// Static graph parameters per requirements
	// Defaults with optional overrides via queryParams
	duration := DefaultRateInterval
	graphType := "versionedApp"
	if v, ok := queryParams["rateInterval"]; ok && strings.TrimSpace(v) != "" {
		duration = strings.TrimSpace(v)
	}
	if v, ok := queryParams["graphType"]; ok && strings.TrimSpace(v) != "" {
		graphType = strings.TrimSpace(v)
	}
	q.Set("duration", duration)
	q.Set("graphType", graphType)
	q.Set("includeIdleEdges", "false")
	q.Set("injectServiceNodes", "true")
	q.Set("boxBy", "cluster,namespace,app")
	q.Set("ambientTraffic", "none")
	q.Set("appenders", "deadNode,istio,serviceEntry,meshCheck,workloadEntry,health")
	q.Set("rateGrpc", "requests")
	q.Set("rateHttp", "requests")
	q.Set("rateTcp", "sent")
	// Optional namespaces param
	cleaned := make([]string, 0, len(namespaces))
	for _, ns := range namespaces {
		ns = strings.TrimSpace(ns)
		if ns != "" {
			cleaned = append(cleaned, ns)
		}
	}
	if len(cleaned) > 0 {
		q.Set("namespaces", strings.Join(cleaned, ","))
	}
	u.RawQuery = q.Encode()
	endpoint := u.String()

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}
