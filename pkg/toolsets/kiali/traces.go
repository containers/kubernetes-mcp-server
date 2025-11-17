package kiali

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initTraces() []api.ServerTool {
	ret := make([]api.ServerTool, 0)

	// App traces tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "kiali_app_traces",
			Description: "Get distributed tracing data for a specific app in a namespace. Returns trace information including spans, duration, and error details for troubleshooting and performance analysis.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the app",
					},
					"app": {
						Type:        "string",
						Description: "Name of the app to get traces for",
					},
					"startMicros": {
						Type:        "string",
						Description: "Start time for traces in microseconds since epoch (optional)",
					},
					"endMicros": {
						Type:        "string",
						Description: "End time for traces in microseconds since epoch (optional)",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of traces to return (default: 100)",
						Minimum:     ptr.To(float64(1)),
					},
					"minDuration": {
						Type:        "integer",
						Description: "Minimum trace duration in microseconds (optional)",
						Minimum:     ptr.To(float64(0)),
					},
					"tags": {
						Type:        "string",
						Description: "JSON string of tags to filter traces (optional)",
					},
					"clusterName": {
						Type:        "string",
						Description: "Cluster name for multi-cluster environments (optional)",
					},
				},
				Required: []string{"namespace", "app"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "App: Traces",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		},
		Handler: appTracesHandler,
	})

	// Service traces tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "kiali_service_traces",
			Description: "Get distributed tracing data for a specific service in a namespace. Returns trace information including spans, duration, and error details for troubleshooting and performance analysis.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the service",
					},
					"service": {
						Type:        "string",
						Description: "Name of the service to get traces for",
					},
					"startMicros": {
						Type:        "string",
						Description: "Start time for traces in microseconds since epoch (optional)",
					},
					"endMicros": {
						Type:        "string",
						Description: "End time for traces in microseconds since epoch (optional)",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of traces to return (default: 100)",
						Minimum:     ptr.To(float64(1)),
					},
					"minDuration": {
						Type:        "integer",
						Description: "Minimum trace duration in microseconds (optional)",
						Minimum:     ptr.To(float64(0)),
					},
					"tags": {
						Type:        "string",
						Description: "JSON string of tags to filter traces (optional)",
					},
					"clusterName": {
						Type:        "string",
						Description: "Cluster name for multi-cluster environments (optional)",
					},
				},
				Required: []string{"namespace", "service"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Service: Traces",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		},
		Handler: serviceTracesHandler,
	})

	// Workload traces tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "kiali_workload_traces",
			Description: "Get distributed tracing data for a specific workload in a namespace. Returns trace information including spans, duration, and error details for troubleshooting and performance analysis.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the workload",
					},
					"workload": {
						Type:        "string",
						Description: "Name of the workload to get traces for",
					},
					"startMicros": {
						Type:        "string",
						Description: "Start time for traces in microseconds since epoch (optional)",
					},
					"endMicros": {
						Type:        "string",
						Description: "End time for traces in microseconds since epoch (optional)",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of traces to return (default: 100)",
						Minimum:     ptr.To(float64(1)),
					},
					"minDuration": {
						Type:        "integer",
						Description: "Minimum trace duration in microseconds (optional)",
						Minimum:     ptr.To(float64(0)),
					},
					"tags": {
						Type:        "string",
						Description: "JSON string of tags to filter traces (optional)",
					},
					"clusterName": {
						Type:        "string",
						Description: "Cluster name for multi-cluster environments (optional)",
					},
				},
				Required: []string{"namespace", "workload"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Workload: Traces",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		},
		Handler: workloadTracesHandler,
	})

	return ret
}

func appTracesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	namespace := params.GetArguments()["namespace"].(string)
	app := params.GetArguments()["app"].(string)

	// Build query parameters from optional arguments
	queryParams := make(map[string]string)
	if startMicros, ok := params.GetArguments()["startMicros"].(string); ok && startMicros != "" {
		queryParams["startMicros"] = startMicros
	}
	if endMicros, ok := params.GetArguments()["endMicros"].(string); ok && endMicros != "" {
		queryParams["endMicros"] = endMicros
	}
	if limit, ok := params.GetArguments()["limit"].(string); ok && limit != "" {
		queryParams["limit"] = limit
	}
	if minDuration, ok := params.GetArguments()["minDuration"].(string); ok && minDuration != "" {
		queryParams["minDuration"] = minDuration
	}
	if tags, ok := params.GetArguments()["tags"].(string); ok && tags != "" {
		queryParams["tags"] = tags
	}
	if clusterName, ok := params.GetArguments()["clusterName"].(string); ok && clusterName != "" {
		queryParams["clusterName"] = clusterName
	}
	k := params.NewKiali()
	content, err := k.AppTraces(params.Context, namespace, app, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get app traces: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func serviceTracesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	namespace := params.GetArguments()["namespace"].(string)
	service := params.GetArguments()["service"].(string)

	// Build query parameters from optional arguments
	queryParams := make(map[string]string)
	if startMicros, ok := params.GetArguments()["startMicros"].(string); ok && startMicros != "" {
		queryParams["startMicros"] = startMicros
	}
	if endMicros, ok := params.GetArguments()["endMicros"].(string); ok && endMicros != "" {
		queryParams["endMicros"] = endMicros
	}
	if limit, ok := params.GetArguments()["limit"].(string); ok && limit != "" {
		queryParams["limit"] = limit
	}
	if minDuration, ok := params.GetArguments()["minDuration"].(string); ok && minDuration != "" {
		queryParams["minDuration"] = minDuration
	}
	if tags, ok := params.GetArguments()["tags"].(string); ok && tags != "" {
		queryParams["tags"] = tags
	}
	if clusterName, ok := params.GetArguments()["clusterName"].(string); ok && clusterName != "" {
		queryParams["clusterName"] = clusterName
	}

	k := params.NewKiali()
	content, err := k.ServiceTraces(params.Context, namespace, service, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get service traces: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func workloadTracesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	namespace := params.GetArguments()["namespace"].(string)
	workload := params.GetArguments()["workload"].(string)

	// Build query parameters from optional arguments
	queryParams := make(map[string]string)
	if startMicros, ok := params.GetArguments()["startMicros"].(string); ok && startMicros != "" {
		queryParams["startMicros"] = startMicros
	}
	if endMicros, ok := params.GetArguments()["endMicros"].(string); ok && endMicros != "" {
		queryParams["endMicros"] = endMicros
	}
	if limit, ok := params.GetArguments()["limit"].(string); ok && limit != "" {
		queryParams["limit"] = limit
	}
	if minDuration, ok := params.GetArguments()["minDuration"].(string); ok && minDuration != "" {
		queryParams["minDuration"] = minDuration
	}
	if tags, ok := params.GetArguments()["tags"].(string); ok && tags != "" {
		queryParams["tags"] = tags
	}
	if clusterName, ok := params.GetArguments()["clusterName"].(string); ok && clusterName != "" {
		queryParams["clusterName"] = clusterName
	}

	k := params.NewKiali()
	content, err := k.WorkloadTraces(params.Context, namespace, workload, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get workload traces: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
