package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitGetPodPerformance() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_pod_performance"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Returns a human-readable text summary with current Pod CPU/memory usage (from Prometheus) compared to Kubernetes requests/limits (from the Pod spec). Useful to answer questions like 'Is this workload using too much memory?'",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Kubernetes namespace of the Pod.",
					},
					"podName": {
						Type:        "string",
						Description: "Kubernetes Pod name. If workloadName is provided, the tool will attempt to resolve a Pod from that workload first.",
					},
					"workloadName": {
						Type:        "string",
						Description: "Kubernetes Workload name (e.g. Deployment/StatefulSet/etc). Tool will look up the workload and pick one of its Pods. If not found, it will fall back to treating this value as a podName.",
					},
					"timeRange": {
						Type:        "string",
						Description: "Time window used to compute CPU rate (Prometheus duration like '5m', '10m', '1h', '1d'). Defaults to '10m'.",
						Default:     api.ToRawMessage(DefaultRateInterval),
					},
					"queryTime": {
						Type:        "string",
						Description: "Optional end timestamp (RFC3339) for the query. Defaults to now.",
					},
					"clusterName": {
						Type:        "string",
						Description: "Optional. Name of the cluster to get resources from. If not provided, will use the default cluster name in the Kiali KubeConfig",
					},
				},
				Required: []string{"namespace"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Get Pod Performance",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: getPodPerformanceHandler,
	})

	return ret
}

func getPodPerformanceHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	arguments := params.GetArguments()
	content, err := kiali.ExecuteRequest(params.Context, KialiGetPodPerformanceEndpoint, arguments)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get pod performance: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
