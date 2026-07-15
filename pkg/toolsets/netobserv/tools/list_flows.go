package tools

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/internal/defaults"
)

func InitListFlows() []api.ServerTool {
	name := defaults.ToolsetName() + "_list_flows"
	return []api.ServerTool{{
		Tool: api.Tool{
			Name:        name,
			Description: "Lists NetObserv network flow records from Loki. Use when investigating traffic between workloads, IPs, ports, or protocols in a namespace or time window.",
			InputSchema: toolInputSchema(flowQueryProperties(), nil),
			Annotations: readOnlyAnnotations("List NetObserv Flow Records"),
		},
		Handler: listFlowsHandler,
	}}
}

func listFlowsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	client := netobservclient.NewNetObserv(params.Context, params, params.KubernetesClient, params.FilteringProvider)
	content, err := client.ExecuteGet(params.Context, NetObservFlowsEndpoint, params.GetArguments())
	return jsonAPIResult(content, wrapAPIError("list flow records", err))
}
