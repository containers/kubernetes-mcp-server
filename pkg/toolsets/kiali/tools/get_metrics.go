package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitGetMetrics() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_metrics"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Returns metrics for the given resource type, namespaces and resource name.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"resourceType": {
						Type:        "string",
						Description: "Type of resource to get metrics",
						Enum:        []any{"service", "workload", "app"},
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace to get metrics from",
					},
					"clusterName": {
						Type:        "string",
						Description: "Cluster name to get metrics from. Optional, defaults to the cluster name in the Kiali configuration (KubeConfig)",
					},
					"resourceName": {
						Type:        "string",
						Description: "Name of the resource to get metrics for",
					},
					"step": {
						Type:        "string",
						Description: "Step between data points in seconds (e.g., '15'). Optional, defaults to 15 seconds",
						Default:     api.ToRawMessage(DefaultStep),
					},
					"rateInterval": {
						Type:        "string",
						Description: "Rate interval for metrics (e.g., '1m', '5m'). Optional, defaults to '10m'",
						Default:     api.ToRawMessage(DefaultRateInterval),
					},
					"direction": {
						Type:        "string",
						Description: "Traffic direction. Optional, defaults to 'outbound'",
						Default:     api.ToRawMessage(DefaultDirection),
						Enum:        []any{"inbound", "outbound"},
					},
					"reporter": {
						Type:        "string",
						Description: "Metrics reporter. Optional, defaults to 'source'",
						Default:     api.ToRawMessage(DefaultReporter),
						Enum:        []any{"source", "destination", "both"},
					},
					"requestProtocol": {
						Type:        "string",
						Description: "Filter by request protocol (e.g., 'http', 'grpc', 'tcp'). Optional",
					},
					"quantiles": {
						Type:        "string",
						Description: "Comma-separated list of quantiles for histogram metrics (e.g., '0.5,0.95,0.99'). Optional",
						Default:     api.ToRawMessage(DefaultQuantiles),
					},
					"byLabels": {
						Type:        "string",
						Description: "Comma-separated list of labels to group metrics by (e.g., 'source_workload,destination_service'). Optional",
					},
				},
				Required: []string{"resourceType", "namespace", "resourceName"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Get Metrics for a Resource",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: resourceMetricsHandler,
	})

	return ret
}

func resourceMetricsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	arguments := params.GetArguments()
	content, err := kiali.ExecuteRequest(params.Context, KialiGetMetricsEndpoint, arguments)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve metrics: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
