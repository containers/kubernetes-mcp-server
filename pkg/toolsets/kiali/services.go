package kiali

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initServices() []api.ServerTool {
	ret := make([]api.ServerTool, 0)

	// Services list tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "kiali_services_list",
			Description: "Get all services in the mesh across specified namespaces with health and Istio resource information",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespaces": {
						Type:        "string",
						Description: "Comma-separated list of namespaces to get services from (e.g. 'bookinfo' or 'bookinfo,default'). If not provided, will list services from all accessible namespaces",
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Services: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: servicesListHandler,
	})

	// Service details tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "kiali_service_details",
			Description: "Get detailed information for a specific service in a namespace, including validation, health status, and configuration",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the service",
					},
					"service": {
						Type:        "string",
						Description: "Name of the service to get details for",
					},
				},
				Required: []string{"namespace", "service"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Service: Details",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: serviceDetailsHandler,
	})

	// Service metrics tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "kiali_service_metrics",
			Description: "Get metrics for a specific service in a namespace. Supports filtering by time range, direction (inbound/outbound), reporter, and other query parameters",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the service",
					},
					"service": {
						Type:        "string",
						Description: "Name of the service to get metrics for",
					},
					"duration": {
						Type:        "string",
						Description: "Duration of the query period in seconds (e.g., '1800' for 30 minutes). Optional, defaults to 1800 seconds",
					},
					"step": {
						Type:        "string",
						Description: "Step between data points in seconds (e.g., '15'). Optional, defaults to 15 seconds",
					},
					"rateInterval": {
						Type:        "string",
						Description: "Rate interval for metrics (e.g., '1m', '5m'). Optional, defaults to '1m'",
					},
					"direction": {
						Type:        "string",
						Description: "Traffic direction: 'inbound' or 'outbound'. Optional, defaults to 'outbound'",
					},
					"reporter": {
						Type:        "string",
						Description: "Metrics reporter: 'source', 'destination', or 'both'. Optional, defaults to 'source'",
					},
					"requestProtocol": {
						Type:        "string",
						Description: "Filter by request protocol (e.g., 'http', 'grpc', 'tcp'). Optional",
					},
					"quantiles": {
						Type:        "string",
						Description: "Comma-separated list of quantiles for histogram metrics (e.g., '0.5,0.95,0.99'). Optional",
					},
					"byLabels": {
						Type:        "string",
						Description: "Comma-separated list of labels to group metrics by (e.g., 'source_workload,destination_service'). Optional",
					},
				},
				Required: []string{"namespace", "service"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Service: Metrics",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: serviceMetricsHandler,
	})

	return ret
}

func servicesListHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	namespaces, _ := params.GetArguments()["namespaces"].(string)

	k := params.NewKiali()
	content, err := k.ServicesList(params.Context, namespaces)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list services: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func serviceDetailsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	namespace, _ := params.GetArguments()["namespace"].(string)
	service, _ := params.GetArguments()["service"].(string)

	if namespace == "" {
		return api.NewToolCallResult("", fmt.Errorf("namespace parameter is required")), nil
	}
	if service == "" {
		return api.NewToolCallResult("", fmt.Errorf("service parameter is required")), nil
	}

	k := params.NewKiali()
	content, err := k.ServiceDetails(params.Context, namespace, service)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get service details: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func serviceMetricsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract required parameters
	namespace, _ := params.GetArguments()["namespace"].(string)
	service, _ := params.GetArguments()["service"].(string)

	if namespace == "" {
		return api.NewToolCallResult("", fmt.Errorf("namespace parameter is required")), nil
	}
	if service == "" {
		return api.NewToolCallResult("", fmt.Errorf("service parameter is required")), nil
	}

	// Extract optional query parameters
	queryParams := make(map[string]string)
	if duration, ok := params.GetArguments()["duration"].(string); ok && duration != "" {
		queryParams["duration"] = duration
	}
	if step, ok := params.GetArguments()["step"].(string); ok && step != "" {
		queryParams["step"] = step
	}
	if rateInterval, ok := params.GetArguments()["rateInterval"].(string); ok && rateInterval != "" {
		queryParams["rateInterval"] = rateInterval
	}
	if direction, ok := params.GetArguments()["direction"].(string); ok && direction != "" {
		queryParams["direction"] = direction
	}
	if reporter, ok := params.GetArguments()["reporter"].(string); ok && reporter != "" {
		queryParams["reporter"] = reporter
	}
	if requestProtocol, ok := params.GetArguments()["requestProtocol"].(string); ok && requestProtocol != "" {
		queryParams["requestProtocol"] = requestProtocol
	}
	if quantiles, ok := params.GetArguments()["quantiles"].(string); ok && quantiles != "" {
		queryParams["quantiles"] = quantiles
	}
	if byLabels, ok := params.GetArguments()["byLabels"].(string); ok && byLabels != "" {
		queryParams["byLabels"] = byLabels
	}

	k := params.NewKiali()
	content, err := k.ServiceMetrics(params.Context, namespace, service, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get service metrics: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
