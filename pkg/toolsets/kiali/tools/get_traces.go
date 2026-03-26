package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitGetTraces() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_traces"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Fetches a distributed trace (Jaeger/Tempo) by trace_id or searches by service_name (optionally only error traces) and summarizes bottlenecks and error spans.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"traceId": {
						Type:        "string",
						Description: "Trace ID to fetch and summarize. If provided, namespace/service_name are ignored.",
					},
					"namespace": {
						Type:        "string",
						Description: "Kubernetes namespace of the service (required when trace_id is not provided).",
					},
					"serviceName": {
						Type:        "string",
						Description: "Service name to search traces for (required when trace_id is not provided).",
					},
					"errorOnly": {
						Type:        "boolean",
						Description: "If true, only consider traces that contain errors (e.g. error=true / non-200 status). Default false.",
					},
					"clusterName": {
						Type:        "string",
						Description: "Optional cluster name. Defaults to the cluster name in the Kiali configuration.",
					},
					"lookbackSeconds": {
						Type:        "integer",
						Description: "How far back to search when using service_name. Default 600 (10m).",
						Default:     api.ToRawMessage(DefaultLookbackSeconds),
					},
					"limit": {
						Type:        "integer",
						Description: "Max number of traces to consider when searching by service_name. Default 10.",
						Default:     api.ToRawMessage(DefaultLimit),
					},
					"maxSpans": {
						Type:        "integer",
						Description: "Max number of spans to return in each summary section (bottlenecks, errors, roots). Default 7.",
						Default:     api.ToRawMessage(DefaultMaxSpans),
					},
				},
				Required: []string{},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Get Traces for a Resource or Trace Details",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: tracesHandler,
	})

	return ret
}

func tracesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	arguments := params.GetArguments()
	content, err := kiali.ExecuteRequest(params.Context, KialiGetTracesEndpoint, arguments)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve traces: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
