package traces

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"

	"github.com/rhobs/obs-mcp/pkg/tools"
	"github.com/rhobs/obs-mcp/pkg/traces/discovery"
)

// listInstancesOutput defines the output schema for the tempo_list_instances tool.
type listInstancesOutput struct {
	Instances []discovery.TempoInstance `json:"instances" jsonschema:"List of available Tempo instances"`
}

var listInstancesOutputSchema = tools.MustSchema[listInstancesOutput]()

func initListInstances(p api.FilteringProvider) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name: "tempo_list_instances",
			Description: `List all Tempo instances available in the Kubernetes cluster.
Call this tool first to discover available Tempo instances before using other Tempo tools,
as the returned namespace, name, and tenant values are required parameters for all other Tempo tools.
Always print the output of this tool in a table.`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
			},
			OutputSchema: listInstancesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "List Tempo instances",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: listInstancesHandler,
		TargetCompatibilityFilters: []func() bool{
			hasTempoStackCRD(p),
		},
	}
}

func listInstancesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	cfg := getToolsetConfig(params)
	instances, err := discovery.ListInstances(params.Context, params.DynamicClient(), cfg.UseRoute)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	return api.NewToolCallResultStructured(listInstancesOutput{
		Instances: instances,
	}, nil), nil
}
