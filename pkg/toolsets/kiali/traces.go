package kiali

import (
	"fmt"
	"strconv"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initTraces() []api.ServerTool {
	ret := make([]api.ServerTool, 0)

	// App traces tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "app_traces",
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
						Description: "Start time for traces in microseconds since epoch (optional, defaults to 10 minutes before current time if not provided)",
					},
					"endMicros": {
						Type:        "string",
						Description: "End time for traces in microseconds since epoch (optional, defaults to 10 minutes after startMicros if not provided)",
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
			Name:        "service_traces",
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
						Description: "Start time for traces in microseconds since epoch (optional, defaults to 10 minutes before current time if not provided)",
					},
					"endMicros": {
						Type:        "string",
						Description: "End time for traces in microseconds since epoch (optional, defaults to 10 minutes after startMicros if not provided)",
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
			Name:        "workload_traces",
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
						Description: "Start time for traces in microseconds since epoch (optional, defaults to 10 minutes before current time if not provided)",
					},
					"endMicros": {
						Type:        "string",
						Description: "End time for traces in microseconds since epoch (optional, defaults to 10 minutes after startMicros if not provided)",
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

	// Trace details tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "trace_details",
			Description: "Get detailed information for a specific trace by its ID. Returns complete trace information including all spans, timing details, and metadata for in-depth analysis.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"traceId": {
						Type:        "string",
						Description: "Unique identifier of the trace to retrieve",
					},
				},
				Required: []string{"traceId"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Trace: Details",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		},
		Handler: traceDetailsHandler,
	})

	return ret
}

func appTracesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	namespace := params.GetArguments()["namespace"].(string)
	app := params.GetArguments()["app"].(string)

	// Build query parameters from optional arguments
	queryParams := make(map[string]string)

	// Handle startMicros: if not provided, default to 10 minutes ago
	var startMicros string
	if startMicrosVal, ok := params.GetArguments()["startMicros"].(string); ok && startMicrosVal != "" {
		startMicros = startMicrosVal
	} else {
		// Default to 10 minutes before current time
		now := time.Now()
		tenMinutesAgo := now.Add(-10 * time.Minute)
		startMicros = strconv.FormatInt(tenMinutesAgo.UnixMicro(), 10)
	}
	queryParams["startMicros"] = startMicros

	// Handle endMicros: if not provided, default to 10 minutes after startMicros
	var endMicros string
	if endMicrosVal, ok := params.GetArguments()["endMicros"].(string); ok && endMicrosVal != "" {
		endMicros = endMicrosVal
	} else {
		// Parse startMicros to calculate endMicros
		startMicrosInt, err := strconv.ParseInt(startMicros, 10, 64)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("invalid startMicros value: %v", err)), nil
		}
		startTime := time.UnixMicro(startMicrosInt)
		endTime := startTime.Add(10 * time.Minute)
		endMicros = strconv.FormatInt(endTime.UnixMicro(), 10)
	}
	queryParams["endMicros"] = endMicros

	// Handle limit: convert integer to string if provided
	if limit := params.GetArguments()["limit"]; limit != nil {
		switch v := limit.(type) {
		case float64:
			queryParams["limit"] = fmt.Sprintf("%.0f", v)
		case int:
			queryParams["limit"] = fmt.Sprintf("%d", v)
		case int64:
			queryParams["limit"] = fmt.Sprintf("%d", v)
		}
	}

	// Handle minDuration: convert integer to string if provided
	if minDuration := params.GetArguments()["minDuration"]; minDuration != nil {
		switch v := minDuration.(type) {
		case float64:
			queryParams["minDuration"] = fmt.Sprintf("%.0f", v)
		case int:
			queryParams["minDuration"] = fmt.Sprintf("%d", v)
		case int64:
			queryParams["minDuration"] = fmt.Sprintf("%d", v)
		}
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

	// Handle startMicros: if not provided, default to 10 minutes ago
	var startMicros string
	if startMicrosVal, ok := params.GetArguments()["startMicros"].(string); ok && startMicrosVal != "" {
		startMicros = startMicrosVal
	} else {
		// Default to 10 minutes before current time
		now := time.Now()
		tenMinutesAgo := now.Add(-10 * time.Minute)
		startMicros = strconv.FormatInt(tenMinutesAgo.UnixMicro(), 10)
	}
	queryParams["startMicros"] = startMicros

	// Handle endMicros: if not provided, default to 10 minutes after startMicros
	var endMicros string
	if endMicrosVal, ok := params.GetArguments()["endMicros"].(string); ok && endMicrosVal != "" {
		endMicros = endMicrosVal
	} else {
		// Parse startMicros to calculate endMicros
		startMicrosInt, err := strconv.ParseInt(startMicros, 10, 64)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("invalid startMicros value: %v", err)), nil
		}
		startTime := time.UnixMicro(startMicrosInt)
		endTime := startTime.Add(10 * time.Minute)
		endMicros = strconv.FormatInt(endTime.UnixMicro(), 10)
	}
	queryParams["endMicros"] = endMicros

	// Handle limit: convert integer to string if provided
	if limit := params.GetArguments()["limit"]; limit != nil {
		switch v := limit.(type) {
		case float64:
			queryParams["limit"] = fmt.Sprintf("%.0f", v)
		case int:
			queryParams["limit"] = fmt.Sprintf("%d", v)
		case int64:
			queryParams["limit"] = fmt.Sprintf("%d", v)
		}
	}

	// Handle minDuration: convert integer to string if provided
	if minDuration := params.GetArguments()["minDuration"]; minDuration != nil {
		switch v := minDuration.(type) {
		case float64:
			queryParams["minDuration"] = fmt.Sprintf("%.0f", v)
		case int:
			queryParams["minDuration"] = fmt.Sprintf("%d", v)
		case int64:
			queryParams["minDuration"] = fmt.Sprintf("%d", v)
		}
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

	// Handle startMicros: if not provided, default to 10 minutes ago
	var startMicros string
	if startMicrosVal, ok := params.GetArguments()["startMicros"].(string); ok && startMicrosVal != "" {
		startMicros = startMicrosVal
	} else {
		// Default to 10 minutes before current time
		now := time.Now()
		tenMinutesAgo := now.Add(-10 * time.Minute)
		startMicros = strconv.FormatInt(tenMinutesAgo.UnixMicro(), 10)
	}
	queryParams["startMicros"] = startMicros

	// Handle endMicros: if not provided, default to 10 minutes after startMicros
	var endMicros string
	if endMicrosVal, ok := params.GetArguments()["endMicros"].(string); ok && endMicrosVal != "" {
		endMicros = endMicrosVal
	} else {
		// Parse startMicros to calculate endMicros
		startMicrosInt, err := strconv.ParseInt(startMicros, 10, 64)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("invalid startMicros value: %v", err)), nil
		}
		startTime := time.UnixMicro(startMicrosInt)
		endTime := startTime.Add(10 * time.Minute)
		endMicros = strconv.FormatInt(endTime.UnixMicro(), 10)
	}
	queryParams["endMicros"] = endMicros

	// Handle limit: convert integer to string if provided
	if limit := params.GetArguments()["limit"]; limit != nil {
		switch v := limit.(type) {
		case float64:
			queryParams["limit"] = fmt.Sprintf("%.0f", v)
		case int:
			queryParams["limit"] = fmt.Sprintf("%d", v)
		case int64:
			queryParams["limit"] = fmt.Sprintf("%d", v)
		}
	}

	// Handle minDuration: convert integer to string if provided
	if minDuration := params.GetArguments()["minDuration"]; minDuration != nil {
		switch v := minDuration.(type) {
		case float64:
			queryParams["minDuration"] = fmt.Sprintf("%.0f", v)
		case int:
			queryParams["minDuration"] = fmt.Sprintf("%d", v)
		case int64:
			queryParams["minDuration"] = fmt.Sprintf("%d", v)
		}
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

func traceDetailsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract required parameter
	traceId, ok := params.GetArguments()["traceId"].(string)
	if !ok || traceId == "" {
		return api.NewToolCallResult("", fmt.Errorf("traceId parameter is required")), nil
	}

	k := params.NewKiali()
	content, err := k.TraceDetails(params.Context, traceId)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get trace details: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
