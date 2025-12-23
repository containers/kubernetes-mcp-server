package kiali

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// ServicesList returns the list of services across specified namespaces.
func (k *Kiali) ServicesList(ctx context.Context, namespaces string) (string, error) {
	endpoint := ServicesEndpoint + "?health=true&istioResources=true&rateInterval=" + DefaultRateInterval + "&onlyDefinitions=false"
	if namespaces != "" {
		endpoint += "&namespaces=" + url.QueryEscape(namespaces)
	}

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}

// ServiceDetails returns the details for a specific service in a namespace.
func (k *Kiali) ServiceDetails(ctx context.Context, namespace string, service string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if service == "" {
		return "", fmt.Errorf("service name is required")
	}
	endpoint := fmt.Sprintf(ServiceDetailsEndpoint, url.PathEscape(namespace), url.PathEscape(service)) + "?validate=true&rateInterval=" + DefaultRateInterval

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}

// ServiceMetrics returns the metrics for a specific service in a namespace.
// Parameters:
//   - namespace: the namespace containing the service
//   - service: the name of the service
//   - queryParams: optional query parameters map for filtering metrics (e.g., "duration", "step", "rateInterval", "direction", "reporter", "filters[]", "byLabels[]", etc.)
func (k *Kiali) ServiceMetrics(ctx context.Context, namespace string, service string, queryParams map[string]string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if service == "" {
		return "", fmt.Errorf("service name is required")
	}

	endpoint := fmt.Sprintf(ServiceMetricsEndpoint,
		url.PathEscape(namespace), url.PathEscape(service))

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
