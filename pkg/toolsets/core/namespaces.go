package core

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
)

func initNamespaces(o api.Openshift) []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "namespaces_list",
			Description: "List all the Kubernetes namespaces in the current cluster",
			InputSchema: &jsonschema.Schema{
				Type: "object",
			},
			Annotations: api.ToolAnnotations{
				Title:           "Namespaces: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: namespacesList,
	})
	if o.IsOpenShift(context.Background()) {
		ret = append(ret, api.ServerTool{
			Tool: api.Tool{
				Name:        "projects_list",
				Description: "List all the OpenShift projects in the current cluster",
				InputSchema: &jsonschema.Schema{
					Type: "object",
				},
				Annotations: api.ToolAnnotations{
					Title:           "Projects: List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			}, Handler: projectsList,
		})
	}
	return ret
}

func namespacesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	ret, err := kubernetes.NewCore(params).NamespacesList(params, api.ListOptions{AsTable: params.ListOutput.AsTable()})
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "namespace listing")
		return api.NewToolCallResult("", fmt.Errorf("failed to list namespaces: %w", err)), nil
	}
	return api.NewToolCallResult(params.ListOutput.PrintObj(ret)), nil
}

func projectsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	ret, err := kubernetes.NewCore(params).ProjectsList(params, api.ListOptions{AsTable: params.ListOutput.AsTable()})
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "project listing")
		return api.NewToolCallResult("", fmt.Errorf("failed to list projects: %w", err)), nil
	}
	return api.NewToolCallResult(params.ListOutput.PrintObj(ret)), nil
}
