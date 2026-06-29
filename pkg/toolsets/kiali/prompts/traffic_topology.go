package prompts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
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
						Description: "Comma-separated list of namespaces to include in the graph, or 'all' to include all accessible mesh namespaces",
						Required:    true,
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

	if namespaces == "" {
		return nil, fmt.Errorf("namespaces argument is required: provide a comma-separated list or 'all' for all accessible mesh namespaces")
	}

	klog.FromContext(params.Context).Info("Starting traffic topology analysis prompt...")

	kiali := kialiclient.NewKiali(params, params.RESTConfig())

	resolvedNamespaces, err := resolveNamespaces(kiali, params, namespaces)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve namespaces: %w", err)
	}

	reqArgs := map[string]any{"namespaces": resolvedNamespaces}
	graphContent, err := kiali.ExecuteRequest(params.Context, tools.KialiGetMeshTrafficGraphEndpoint, reqArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve traffic graph: %w", err)
	}

	promptText := buildTrafficTopologyPrompt(graphContent, resolvedNamespaces)

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

// allNamespacesKeyword is the special value users can pass to indicate
// "all accessible mesh namespaces" instead of listing them explicitly.
const allNamespacesKeyword = "all"

// resolveNamespaces resolves the namespaces argument. If "all" is provided,
// it discovers all accessible mesh namespaces via the Kiali API. Otherwise
// it returns the input as-is (comma-separated list).
func resolveNamespaces(kiali *kialiclient.Kiali, params api.PromptHandlerParams, namespaces string) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(namespaces), allNamespacesKeyword) {
		return namespaces, nil
	}

	content, err := kiali.ExecuteRequest(params.Context, tools.KialiListOrGetResourcesEndpoint,
		map[string]any{"resourceType": "namespace"})
	if err != nil {
		return "", fmt.Errorf("failed to discover mesh namespaces: %w", err)
	}

	discovered := parseNamespacesFromResponse(params.Context, content)
	if discovered == "" {
		return "", fmt.Errorf("no mesh namespaces found")
	}
	return discovered, nil
}

// parseNamespacesFromResponse extracts namespace names from the Kiali
// list_or_get_resources JSON response and returns them as a comma-separated string.
func parseNamespacesFromResponse(ctx context.Context, content string) string {
	type nsItem struct {
		Name string `json:"name"`
	}
	type nsResponse struct {
		Namespaces []nsItem `json:"namespaces"`
	}

	var resp nsResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		klogutil.LogWarn(klog.FromContext(ctx), "Failed to parse namespace list response", klogutil.Err(err))
		return ""
	}

	names := make([]string, 0, len(resp.Namespaces))
	for _, ns := range resp.Namespaces {
		if ns.Name != "" {
			names = append(names, ns.Name)
		}
	}
	return strings.Join(names, ",")
}

func buildTrafficTopologyPrompt(graphData string, namespaces string) string {
	return fmt.Sprintf(`# Traffic Topology Analysis

## Scope
Analyze traffic topology for namespaces: %s.

## Collected Data

### Traffic Graph
%s

## Instructions

Based on the traffic graph data above, provide an analysis covering:

1. **Service Dependencies**: Map out which services communicate with each other and the direction of traffic flow.
2. **Traffic Patterns**: Identify high-traffic paths, bottlenecks, or unusual communication patterns.
3. **Health Overview**: Highlight any services or edges showing errors or degraded health.
4. **Observations**: Note any unexpected dependencies, circular calls, or services that appear isolated.
`, namespaces, graphData)
}
