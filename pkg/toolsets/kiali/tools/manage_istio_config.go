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
			Description: "Create, patch, or delete Istio, Gateway API, and Inference API config. Supports Istio resources (networking.istio.io, security.istio.io), Gateway API resources (gateway.networking.k8s.io), and Inference API resources (inference.networking.k8s.io) when installed on the cluster. For list and get (read-only) use manage_istio_config_read.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"action": {
						Type:        "string",
						Description: "Action to perform (write)",
						Enum:        []any{"create", "patch", "delete"},
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the Istio object.",
					},
					"group": {
						Type:        "string",
						Description: "API group of the Istio object. Use 'gateway.networking.k8s.io' for Gateway API resources. Use 'inference.networking.k8s.io' for Inference API resources.",
						Enum:        []any{"networking.istio.io", "security.istio.io", "gateway.networking.k8s.io", "inference.networking.k8s.io"},
					},
					"version": {
						Type:        "string",
						Description: "API version. Use 'v1' for all resource types.",
					},
					"kind": {
						Type:        "string",
						Description: "Kind of the Istio object (e.g., 'VirtualService', 'DestinationRule').",
						Enum:        []any{"VirtualService", "DestinationRule", "Gateway", "ServiceEntry", "Sidecar", "WorkloadEntry", "WorkloadGroup", "EnvoyFilter", "AuthorizationPolicy", "PeerAuthentication", "RequestAuthentication", "HTTPRoute", "GRPCRoute", "ReferenceGrant", "TCPRoute", "TLSRoute", "InferencePool"},
					},
					"object": {
						Type:        "string",
						Description: "Name of the Istio object.",
					},
					"data": {
						Type:        "string",
						Description: "JSON or YAML data for the resource. Required for create and patch actions. For create, you can provide partial content (e.g. only spec) and it will be merged onto a valid template with defaults. Arrays (like servers, http, etc.) are REPLACED entirely, so include ALL elements you want.",
					},
					"meshCluster": {
						Type:        "string",
						Description: meshClusterDescription(),
					},
				},
				Required: []string{"action", "namespace", "group", "version", "kind", "object"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Manage Istio, Gateway API, and Inference API Config: Create, Patch, Delete",
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
	content, err := kiali.ExecuteRequest(params.Context, KialiManageIstioConfigEndpoint, remapMeshCluster(arguments))
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to manage istio config: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
