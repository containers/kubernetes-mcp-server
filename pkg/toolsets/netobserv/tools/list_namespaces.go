package tools

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
)

func InitListNamespaces() []api.ServerTool {
	name := defaults.ToolsetName() + "_list_namespaces"
	return []api.ServerTool{{
		Tool: api.Tool{
			Name: name,
			Description: "Lists Kubernetes namespace names observed in NetObserv flow data. " +
				"Use before building filters or the namespace parameter on flow tools.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type: "string",
						Description: "Optional tenant scope (same as the namespace parameter on list_flows). " +
							"When set, restricts discovery to flows visible in that namespace context.",
					},
				},
			},
			Annotations: readOnlyAnnotations("List NetObserv Namespaces"),
		},
		Handler: listNamespacesHandler,
	}}
}

func listNamespacesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	client := netobservclient.NewNetObserv(params, params.KubernetesClient)
	content, err := client.ExecuteGet(params.Context, NetObservNamespacesEndpoint, params.GetArguments())
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list namespaces: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
