package tools

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
)

func InitExportFlows() []api.ServerTool {
	props := flowQueryProperties()
	props["format"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Export format. Only csv is supported.",
		Default:     api.ToRawMessage(DefaultExportFormat),
		Enum:        []any{"csv"},
	}
	props["columns"] = &jsonschema.Schema{
		Type: "string",
		Description: "Optional comma-separated column names to include (e.g. SrcK8S_Namespace,DstK8S_Namespace,Bytes). " +
			"Omit to export all columns present in the result.",
	}

	name := defaults.ToolsetName() + "_export_flows"
	return []api.ServerTool{{
		Tool: api.Tool{
			Name:        name,
			Description: "Exports NetObserv flow records as CSV with the same filters as list_flows. Use when the user needs downloadable flow data for audits or offline analysis.",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: props,
			},
			Annotations: readOnlyAnnotations("Export NetObserv Flows as CSV"),
		},
		Handler: exportFlowsHandler,
	}}
}

func exportFlowsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	args := params.GetArguments()
	if args == nil {
		args = map[string]any{}
	}
	if _, ok := args["format"]; !ok {
		args["format"] = DefaultExportFormat
	}
	client := netobservclient.NewNetObserv(params, params.KubernetesClient)
	response, err := client.ExecuteGetAccept(params.Context, NetObservExportFlowsEndpoint, args, "text/csv,*/*", DefaultExportMaxBodyBytes)
	content := response.Body
	if response.Truncated {
		content += "\n\n[truncated: export exceeded maximum response size]"
	}
	return textAPIResult(content, wrapAPIError("export flows", err))
}
