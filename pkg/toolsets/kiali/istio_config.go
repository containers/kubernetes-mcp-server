package kiali

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initIstioConfig() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "istio_config",
			Description: "Get all Istio configuration objects in the mesh including their full YAML resources and details",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{},
				Required:   []string{},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Istio Config: List All",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: istioConfigHandler,
	})
	return ret
}

func istioConfigHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	k := params.NewKiali()
	content, err := k.IstioConfig(params.Context)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve Istio configuration: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func initIstioObjectDetails() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "istio_object_details",
			Description: "Get detailed information about a specific Istio object including validation and help information",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
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
				Required: []string{"namespace", "group", "version", "kind", "name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Istio Object: Details",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: istioObjectDetailsHandler,
	})
	return ret
}

func istioObjectDetailsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract required parameters
	namespace, _ := params.GetArguments()["namespace"].(string)
	group, _ := params.GetArguments()["group"].(string)
	version, _ := params.GetArguments()["version"].(string)
	kind, _ := params.GetArguments()["kind"].(string)
	name, _ := params.GetArguments()["name"].(string)

	k := params.NewKiali()
	content, err := k.IstioObjectDetails(params.Context, namespace, group, version, kind, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve Istio object details: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func initIstioObjectPatch() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "istio_object_patch",
			Description: "Modify an existing Istio object using PATCH method. The JSON patch data will be applied to the existing object.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
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
					"json_patch": {
						Type:        "string",
						Description: "JSON patch data to apply to the object",
					},
				},
				Required: []string{"namespace", "group", "version", "kind", "name", "json_patch"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Istio Object: Patch",
				ReadOnlyHint:    ptr.To(false),
				DestructiveHint: ptr.To(true),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(false),
			},
		}, Handler: istioObjectPatchHandler,
	})
	return ret
}

func istioObjectPatchHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract required parameters
	namespace, _ := params.GetArguments()["namespace"].(string)
	group, _ := params.GetArguments()["group"].(string)
	version, _ := params.GetArguments()["version"].(string)
	kind, _ := params.GetArguments()["kind"].(string)
	name, _ := params.GetArguments()["name"].(string)
	jsonPatch, _ := params.GetArguments()["json_patch"].(string)

	k := params.NewKiali()
	content, err := k.IstioObjectPatch(params.Context, namespace, group, version, kind, name, jsonPatch)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to patch Istio object: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func initIstioObjectCreate() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "istio_object_create",
			Description: "Create a new Istio object using POST method. The JSON data will be used to create the new object.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace where the Istio object will be created",
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
					"json_data": {
						Type:        "string",
						Description: "JSON data for the new object",
					},
				},
				Required: []string{"namespace", "group", "version", "kind", "json_data"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Istio Object: Create",
				ReadOnlyHint:    ptr.To(false),
				DestructiveHint: ptr.To(true),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(false),
			},
		}, Handler: istioObjectCreateHandler,
	})
	return ret
}

func istioObjectCreateHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract required parameters
	namespace, _ := params.GetArguments()["namespace"].(string)
	group, _ := params.GetArguments()["group"].(string)
	version, _ := params.GetArguments()["version"].(string)
	kind, _ := params.GetArguments()["kind"].(string)
	jsonData, _ := params.GetArguments()["json_data"].(string)

	k := params.NewKiali()
	content, err := k.IstioObjectCreate(params.Context, namespace, group, version, kind, jsonData)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Istio object: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func initIstioObjectDelete() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "istio_object_delete",
			Description: "Delete an existing Istio object using DELETE method.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
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
				Required: []string{"namespace", "group", "version", "kind", "name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Istio Object: Delete",
				ReadOnlyHint:    ptr.To(false),
				DestructiveHint: ptr.To(true),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(false),
			},
		}, Handler: istioObjectDeleteHandler,
	})
	return ret
}

func istioObjectDeleteHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract required parameters
	namespace, _ := params.GetArguments()["namespace"].(string)
	group, _ := params.GetArguments()["group"].(string)
	version, _ := params.GetArguments()["version"].(string)
	kind, _ := params.GetArguments()["kind"].(string)
	name, _ := params.GetArguments()["name"].(string)

	k := params.NewKiali()
	content, err := k.IstioObjectDelete(params.Context, namespace, group, version, kind, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete Istio object: %v", err)), nil
	}

	return api.NewToolCallResult(content, nil), nil
}
