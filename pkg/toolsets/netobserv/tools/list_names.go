package tools

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
)

func InitListNames() []api.ServerTool {
	name := defaults.ToolsetName() + "_list_names"
	return []api.ServerTool{{
		Tool: api.Tool{
			Name: name,
			Description: "Lists workload or resource names seen in NetObserv flows for a namespace and kind. " +
				"Use results to build SrcK8S_Name, DstK8S_Name, SrcK8S_OwnerName, or DstK8S_OwnerName filters.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Kubernetes namespace to search in.",
					},
					"kind": {
						Type: "string",
						Description: "Resource kind. Pod, Service, and Node use K8S_Name fields; " +
							"other kinds (Deployment, StatefulSet, …) use K8S_OwnerName / K8S_OwnerType.",
						Enum: []any{
							"Pod", "Service", "Node", "Gateway",
							"Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob",
						},
					},
				},
				Required: []string{"namespace", "kind"},
			},
			Annotations: readOnlyAnnotations("List NetObserv Resource Names"),
		},
		Handler: listNamesHandler,
	}}
}

func listNamesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	client := netobservclient.NewNetObserv(params, params.KubernetesClient)
	content, err := client.ExecuteGet(params.Context, NetObservNamesEndpoint, params.GetArguments())
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list names: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
