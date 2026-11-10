package helm

import (
	"fmt"
	"math"

	"github.com/containers/kubernetes-mcp-server/pkg/helm"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initHelm() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "helm_install",
			Description: "Install (deploy) a Helm chart to create a release in the current or provided namespace",
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
		}, RBAC: api.RBACUnbounded("Permissions depend on resources contained in the referenced Helm chart"), Handler: helmInstall},
		{Tool: api.Tool{
			Name:        "helm_list",
			Description: "List all the Helm releases in the current or provided namespace (or in all namespaces if specified). Use limit and offset to page the releases returned to you. Helm loads release secrets before applying that page, so a secret list that exceeds max_backend_response_bytes can still fail.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace to list Helm releases from (Optional, current namespace if not provided)",
					},
					"all_namespaces": {
						Type:        "boolean",
						Description: "If true, lists all Helm releases in all namespaces ignoring the namespace argument (Optional)",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of releases to return. Omit or set to 0 to return every release Helm loaded. A full page tells you the next offset.",
						Minimum:     ptr.To(float64(0)),
					},
					"offset": {
						Type:        "integer",
						Description: "Number of releases to skip. Send the next offset from a full page, with the same limit.",
						Minimum:     ptr.To(float64(0)),
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Helm: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, RBAC: api.RBACBounded(
			api.RBACRequirement{
				Verbs:     []string{"list"},
				Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "secrets"}},
				Namespace: &api.RBACNamespace{AllNamespaces: true},
			},
			api.RBACRequirement{
				Verbs:     []string{"list"},
				Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "configmaps"}},
				Namespace: &api.RBACNamespace{AllNamespaces: true},
			},
		), Handler: helmList},
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
		}, RBAC: api.RBACUnbounded("Permissions depend on resources contained in the installed Helm release"), Handler: helmUninstall},
	}
}

func helmPage(p *api.Params) (int, int, error) {
	limit := p.OptionalInt64("limit", 0)
	offset := p.OptionalInt64("offset", 0)
	if err := p.Err(); err != nil {
		return 0, 0, err
	}
	if limit < 0 {
		return 0, 0, fmt.Errorf("limit must be greater than or equal to 0")
	}
	if offset < 0 {
		return 0, 0, fmt.Errorf("offset must be greater than or equal to 0")
	}
	if limit > int64(math.MaxInt) || offset > int64(math.MaxInt) {
		return 0, 0, fmt.Errorf("limit and offset must fit in an integer")
	}
	return int(limit), int(offset), nil
}

func newHelmClient(params api.ToolHandlerParams) (*helm.Helm, error) {
	if params.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	var cfg *helm.Config
	if c, ok := params.Config.GetToolsetConfig("helm"); ok {
		if hc, ok := c.(*helm.Config); ok {
			cfg = hc
		}
	}
	return helm.NewHelm(params, cfg), nil
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
	client, err := newHelmClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	ret, err := client.Install(params, chart, values, name, namespace)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to install helm chart '%s': %w", chart, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}

func helmList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	allNamespaces := p.OptionalBool("all_namespaces", false)
	namespace := p.OptionalString("namespace", "")
	limit, offset, pageErr := helmPage(p)
	if pageErr != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list helm releases: %w", pageErr)), nil
	}
	client, err := newHelmClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	ret, err := client.List(params.Context, namespace, allNamespaces, limit, offset)
	if err != nil {
		if kubernetes.IsResponseTooLarge(err) {
			err = fmt.Errorf("%w; retry with a smaller limit", err)
		}
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
	client, err := newHelmClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	ret, err := client.Uninstall(params.Context, name, namespace)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to uninstall helm chart '%s': %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, err), nil
}
