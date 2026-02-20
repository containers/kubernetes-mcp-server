package clone

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
)

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_clone",
				Description: "Clone a KubeVirt VirtualMachine by creating a VirtualMachineClone resource. This creates a copy of the source VM with a new name using the KubeVirt Clone API",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the source virtual machine",
						},
						"name": {
							Type:        "string",
							Description: "The name of the source virtual machine to clone",
						},
						"targetName": {
							Type:        "string",
							Description: "The name for the new cloned virtual machine",
						},
					},
					Required: []string{"namespace", "name", "targetName"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Clone",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: cloneVM,
		},
	}
}

func cloneVM(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	targetName, err := api.RequiredString(params, "targetName")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	dynamicClient := params.DynamicClient()

	result, err := kubevirt.CloneVM(params.Context, dynamicClient, namespace, name, targetName)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	marshalledYaml, err := output.MarshalYaml([]*unstructured.Unstructured{result})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal VirtualMachineClone: %w", err)), nil
	}

	return api.NewToolCallResult("# VirtualMachineClone created successfully\n"+marshalledYaml, nil), nil
}
