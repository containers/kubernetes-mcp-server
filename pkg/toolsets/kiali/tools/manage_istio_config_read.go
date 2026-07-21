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
			Name: name,
			Description: "Read Istio, Gateway API, and Inference API config. 'list' groups by namespace→'group/version/kind'→{valid:[...],invalid:[...]} " +
				"where valid/invalid arrays contain resource names; omit group/kind to retrieve ALL config types in a single call. " +
				"Supports Istio (networking.istio.io, security.istio.io), Gateway API (gateway.networking.k8s.io), and Inference API (inference.networking.k8s.io) when installed. " +
				"'get' returns full YAML. For writes use manage_istio_config.",
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
						Description: "API group of the Istio object. Required ONLY for 'get' action. For 'list', OMIT group and kind to retrieve ALL config types in a single call. Use 'gateway.networking.k8s.io' for Gateway API resources. Use 'inference.networking.k8s.io' for Inference API resources.",
						Enum:        []any{"networking.istio.io", "security.istio.io", "gateway.networking.k8s.io", "inference.networking.k8s.io"},
					},
					"version": {
						Type:        "string",
						Description: "API version. Use 'v1' for all resource types. Required for 'get' action.",
					},
					"kind": {
						Type:        "string",
						Description: "Kind of the Istio object. Required ONLY for 'get' action. For 'list', OMIT to return all kinds at once \u2014 do NOT call separately for each kind.",
						Enum:        []any{"VirtualService", "DestinationRule", "Gateway", "ServiceEntry", "Sidecar", "WorkloadEntry", "WorkloadGroup", "EnvoyFilter", "AuthorizationPolicy", "PeerAuthentication", "RequestAuthentication", "HTTPRoute", "GRPCRoute", "ReferenceGrant", "TCPRoute", "TLSRoute", "InferencePool"},
					},
					"object": {
						Type:        "string",
						Description: "Name of the Istio object. Required for 'get' action.",
					},
					"meshCluster": {
						Type:        "string",
						Description: meshClusterDescription(),
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
				Title:           "Manage Istio, Gateway API, and Inference API Config: List or Get",
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
	content, err := kiali.ExecuteRequest(params.Context, KialiManageIstioConfigReadEndpoint, remapMeshCluster(arguments))
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve istio config: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
