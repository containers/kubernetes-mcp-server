package full

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func initNamespaces(k *internalk8s.Manager) []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "namespaces_list",
			Description: "List all the Kubernetes namespaces in the cluster (current or provided context)",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"context": api.ContextParameterSchema,
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Namespaces: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: namespacesList,
	})
	if k.IsOpenShift(context.Background()) {
		ret = append(ret, api.ServerTool{
			Tool: api.Tool{
				Name:        "projects_list",
				Description: "List all the OpenShift projects in the cluster (current or provided context)",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"context": api.ContextParameterSchema,
					},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Projects: List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			}, Handler: projectsList,
		})
	}
	return ret
}

func namespacesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Get Kubernetes client for the specified context (or default)
	k8sClient, err := api.GetKubernetesWithContext(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	ret, err := k8sClient.NamespacesList(params.Context, internalk8s.ResourceListOptions{AsTable: params.ListOutput.AsTable()})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list namespaces: %v", err)), nil
	}
	return api.NewToolCallResult(params.ListOutput.PrintObj(ret)), nil
}

func projectsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Get Kubernetes client for the specified context (or default)
	k8sClient, err := api.GetKubernetesWithContext(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	ret, err := k8sClient.ProjectsList(params.Context, internalk8s.ResourceListOptions{AsTable: params.ListOutput.AsTable()})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list projects: %v", err)), nil
	}
	return api.NewToolCallResult(params.ListOutput.PrintObj(ret)), nil
}
