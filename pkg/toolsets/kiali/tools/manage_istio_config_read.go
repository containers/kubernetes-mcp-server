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
			Description: "Read-only Istio config: list or get objects. For action 'list', returns an array of objects with {name, namespace, type, validation}. For create, patch, or delete use manage_istio_config.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"action": {
						Type:        "string",
						Description: "Action to perform (read-only)",
						Enum:        []any{"list", "get"},
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the Istio object. For 'list', if not provided, returns objects across all namespaces. For 'get', required.",
					},
					"group": {
						Type:        "string",
						Description: "API group of the Istio object. Required for 'get' action.",
						Enum:        []any{"networking.istio.io", "security.istio.io"},
					},
					"version": {
						Type:        "string",
						Description: "API version. Use 'v1' for VirtualService, DestinationRule, and Gateway. Required for 'get' action.",
					},
					"kind": {
						Type:        "string",
						Description: "Kind of the Istio object. Required for 'get' action.",
						Enum:        []any{"VirtualService", "DestinationRule", "Gateway", "ServiceEntry", "Sidecar", "WorkloadEntry", "WorkloadGroup", "EnvoyFilter", "AuthorizationPolicy", "PeerAuthentication", "RequestAuthentication"},
					},
					"object": {
						Type:        "string",
						Description: "Name of the Istio object. Required for 'get' action.",
					},
					"clusterName": {
						Type:        "string",
						Description: "Optional cluster name. Defaults to the cluster name in the Kiali configuration.",
					},
					"serviceName": {
						Type:        "string",
						Description: "Filter Istio configurations (VirtualServices, DestinationRules, and their referenced Gateways) that affect a specific service. Only applicable for 'list' action",
					},
				},
				Required: []string{"action"},
				DependentRequired: map[string][]string{
					"object": {"group", "version", "kind", "namespace"},
				},
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
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	arguments := params.GetArguments()
	content, err := kiali.ExecuteRequest(params.Context, KialiManageIstioConfigReadEndpoint, arguments)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve istio config: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
