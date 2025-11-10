package stop

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"
)

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_stop",
				Description: "Stop a running VirtualMachine by changing its runStrategy to Halted",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the virtual machine",
						},
						"name": {
							Type:        "string",
							Description: "The name of the virtual machine to stop",
						},
					},
					Required: []string{"namespace", "name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Stop",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: stop,
		},
	}
}

func stop(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse required parameters
	namespace, err := params.GetRequiredString("namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	name, err := params.GetRequiredString("name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	// Get dynamic client
	restConfig := params.RESTConfig()
	if restConfig == nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get REST config")), nil
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create dynamic client: %w", err)), nil
	}

	// Get the current VM
	gvr := schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}

	vm, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(
		params.Context,
		name,
		metav1.GetOptions{},
	)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get VirtualMachine: %w", err)), nil
	}

	// Update runStrategy to Halted
	if err := unstructured.SetNestedField(vm.Object, "Halted", "spec", "runStrategy"); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to set runStrategy: %w", err)), nil
	}

	// Update the VM
	updatedVM, err := dynamicClient.Resource(gvr).Namespace(namespace).Update(
		params.Context,
		vm,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to update VirtualMachine: %w", err)), nil
	}

	// Format the output
	marshalledYaml, err := output.MarshalYaml(updatedVM)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal VirtualMachine: %w", err)), nil
	}

	return api.NewToolCallResult("# VirtualMachine stopped successfully\n"+marshalledYaml, nil), nil
}
