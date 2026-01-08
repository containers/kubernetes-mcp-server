package helm

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/helm"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initHelm() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "helm_install",
			Description: "Install a Helm chart in the current or provided namespace",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"chart": {
						Type:        "string",
						Description: "Chart reference to install (for example: stable/grafana, oci://ghcr.io/nginxinc/charts/nginx-ingress)",
					},
					"values": {
						Type:        "object",
						Description: "Values to pass to the Helm chart (Optional)",
						Properties:  make(map[string]*jsonschema.Schema),
					},
					"name": {
						Type:        "string",
						Description: "Name of the Helm release (Optional, random name if not provided)",
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace to install the Helm chart in (Optional, current namespace if not provided)",
					},
				},
				Required: []string{"chart"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Helm: Install",
				DestructiveHint: ptr.To(false),
				IdempotentHint:  nil, // TODO: consider replacing implementation with equivalent to: helm upgrade --install
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: helmInstall},
		{Tool: api.Tool{
			Name:        "helm_list",
			Description: "List all the Helm releases in the current or provided namespace (or in all namespaces if specified)",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace to list Helm releases from (Optional, all namespaces if not provided)",
					},
					"all_namespaces": {
						Type:        "boolean",
						Description: "If true, lists all Helm releases in all namespaces ignoring the namespace argument (Optional)",
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Helm: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: helmList},
		{Tool: api.Tool{
			Name:        "helm_uninstall",
			Description: "Uninstall a Helm release in the current or provided namespace",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the Helm release to uninstall",
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace to uninstall the Helm release from (Optional, current namespace if not provided)",
					},
				},
				Required: []string{"name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Helm: Uninstall",
				DestructiveHint: ptr.To(true),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: helmUninstall},
	}
}

func helmInstall(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var chart string
	ok := false
	if chart, ok = params.GetArguments()["chart"].(string); !ok {
		return api.NewToolCallResult("", fmt.Errorf("failed to install helm chart, missing argument chart")), nil
	}
	values := map[string]interface{}{}
	if v, ok := params.GetArguments()["values"].(map[string]interface{}); ok {
		values = v
	}
	name := ""
	if v, ok := params.GetArguments()["name"].(string); ok {
		name = v
	}
	namespace := ""
	if v, ok := params.GetArguments()["namespace"].(string); ok {
		namespace = v
	}
	ret, err := helm.NewHelm(params).Install(params, chart, values, name, namespace)
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "helm install")
		return api.NewToolCallResult("", fmt.Errorf("failed to install helm chart '%s': %w", chart, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}

func helmList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	allNamespaces := false
	if v, ok := params.GetArguments()["all_namespaces"].(bool); ok {
		allNamespaces = v
	}
	namespace := ""
	if v, ok := params.GetArguments()["namespace"].(string); ok {
		namespace = v
	}
	ret, err := helm.NewHelm(params).List(namespace, allNamespaces)
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "helm list")
		return api.NewToolCallResult("", fmt.Errorf("failed to list helm releases in namespace '%s': %w", namespace, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}

func helmUninstall(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var name string
	ok := false
	if name, ok = params.GetArguments()["name"].(string); !ok {
		return api.NewToolCallResult("", fmt.Errorf("failed to uninstall helm chart, missing argument name")), nil
	}
	namespace := ""
	if v, ok := params.GetArguments()["namespace"].(string); ok {
		namespace = v
	}
	ret, err := helm.NewHelm(params).Uninstall(name, namespace)
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "helm uninstall")
		return api.NewToolCallResult("", fmt.Errorf("failed to uninstall helm chart '%s': %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}
