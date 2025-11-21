package kiali

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// Health returns health status for apps, workloads, and services across namespaces.
// Parameters:
//   - namespaces: comma-separated list of namespaces (optional, if empty returns health for all accessible namespaces)
//   - queryParams: optional query parameters map for filtering health data (e.g., "type", "rateInterval", "queryTime")
//   - type: health type - "app", "service", or "workload" (default: "app")
//   - rateInterval: rate interval for fetching error rate (default: "1m")
//   - queryTime: Unix timestamp for the prometheus query (optional)
func (k *Kiali) Health(ctx context.Context, namespaces string, queryParams map[string]string) (string, error) {
	// Build query parameters
	u, err := url.Parse(HealthEndpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()

	// Add namespaces if provided
	if namespaces != "" {
		q.Set("namespaces", namespaces)
	}

	// Add optional query parameters
	if len(queryParams) > 0 {
		for key, value := range queryParams {
			q.Set(key, value)
		}
	}

	// Ensure health "type" aligns with graphType (versionedApp -> app)
	healthType := "app"
	if gt, ok := queryParams["graphType"]; ok && strings.TrimSpace(gt) != "" {
		v := strings.TrimSpace(gt)
		if strings.EqualFold(v, "versionedApp") {
			healthType = "app"
		} else {
			healthType = v
		}
	}
	q.Set("type", healthType)

	u.RawQuery = q.Encode()
	endpoint := u.String()

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}
