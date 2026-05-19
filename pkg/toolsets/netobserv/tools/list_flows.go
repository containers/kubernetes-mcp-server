package tools

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
)

func InitListFlows() []api.ServerTool {
	name := defaults.ToolsetName() + "_list_flows"
	return []api.ServerTool{{
		Tool: api.Tool{
			Name:        name,
			Description: "Lists network flow records from NetObserv (Loki). Returns aggregated flow log entries with optional filters on namespaces, workloads, IPs, ports, and protocols.",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: flowQueryProperties(),
			},
			Annotations: readOnlyAnnotations("List NetObserv Flow Records"),
		},
		Handler: listFlowsHandler,
	}}
}

func listFlowsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	client := netobservclient.NewNetObserv(params, params.KubernetesClient)
	content, err := client.ExecuteGet(params.Context, NetObservFlowsEndpoint, params.GetArguments())
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list flow records: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
