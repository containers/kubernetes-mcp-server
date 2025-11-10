package kiali

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// WorkloadsList returns the list of workloads across specified namespaces.
func (k *Kiali) WorkloadsList(ctx context.Context, namespaces string) (string, error) {

	endpoint := WorkloadsEndpoint + "?health=true&istioResources=true&rateInterval=60s"
	if namespaces != "" {
		endpoint += "&namespaces=" + url.QueryEscape(namespaces)
	}

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}

// WorkloadDetails returns the details for a specific workload in a namespace.
func (k *Kiali) WorkloadDetails(ctx context.Context, namespace string, workload string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if workload == "" {
		return "", fmt.Errorf("workload name is required")
	}
	endpoint := fmt.Sprintf(WorkloadDetailsEndpoint, url.PathEscape(namespace), url.PathEscape(workload)) + "?validate=true&rateInterval=60s&health=true"

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}

// WorkloadMetrics returns the metrics for a specific workload in a namespace.
// Parameters:
//   - namespace: the namespace containing the workload
//   - workload: the name of the workload
//   - queryParams: optional query parameters map for filtering metrics (e.g., "duration", "step", "rateInterval", "direction", "reporter", "filters[]", "byLabels[]", etc.)
func (k *Kiali) WorkloadMetrics(ctx context.Context, namespace string, workload string, queryParams map[string]string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if workload == "" {
		return "", fmt.Errorf("workload name is required")
	}

	endpoint := fmt.Sprintf(WorkloadMetricsEndpoint, url.PathEscape(namespace), url.PathEscape(workload))

	// Add query parameters if provided
	if len(queryParams) > 0 {
		u, err := url.Parse(endpoint)
		if err != nil {
			return "", err
		}
		q := u.Query()
		for key, value := range queryParams {
			q.Set(key, value)
		}
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}
