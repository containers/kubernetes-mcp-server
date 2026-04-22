package tools

import (
	"fmt"

	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitGetLogs() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_logs"
	// Workload logs tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Get the logs of a Kubernetes Pod (or workload name that will be resolved to a pod) in a namespace. Output is plain text, matching kubernetes-mcp-server pods_log.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace to get the Pod logs from",
					},
					"name": {
						Type:        "string",
						Description: "Name of the Pod to get the logs from. If it does not exist, it will be treated as a workload name and a running pod will be selected.",
					},
					"workload": {
						Type:        "string",
						Description: "Optional. Workload name override (used when name lookup fails).",
					},
					"container": {
						Type:        "string",
						Description: "Optional. Name of the Pod container to get the logs from.",
					},
					"tail": {
						Type:        "integer",
						Description: "Number of lines to retrieve from the end of the logs (Optional, defaults to 50). Cannot exceed 200 lines.",
						Minimum:     ptr.To(float64(1)),
						Default:     api.ToRawMessage(DefaultTail),
					},
					"severity": {
						Type:        "string",
						Description: "Optional severity filter applied client-side. Accepts 'ERROR', 'WARN' or combinations like 'ERROR,WARN'.",
					},
					"previous": {
						Type:        "boolean",
						Description: "Optional. Return previous terminated container logs",
					},
					"format": {
						Type:        "string",
						Description: "Output formatting for chat. 'codeblock' wraps logs in ~~~ fences (recommended). 'plain' returns raw text like kubernetes-mcp-server pods_log.",
						Enum:        []any{"codeblock", "plain"},
					},
					"clusterName": {
						Type:        "string",
						Description: "Optional. Name of the cluster to get the logs from. If not provided, will use the default cluster name in the Kiali KubeConfig",
					},
				},
				Required: []string{"namespace", "name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Workload: Logs",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: workloadLogsHandler,
	})

	return ret
}

func workloadLogsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	arguments := params.GetArguments()
	content, err := kiali.ExecuteRequest(params.Context, KialiGetLogsEndpoint, arguments)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get logs: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
