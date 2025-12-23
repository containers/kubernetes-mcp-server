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
//   - rateInterval: rate interval for fetching error rate (default: DefaultRateInterval, which is "10m")
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

	// Ensure health "type" aligns with graphType (versionedApp -> app, mesh -> app)
	// The Kiali health API only accepts "app", "service", or "workload" as valid types
	healthType := "app"
	if gt, ok := queryParams["graphType"]; ok && strings.TrimSpace(gt) != "" {
		v := strings.TrimSpace(gt)
		if strings.EqualFold(v, "versionedApp") {
			healthType = "app"
		} else if v == "workload" || v == "service" {
			healthType = v
		} else {
			// For "mesh" or any other graphType, default to "app"
			healthType = "app"
		}
	}
	q.Set("type", healthType)

	u.RawQuery = q.Encode()
	endpoint := u.String()

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}
