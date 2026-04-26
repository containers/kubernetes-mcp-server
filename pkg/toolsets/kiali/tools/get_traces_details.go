package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitGetTraceDetails() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_trace_details"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Fetches a single distributed trace by trace_id and returns its call hierarchy (service tree with duration, status, and nested calls). Use this after list_traces to drill into a specific trace.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"traceId": {
						Type:        "string",
						Description: "Trace ID to fetch and summarize. If provided, namespace/service_name are ignored.",
					},
				},
				Required: []string{"traceId"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Get Trace Details",
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
	content, err := kiali.ExecuteRequest(params.Context, KialiGetTraceDetailsEndpoint, arguments)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve traces: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
