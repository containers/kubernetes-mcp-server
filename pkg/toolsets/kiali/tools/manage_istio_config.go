package tools

import (
	"fmt"

	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitManageIstioConfig() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_manage_istio_config"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Creates, patches, or deletes Istio configuration objects (Gateways, VirtualServices, etc.)",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"action": {
						Type:        "string",
						Description: "Action to perform: create, patch, or delete",
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
					"json_data": {
						Type:        "string",
						Description: "JSON data to apply or create the object",
					},
				},
				Required: []string{"action"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Manage Istio Config: Create, Patch, Delete",
				ReadOnlyHint:    ptr.To(false),
				DestructiveHint: ptr.To(true),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: istioConfigHandler,
	})
	return ret
}

func istioConfigHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	action, _ := params.GetArguments()["action"].(string)
	namespace, _ := params.GetArguments()["namespace"].(string)
	group, _ := params.GetArguments()["group"].(string)
	version, _ := params.GetArguments()["version"].(string)
	kind, _ := params.GetArguments()["kind"].(string)
	name, _ := params.GetArguments()["name"].(string)
	jsonData, _ := params.GetArguments()["json_data"].(string)
	if err := validateIstioConfigInput(action, namespace, group, version, kind, name, jsonData); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	content, err := kiali.IstioConfig(params.Context, action, namespace, group, version, kind, name, jsonData)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve Istio configuration: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

// validateIstioConfigInput centralizes validation rules for the write tool.
// Rules:
// - namespace, group, version, kind are required
// - If action is "create": json_data is required
// - If action is "patch": name and json_data are required
// - If action is "delete": name is required
func validateIstioConfigInput(action, namespace, group, version, kind, name, jsonData string) error {
	switch action {
	case "create", "patch", "delete":
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
		if action == "create" {
			if jsonData == "" {
				return fmt.Errorf("json_data is required for action %q", action)
			}
		}
		if action == "patch" {
			if name == "" {
				return fmt.Errorf("name is required for action %q", action)
			}
			if jsonData == "" {
				return fmt.Errorf("json_data is required for action %q", action)
			}
		}
		if action == "delete" {
			if name == "" {
				return fmt.Errorf("name is required for action %q", action)
			}
		}
	default:
		return fmt.Errorf("invalid action %q: must be one of create, patch, delete", action)
	}
	return nil
}
