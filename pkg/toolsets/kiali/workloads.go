package kiali

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initWorkloads() []api.ServerTool {
	ret := make([]api.ServerTool, 0)

	// Workloads list tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "workloads_list",
			Description: "Get all workloads in the mesh across specified namespaces with health and Istio resource information",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespaces": {
						Type:        "string",
						Description: "Comma-separated list of namespaces to get workloads from (e.g. 'bookinfo' or 'bookinfo,default'). If not provided, will list workloads from all accessible namespaces",
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Workloads: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: workloadsListHandler,
	})

	// Workload details tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "workload_details",
			Description: "Get detailed information for a specific workload in a namespace, including validation, health status, and configuration",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the workload",
					},
					"workload": {
						Type:        "string",
						Description: "Name of the workload to get details for",
					},
				},
				Required: []string{"namespace", "workload"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Workload: Details",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: workloadDetailsHandler,
	})

	// Workload metrics tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "workload_metrics",
			Description: "Get metrics for a specific workload in a namespace. Supports filtering by time range, direction (inbound/outbound), reporter, and other query parameters",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the workload",
					},
					"workload": {
						Type:        "string",
						Description: "Name of the workload to get metrics for",
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
				Required: []string{"namespace", "workload"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Workload: Metrics",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: workloadMetricsHandler,
	})

	return ret
}

func workloadsListHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	namespaces, _ := params.GetArguments()["namespaces"].(string)

	k := params.NewKiali()
	content, err := k.WorkloadsList(params.Context, namespaces)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list workloads: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func workloadDetailsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	namespace, _ := params.GetArguments()["namespace"].(string)
	workload, _ := params.GetArguments()["workload"].(string)

	if namespace == "" {
		return api.NewToolCallResult("", fmt.Errorf("namespace parameter is required")), nil
	}
	if workload == "" {
		return api.NewToolCallResult("", fmt.Errorf("workload parameter is required")), nil
	}

	k := params.NewKiali()
	content, err := k.WorkloadDetails(params.Context, namespace, workload)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get workload details: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func workloadMetricsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract required parameters
	namespace, _ := params.GetArguments()["namespace"].(string)
	workload, _ := params.GetArguments()["workload"].(string)

	if namespace == "" {
		return api.NewToolCallResult("", fmt.Errorf("namespace parameter is required")), nil
	}
	if workload == "" {
		return api.NewToolCallResult("", fmt.Errorf("workload parameter is required")), nil
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
	content, err := k.WorkloadMetrics(params.Context, namespace, workload, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get workload metrics: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
