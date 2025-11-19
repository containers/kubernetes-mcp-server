package kiali

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initHealth() []api.ServerTool {
	ret := make([]api.ServerTool, 0)

	// Cluster health tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "health",
			Description: "Get health status for apps, workloads, and services across specified namespaces in the mesh. Returns health information including error rates and status for the requested resource type",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespaces": {
						Type:        "string",
						Description: "Comma-separated list of namespaces to get health from (e.g. 'bookinfo' or 'bookinfo,default'). If not provided, returns health for all accessible namespaces",
					},
					"type": {
						Type:        "string",
						Description: "Type of health to retrieve: 'app', 'service', or 'workload'. Default: 'app'",
					},
					"rateInterval": {
						Type:        "string",
						Description: "Rate interval for fetching error rate (e.g., '10m', '5m', '1h'). Default: '10m'",
					},
					"queryTime": {
						Type:        "string",
						Description: "Unix timestamp (in seconds) for the prometheus query. If not provided, uses current time. Optional",
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Health",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: clusterHealthHandler,
	})

	return ret
}

func initHealthSummary() []api.ServerTool {
	ret := make([]api.ServerTool, 0)

	// Mesh health summary tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "mesh_health_summary",
			Description: "Get aggregated health summary for the entire mesh or specific namespaces. Returns overall availability, error rates, and counts of healthy/unhealthy entities across apps, services, and workloads. Provides global and per-namespace breakdowns with identification of top unhealthy entities.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespaces": {
						Type:        "string",
						Description: "Comma-separated list of namespaces to include in summary (e.g. 'bookinfo,default'). If not provided, summarizes health for all accessible namespaces",
					},
					"rateInterval": {
						Type:        "string",
						Description: "Rate interval for fetching error rate (e.g., '10m', '5m', '1h'). Default: '10m'",
					},
					"queryTime": {
						Type:        "string",
						Description: "Unix timestamp (in seconds) for the prometheus query. If not provided, uses current time. Optional",
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Mesh Health Summary",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		},
		Handler: meshHealthSummaryHandler,
	})

	return ret
}

func meshHealthSummaryHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	namespaces, _ := params.GetArguments()["namespaces"].(string)

	// Extract optional query parameters
	queryParams := make(map[string]string)
	if rateInterval, ok := params.GetArguments()["rateInterval"].(string); ok && rateInterval != "" {
		queryParams["rateInterval"] = rateInterval
	}
	if queryTime, ok := params.GetArguments()["queryTime"].(string); ok && queryTime != "" {
		queryParams["queryTime"] = queryTime
	}

	k := params.NewKiali()
	content, err := k.MeshHealthSummary(params.Context, namespaces, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get mesh health summary: %v", err)), nil
	}

	return api.NewToolCallResult(content, nil), nil
}

func clusterHealthHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	namespaces, _ := params.GetArguments()["namespaces"].(string)

	// Extract optional query parameters
	queryParams := make(map[string]string)
	if healthType, ok := params.GetArguments()["type"].(string); ok && healthType != "" {
		// Validate type parameter
		if healthType != "app" && healthType != "service" && healthType != "workload" {
			return api.NewToolCallResult("", fmt.Errorf("invalid type parameter: must be one of 'app', 'service', or 'workload'")), nil
		}
		queryParams["type"] = healthType
	}
	if rateInterval, ok := params.GetArguments()["rateInterval"].(string); ok && rateInterval != "" {
		queryParams["rateInterval"] = rateInterval
	}
	if queryTime, ok := params.GetArguments()["queryTime"].(string); ok && queryTime != "" {
		queryParams["queryTime"] = queryTime
	}

	k := params.NewKiali()
	content, err := k.Health(params.Context, namespaces, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get health: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
