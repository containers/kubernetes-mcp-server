package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

type resourceOperations struct {
	singularName string
	metricsFunc  func(ctx context.Context, k *kialiclient.Kiali, namespace, name string, queryParams map[string]string) (string, error)
}

var opsMap = map[string]resourceOperations{
	"service": {
		singularName: "service",
		metricsFunc: func(ctx context.Context, k *kialiclient.Kiali, ns, name string, queryParams map[string]string) (string, error) {
			return k.ServiceMetrics(ctx, ns, name, queryParams)
		},
	},
	"workload": {
		singularName: "workload",
		metricsFunc: func(ctx context.Context, k *kialiclient.Kiali, ns, name string, queryParams map[string]string) (string, error) {
			return k.WorkloadMetrics(ctx, ns, name, queryParams)
		},
	},
}

func InitGetMetrics() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_metrics"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Gets lists or detailed info for Kubernetes resources (services, workloads) within the mesh",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"resource_type": {
						Type:        "string",
						Description: "Type of resource to get details for (service, workload)",
						Enum:        []any{"service", "workload"},
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace to get resources from",
					},
					"resource_name": {
						Type:        "string",
						Description: "Name of the resource to get details for (optional string - if provided, gets details; if empty, lists all).",
					},
					"duration": {
						Type:        "string",
						Description: "Time range to get metrics for (optional string - if provided, gets metrics (e.g., '1m', '5m', '1h'); if empty, get default 30m).", Default: api.ToRawMessage(kialiclient.DefaultDuration),
					},
					"step": {
						Type:        "string",
						Description: "Step between data points in seconds (e.g., '15'). Optional, defaults to 15 seconds",
						Default:     api.ToRawMessage(kialiclient.DefaultStep),
					},
					"rateInterval": {
						Type:        "string",
						Description: "Rate interval for metrics (e.g., '1m', '5m'). Optional, defaults to '10m'",
						Default:     api.ToRawMessage(kialiclient.DefaultRateInterval),
					},
					"direction": {
						Type:        "string",
						Description: "Traffic direction: 'inbound' or 'outbound'. Optional, defaults to 'outbound'",
						Default:     api.ToRawMessage(kialiclient.DefaultDirection),
					},
					"reporter": {
						Type:        "string",
						Description: "Metrics reporter: 'source', 'destination', or 'both'. Optional, defaults to 'source'",
						Default:     api.ToRawMessage(kialiclient.DefaultReporter),
					},
					"requestProtocol": {
						Type:        "string",
						Description: "Filter by request protocol (e.g., 'http', 'grpc', 'tcp'). Optional",
					},
					"quantiles": {
						Type:        "string",
						Description: "Comma-separated list of quantiles for histogram metrics (e.g., '0.5,0.95,0.99'). Optional",
						Default:     api.ToRawMessage(kialiclient.DefaultQuantiles),
					},
					"byLabels": {
						Type:        "string",
						Description: "Comma-separated list of labels to group metrics by (e.g., 'source_workload,destination_service'). Optional",
					},
				},
				Required: []string{"resource_type", "namespace", "resource_name"},
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
	// Extract parameters
	resourceType, _ := params.GetArguments()["resource_type"].(string)
	namespace, _ := params.GetArguments()["namespace"].(string)
	resourceName, _ := params.GetArguments()["resource_name"].(string)

	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	namespace = strings.TrimSpace(namespace)
	resourceName = strings.TrimSpace(resourceName)

	if resourceType == "" {
		return api.NewToolCallResult("", fmt.Errorf("resource_type is required")), nil
	}
	if namespace == "" || len(strings.Split(namespace, ",")) != 1 {
		return api.NewToolCallResult("", fmt.Errorf("namespace is required")), nil
	}
	if resourceName == "" {
		return api.NewToolCallResult("", fmt.Errorf("resource_name is required")), nil
	}

	ops, ok := opsMap[resourceType]
	if !ok {
		return api.NewToolCallResult("", fmt.Errorf("invalid resource type: %s", resourceType)), nil
	}

	queryParams := make(map[string]string)
	if err := setQueryParam(params, queryParams, "duration", kialiclient.DefaultDuration); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	if err := setQueryParam(params, queryParams, "step", kialiclient.DefaultStep); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	if err := setQueryParam(params, queryParams, "rateInterval", kialiclient.DefaultRateInterval); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	if err := setQueryParam(params, queryParams, "direction", kialiclient.DefaultDirection); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	if err := setQueryParam(params, queryParams, "reporter", kialiclient.DefaultReporter); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	if requestProtocol, ok := params.GetArguments()["requestProtocol"].(string); ok && requestProtocol != "" {
		queryParams["requestProtocol"] = requestProtocol
	}
	if err := setQueryParam(params, queryParams, "quantiles", kialiclient.DefaultQuantiles); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	if byLabels, ok := params.GetArguments()["byLabels"].(string); ok && byLabels != "" {
		queryParams["byLabels"] = byLabels
	}

	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	content, err := ops.metricsFunc(params.Context, kiali, namespace, resourceName, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get %s metrics: %w", ops.singularName, err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
