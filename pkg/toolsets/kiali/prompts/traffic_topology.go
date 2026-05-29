package prompts

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

func InitTrafficTopology() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "traffic-topology",
				Title:       "Traffic Topology Analysis",
				Description: "Analyze the service mesh traffic topology showing service dependencies, traffic flow, and communication patterns",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespaces",
						Description: "Comma-separated list of namespaces to include in the graph (default: all mesh namespaces)",
						Required:    false,
					},
				},
			},
			Handler: trafficTopologyHandler,
		},
	}
}

func trafficTopologyHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespaces := args["namespaces"]

	klog.Info("Starting traffic topology analysis prompt...")

	reqArgs := map[string]any{}
	if namespaces != "" {
		reqArgs["namespaces"] = namespaces
	}

	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	graphContent, err := kiali.ExecuteRequest(params.Context, tools.KialiGetMeshTrafficGraphEndpoint, reqArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve traffic graph: %w", err)
	}

	promptText := buildTrafficTopologyPrompt(graphContent, namespaces)

	return api.NewPromptCallResult(
		"Traffic topology data gathered successfully",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: promptText,
				},
			},
			{
				Role: "assistant",
				Content: api.PromptContent{
					Type: "text",
					Text: "I'll analyze the traffic topology and identify service dependencies and communication patterns.",
				},
			},
		},
		nil,
	), nil
}

func buildTrafficTopologyPrompt(graphData string, namespaces string) string {
	scope := "all mesh namespaces"
	if namespaces != "" {
		scope = fmt.Sprintf("namespaces: %s", namespaces)
	}

	return fmt.Sprintf(`# Traffic Topology Analysis

## Scope
Analyze traffic topology for %s.

## Collected Data

### Traffic Graph
%s

## Instructions

Based on the traffic graph data above, provide an analysis covering:

1. **Service Dependencies**: Map out which services communicate with each other and the direction of traffic flow.
2. **Traffic Patterns**: Identify high-traffic paths, bottlenecks, or unusual communication patterns.
3. **Health Overview**: Highlight any services or edges showing errors or degraded health.
4. **Observations**: Note any unexpected dependencies, circular calls, or services that appear isolated.
`, scope, graphData)
}
