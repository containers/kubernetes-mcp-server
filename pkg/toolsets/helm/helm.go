package helm

import (
	"fmt"

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
		{Tool: api.Tool{
			Name:        "helm_upgrade",
			Description: "Upgrade an existing Helm release with a new chart version or values",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the Helm release to upgrade",
					},
					"chart": {
						Type:        "string",
						Description: "Chart reference to upgrade to (for example: stable/grafana, oci://ghcr.io/nginxinc/charts/nginx-ingress)",
					},
					"values": {
						Type:        "object",
						Description: "Values to pass to the Helm chart (Optional)",
						Properties:  make(map[string]*jsonschema.Schema),
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace where the Helm release is installed (Optional, current namespace if not provided)",
					},
				},
				Required: []string{"name", "chart"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Helm: Upgrade",
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: helmUpgrade},
		{Tool: api.Tool{
			Name:        "helm_get_values",
			Description: "Get the values of a Helm release to see the current configuration",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the Helm release",
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace where the Helm release is installed (Optional, current namespace if not provided)",
					},
					"all_values": {
						Type:        "boolean",
						Description: "If true, returns all values including defaults from the chart (Optional, defaults to false which returns only user-supplied values)",
					},
				},
				Required: []string{"name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Helm: Get Values",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: helmGetValues},
		{Tool: api.Tool{
			Name:        "helm_status",
			Description: "Get the status of a Helm release including deployment state, notes, and resource information",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the Helm release",
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace where the Helm release is installed (Optional, current namespace if not provided)",
					},
				},
				Required: []string{"name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Helm: Status",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: helmStatus},
		{Tool: api.Tool{
			Name:        "helm_history",
			Description: "Get the revision history of a Helm release to see past deployments and their status",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the Helm release",
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace where the Helm release is installed (Optional, current namespace if not provided)",
					},
					"max": {
						Type:        "integer",
						Description: "Maximum number of revisions to return (Optional, returns all if not specified)",
					},
				},
				Required: []string{"name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Helm: History",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: helmHistory},
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
	ret, err := params.NewHelm().Install(params, chart, values, name, namespace)
	if err != nil {
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
	ret, err := params.NewHelm().List(namespace, allNamespaces)
	if err != nil {
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
	ret, err := params.NewHelm().Uninstall(name, namespace)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to uninstall helm chart '%s': %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}

func helmUpgrade(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var name string
	ok := false
	if name, ok = params.GetArguments()["name"].(string); !ok {
		return api.NewToolCallResult("", fmt.Errorf("failed to upgrade helm release, missing argument name")), nil
	}
	var chart string
	if chart, ok = params.GetArguments()["chart"].(string); !ok {
		return api.NewToolCallResult("", fmt.Errorf("failed to upgrade helm release, missing argument chart")), nil
	}
	values := map[string]interface{}{}
	if v, ok := params.GetArguments()["values"].(map[string]interface{}); ok {
		values = v
	}
	namespace := ""
	if v, ok := params.GetArguments()["namespace"].(string); ok {
		namespace = v
	}
	ret, err := params.NewHelm().Upgrade(params, name, chart, values, namespace)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to upgrade helm release '%s': %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}

func helmGetValues(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var name string
	ok := false
	if name, ok = params.GetArguments()["name"].(string); !ok {
		return api.NewToolCallResult("", fmt.Errorf("failed to get helm values, missing argument name")), nil
	}
	namespace := ""
	if v, ok := params.GetArguments()["namespace"].(string); ok {
		namespace = v
	}
	allValues := false
	if v, ok := params.GetArguments()["all_values"].(bool); ok {
		allValues = v
	}
	ret, err := params.NewHelm().GetValues(name, namespace, allValues)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get values for helm release '%s': %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}

func helmStatus(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var name string
	ok := false
	if name, ok = params.GetArguments()["name"].(string); !ok {
		return api.NewToolCallResult("", fmt.Errorf("failed to get helm status, missing argument name")), nil
	}
	namespace := ""
	if v, ok := params.GetArguments()["namespace"].(string); ok {
		namespace = v
	}
	ret, err := params.NewHelm().Status(name, namespace)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get status for helm release '%s': %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}

func helmHistory(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var name string
	ok := false
	if name, ok = params.GetArguments()["name"].(string); !ok {
		return api.NewToolCallResult("", fmt.Errorf("failed to get helm history, missing argument name")), nil
	}
	namespace := ""
	if v, ok := params.GetArguments()["namespace"].(string); ok {
		namespace = v
	}
	max := 0
	if v, ok := params.GetArguments()["max"].(float64); ok {
		max = int(v)
	}
	ret, err := params.NewHelm().History(name, namespace, max)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get history for helm release '%s': %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}
