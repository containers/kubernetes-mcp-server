package kiali

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// AppTraces returns distributed tracing data for a specific app in a namespace.
// Parameters:
//   - namespace: the namespace containing the app
//   - app: the name of the app
//   - queryParams: optional query parameters map for filtering traces (e.g., "startMicros", "endMicros", "limit", "minDuration", "tags", "clusterName")
func (k *Kiali) AppTraces(ctx context.Context, namespace string, app string, queryParams map[string]string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if app == "" {
		return "", fmt.Errorf("app name is required")
	}

	endpoint := fmt.Sprintf(AppTracesEndpoint,
		url.PathEscape(namespace), url.PathEscape(app))

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

// ServiceTraces returns distributed tracing data for a specific service in a namespace.
// Parameters:
//   - namespace: the namespace containing the service
//   - service: the name of the service
//   - queryParams: optional query parameters map for filtering traces (e.g., "startMicros", "endMicros", "limit", "minDuration", "tags", "clusterName")
func (k *Kiali) ServiceTraces(ctx context.Context, namespace string, service string, queryParams map[string]string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if service == "" {
		return "", fmt.Errorf("service name is required")
	}

	endpoint := fmt.Sprintf(ServiceTracesEndpoint,
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

// WorkloadTraces returns distributed tracing data for a specific workload in a namespace.
// Parameters:
//   - namespace: the namespace containing the workload
//   - workload: the name of the workload
//   - queryParams: optional query parameters map for filtering traces (e.g., "startMicros", "endMicros", "limit", "minDuration", "tags", "clusterName")
func (k *Kiali) WorkloadTraces(ctx context.Context, namespace string, workload string, queryParams map[string]string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if workload == "" {
		return "", fmt.Errorf("workload name is required")
	}

	endpoint := fmt.Sprintf(WorkloadTracesEndpoint,
		url.PathEscape(namespace), url.PathEscape(workload))

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

// TraceDetails returns detailed information for a specific trace by its ID.
// Parameters:
//   - traceId: the unique identifier of the trace
func (k *Kiali) TraceDetails(ctx context.Context, traceId string) (string, error) {
	if traceId == "" {
		return "", fmt.Errorf("trace ID is required")
	}

	endpoint := fmt.Sprintf("/api/traces/%s", url.PathEscape(traceId))

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}
