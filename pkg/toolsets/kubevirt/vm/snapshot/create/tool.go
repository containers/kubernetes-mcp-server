package create

import (
	"fmt"
	"time"

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
				Name:        "vm_snapshot_create",
				Description: "Create a snapshot of a VirtualMachine for backup or cloning purposes",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the virtual machine",
						},
						"vm_name": {
							Type:        "string",
							Description: "The name of the virtual machine to snapshot",
						},
						"snapshot_name": {
							Type:        "string",
							Description: "Optional name for the snapshot. If not provided, a name will be generated",
						},
					},
					Required: []string{"namespace", "vm_name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Create Snapshot",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: create,
		},
	}
}

func create(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	vmName, err := api.RequiredString(params, "vm_name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	snapshotName := api.OptionalString(params, "snapshot_name", "")
	if snapshotName == "" {
		snapshotName = fmt.Sprintf("%s-snapshot-%d", vmName, time.Now().Unix())
	}

	dynamicClient := params.DynamicClient()

	snapshot, err := kubevirt.CreateSnapshot(params.Context, dynamicClient, namespace, vmName, snapshotName)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create snapshot: %w", err)), nil
	}

	marshalledYaml, err := output.MarshalYaml([]*unstructured.Unstructured{snapshot})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal snapshot: %w", err)), nil
	}

	message := fmt.Sprintf("# VirtualMachineSnapshot '%s' created successfully for VM '%s'\n", snapshotName, vmName)
	return api.NewToolCallResult(message+marshalledYaml, nil), nil
}
