package kiali

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// ValidationsList calls the Kiali validations API using the provided Authorization header value.
// `namespaces` may contain zero, one or many namespaces. If empty, returns validations from all namespaces.
func (k *Kiali) ValidationsList(ctx context.Context, namespaces []string) (string, error) {
	// Add namespaces query parameter if any provided
	cleaned := make([]string, 0, len(namespaces))
	endpoint := ValidationsEndpoint
	for _, ns := range namespaces {
		ns = strings.TrimSpace(ns)
		if ns != "" {
			cleaned = append(cleaned, ns)
		}
	}
	if len(cleaned) > 0 {
		u, err := url.Parse(endpoint)
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Set("namespaces", strings.Join(cleaned, ","))
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}
