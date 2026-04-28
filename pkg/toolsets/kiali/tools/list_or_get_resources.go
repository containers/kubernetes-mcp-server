package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitListOrGetResources() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_resource_details"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Fetches a list of resources OR retrieves detailed data for a specific resource. If 'resourceName' is omitted, it returns a list. If 'resourceName' is provided, it returns details for that specific resource.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"resourceType": {
						Type:        "string",
						Description: "The type of resource to query.",
						Enum:        []any{"app", "namespace", "service", "workload"},
					},
					"namespaces": {
						Type:        "string",
						Description: "Comma-separated list of namespaces to query (e.g., 'bookinfo' or 'bookinfo,default'). If not provided, it will query across all accessible namespaces.",
					},
					"resourceName": {
						Type:        "string",
						Description: "Optional. The specific name of the resource. If left empty, the tool returns a list of all resources of the specified type. If provided, the tool returns deep details for this specific resource.",
					},
					"clusterName": {
						Type:        "string",
						Description: "Optional. Name of the cluster to get resources from. If not provided, will use the default cluster name in the Kiali KubeConfig",
					},
				},
				Required: []string{"resourceType"},
				DependentRequired: map[string][]string{
					"resourceName": {"namespaces"},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "List or Resource Details",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: listOrGetResourcesHandler,
	})

	return ret
}

func listOrGetResourcesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	arguments := params.GetArguments()
	content, err := kiali.ExecuteRequest(params.Context, KialiListOrGetResourcesEndpoint, arguments)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list or get resources: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
