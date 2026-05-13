package traces

import (
	"github.com/rhobs/obs-mcp/pkg/tools"
	"github.com/rhobs/obs-mcp/pkg/traces/discovery"
)

// ListInstancesOutput defines the output schema for the tempo_list_instances tool.
type ListInstancesOutput struct {
	Instances []discovery.TempoInstance `json:"instances" jsonschema:"List of available Tempo instances"`
}

var ListInstancesTool = tools.ToolDef[ListInstancesOutput]{
	Name: "tempo_list_instances",
	Description: `List all Tempo instances available in the Kubernetes cluster.
Call this tool first to discover available Tempo instances before using other Tempo tools,
as the returned namespace, name, and tenant values are required parameters for all other Tempo tools.
Always print the output of this tool in a table.`,
	Title:       "List Tempo instances",
	ReadOnly:    true,
	Destructive: false,
	Idempotent:  true,
	OpenWorld:   true,
}

func (t *Toolset) ListInstancesHandler(params ToolParams) (ListInstancesOutput, error) {
	instances, err := discovery.ListInstances(params.context, params.dynamicClient, params.config.UseRoute)
	if err != nil {
		return ListInstancesOutput{}, err
	}

	return ListInstancesOutput{Instances: instances}, nil
}
