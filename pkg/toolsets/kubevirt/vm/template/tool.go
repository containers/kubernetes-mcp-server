package template

import (
	"encoding/json"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"
)

func Tools(p api.FilteringProvider) []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_create_from_template",
				Description: fmt.Sprintf("Create a VirtualMachine from a VirtualMachineTemplate (virt-template) on %s. Processes the template server-side to substitute parameters (required values, defaults, and auto-generated values like passwords), then creates the resulting VirtualMachine in the same namespace. Cross-namespace template usage is not supported.", defaults.ProductName()),
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the VirtualMachineTemplate and the resulting VirtualMachine (must be the same)",
						},
						"template_name": {
							Type:        "string",
							Description: "The name of the VirtualMachineTemplate to create the VM from",
						},
						"parameters": {
							Type:        "object",
							Description: "Parameter values to substitute in the template. Keys are parameter names (e.g. VM_NAME), values are strings. Required parameters must be provided; optional parameters use their defaults if omitted; parameters with generate/from will be auto-generated if not provided.",
							AdditionalProperties: &jsonschema.Schema{
								Type: "string",
							},
						},
					},
					Required: []string{"namespace", "template_name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Create from Template",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: createFromTemplate,
			TargetCompatibilityFilters: []func() bool{
				kubevirt.HasVirtualMachineTemplate(p),
			},
		},
	}
}

func createFromTemplate(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	namespace := p.RequiredString("namespace")
	templateName := p.RequiredString("template_name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	templateParams, err := extractParameters(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	result, err := kubevirt.CreateVMFromTemplate(params.Context, params.RESTConfig(), namespace, templateName, templateParams)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	marshalledYaml, err := output.MarshalYaml(result)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal created VirtualMachine: %w", err)), nil
	}

	return api.NewToolCallResult("# VirtualMachine created from template successfully\n"+marshalledYaml, nil), nil
}

func extractParameters(params api.ToolHandlerParams) (map[string]string, error) {
	args := params.GetArguments()
	raw, ok := args["parameters"]
	if !ok || raw == nil {
		return nil, nil
	}

	switch v := raw.(type) {
	case map[string]any:
		result := make(map[string]string, len(v))
		for key, val := range v {
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("parameter %q value must be a string, got %T", key, val)
			}
			result[key] = s
		}
		return result, nil
	case map[string]string:
		return v, nil
	default:
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("parameters must be an object mapping parameter names to string values")
		}
		var result map[string]string
		if err := json.Unmarshal(b, &result); err != nil {
			return nil, fmt.Errorf("parameters must be an object mapping parameter names to string values")
		}
		return result, nil
	}
}
