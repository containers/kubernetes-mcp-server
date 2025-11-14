package delete

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_delete",
				Description: "Delete a VirtualMachine in the current cluster by providing its namespace and name",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the virtual machine",
						},
						"name": {
							Type:        "string",
							Description: "The name of the virtual machine to delete",
						},
					},
					Required: []string{"namespace", "name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Delete",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: deleteVM,
		},
	}
}

func deleteVM(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse required parameters
	namespace, err := params.GetRequiredString("namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	name, err := params.GetRequiredString("name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	// Define the VirtualMachine GVK
	gvk := schema.GroupVersionKind{
		Group:   "kubevirt.io",
		Version: "v1",
		Kind:    "VirtualMachine",
	}

	// Delete the VM using the access-controlled method
	err = params.ResourcesDelete(params.Context, &gvk, namespace, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete VirtualMachine: %w", err)), nil
	}

	return api.NewToolCallResult(fmt.Sprintf("# VirtualMachine deleted successfully\nVirtualMachine '%s' in namespace '%s' has been deleted.", name, namespace), nil), nil
}
