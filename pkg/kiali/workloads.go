package kiali

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/containers/kubernetes-mcp-server/pkg/kiali/transforms"
)

// WorkloadsList returns the list of workloads across specified namespaces.
// Response is transformed to workload summaries by cluster (name, namespace, health, type, labels, details, configuration).
func (k *Kiali) WorkloadsList(ctx context.Context, namespaces string) (string, error) {
	endpoint := WorkloadsEndpoint + "?health=true&istioResources=true&rateInterval=" + DefaultRateInterval
	if namespaces != "" {
		endpoint += "&namespaces=" + url.QueryEscape(namespaces)
	}

	raw, err := k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
	if err != nil {
		return "", err
	}

	workloadsByCluster, err := transforms.TransformWorkloadsListResponse(raw)
	if err != nil {
		return raw, err
	}

	jsonBytes, err := json.Marshal(workloadsByCluster)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// WorkloadDetails returns the details for a specific workload in a namespace.
// Response is transformed to a simplified structure (workload, status, istio, pods, associated_services).
func (k *Kiali) WorkloadDetails(ctx context.Context, namespace string, workload string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if workload == "" {
		return "", fmt.Errorf("workload name is required")
	}
	endpoint := fmt.Sprintf(WorkloadDetailsEndpoint, url.PathEscape(namespace), url.PathEscape(workload)) + "?validate=true&rateInterval=" + DefaultRateInterval + "&health=true"

	raw, err := k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
	if err != nil {
		return "", err
	}

	formatted, err := transforms.TransformWorkloadDetailsResponse(raw)
	if err != nil {
		return raw, err
	}

	jsonBytes, err := json.Marshal(formatted)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
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
