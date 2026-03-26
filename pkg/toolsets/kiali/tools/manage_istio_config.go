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
			Description: "Create, patch, or delete Istio config. For list and get (read-only) use manage_istio_config_read.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"action": {
						Type:        "string",
						Description: "Action to perform (write)",
						Enum:        []any{"create", "patch", "delete"},
					},
					"confirmed": {
						Type:        "boolean",
						Description: "CRITICAL: If 'true', the destructive action (create/patch/delete) is executed. If 'false' (or omitted) for create/patch, the tool returns a YAML PREVIEW. Display it to the user and ask for confirmation before calling again with confirmed=true.",
						Default:     api.ToRawMessage(false),
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the Istio object",
					},
					"group": {
						Type:        "string",
						Description: "API group of the Istio object (e.g., 'networking.istio.io', 'gateway.networking.k8s.io').",
					},
					"version": {
						Type:        "string",
						Description: "API version. Use 'v1' for VirtualService, DestinationRule, and Gateway.",
					},
					"kind": {
						Type:        "string",
						Description: "Kind of the Istio object (e.g., 'VirtualService', 'DestinationRule').",
					},
					"object": {
						Type:        "string",
						Description: "Name of the Istio object",
					},
					"data": {
						Type:        "string",
						Description: "Complete JSON or YAML data to apply or create the object. Required for create and patch actions. You MUST provide a COMPLETE and VALID manifest with ALL required fields for the resource type. Arrays (like servers, http, etc.) are REPLACED entirely, so you must include ALL required fields within each array element.",
					},
					"clusterName": {
						Type:        "string",
						Description: "Optional cluster name. Defaults to the cluster name in the Kiali configuration.",
					},
				},
				Required: []string{"action", "namespace", "group", "version", "kind", "object"},
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
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	arguments := params.GetArguments()
	content, err := kiali.ExecuteRequest(params.Context, KialiManageIstioConfigEndpoint, arguments)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to manage istio config: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
