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
	q.Set("duration", queryParams["rateInterval"])
	q.Set("graphType", queryParams["graphType"])
	q.Set("includeIdleEdges", DefaultIncludeIdleEdges)
	q.Set("injectServiceNodes", DefaultInjectServiceNodes)
	q.Set("boxBy", DefaultBoxBy)
	q.Set("ambientTraffic", DefaultAmbientTraffic)
	q.Set("appenders", DefaultAppenders)
	q.Set("rateGrpc", DefaultRateGrpc)
	q.Set("rateHttp", DefaultRateHttp)
	q.Set("rateTcp", DefaultRateTcp)
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
