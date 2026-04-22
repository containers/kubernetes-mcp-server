package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitListTraces() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_list_traces"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Lists distributed traces for a service in a namespace. Returns a summary (namespace, service, total_found, avg_duration_ms) and a list of traces with id, duration_ms, spans_count, root_op, slowest_service, has_errors. Use get_trace_details with a trace id to get full hierarchy.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Kubernetes namespace of the service.",
					},
					"serviceName": {
						Type:        "string",
						Description: "Service name to search traces for (required). Returns multiple traces up to limit.",
					},
					"errorOnly": {
						Type:        "boolean",
						Description: "If true, only consider traces that contain errors. Default false.",
						Default:     api.ToRawMessage(DefaultErrorOnly),
					},
					"clusterName": {
						Type:        "string",
						Description: "Optional cluster name. Defaults to the cluster name in the Kiali configuration.",
					},
					"lookbackSeconds": {
						Type:        "integer",
						Description: "How far back to search. Default 600 (10m).",
						Default:     api.ToRawMessage(DefaultLookbackSeconds),
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of traces to return. Default 10.",
						Default:     api.ToRawMessage(DefaultLimit),
					},
				},
				Required: []string{"namespace", "serviceName"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "List Traces by Service Name",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: listTracesHandler,
	})

	return ret
}

func listTracesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	arguments := params.GetArguments()
	content, err := kiali.ExecuteRequest(params.Context, KialiListTracesEndpoint, arguments)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve list of traces: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
