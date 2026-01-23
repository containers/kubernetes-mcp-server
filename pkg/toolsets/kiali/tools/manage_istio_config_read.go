package tools

import (
	"fmt"

	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitManageIstioConfigRead() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_manage_istio_config_read"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Lists or gets Istio configuration objects (Gateways, VirtualServices, etc.)",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"action": {
						Type:        "string",
						Description: "Action to perform: list or get",
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the Istio object",
					},
					"group": {
						Type:        "string",
						Description: "API group of the Istio object (e.g., 'networking.istio.io', 'gateway.networking.k8s.io')",
					},
					"version": {
						Type:        "string",
						Description: "API version of the Istio object (e.g., 'v1', 'v1beta1')",
					},
					"kind": {
						Type:        "string",
						Description: "Kind of the Istio object (e.g., 'DestinationRule', 'VirtualService', 'HTTPRoute', 'Gateway')",
					},
					"name": {
						Type:        "string",
						Description: "Name of the Istio object",
					},
				},
				Required: []string{"action"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Manage Istio Config: List or Get",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: istioConfigHandlerRead,
	})
	return ret
}

func istioConfigHandlerRead(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	action, _ := params.GetArguments()["action"].(string)
	namespace, _ := params.GetArguments()["namespace"].(string)
	group, _ := params.GetArguments()["group"].(string)
	version, _ := params.GetArguments()["version"].(string)
	kind, _ := params.GetArguments()["kind"].(string)
	name, _ := params.GetArguments()["name"].(string)
	if err := validateIstioConfigInputRead(action, namespace, group, version, kind, name); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	content, err := kiali.IstioConfig(params.Context, action, namespace, group, version, kind, name, "")
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve Istio configuration: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

// validateIstioConfigInputRead centralizes validation rules for the read-only tool.
// Rules:
// - If action is "get": namespace, group, version, kind are required
// - If action is "get": name is required
func validateIstioConfigInputRead(action, namespace, group, version, kind, name string) error {
	switch action {
	case "list", "get":
		if action == "get" {
			if name == "" {
				return fmt.Errorf("name is required for action %q", action)
			}
			if namespace == "" {
				return fmt.Errorf("namespace is required for action %q", action)
			}
			if group == "" {
				return fmt.Errorf("group is required for action %q", action)
			}
			if version == "" {
				return fmt.Errorf("version is required for action %q", action)
			}
			if kind == "" {
				return fmt.Errorf("kind is required for action %q", action)
			}
		}
	default:
		return fmt.Errorf("invalid action %q: must be one of list, get", action)
	}
	return nil
}
