package restore

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
				Name:        "vm_snapshot_restore",
				Description: "Restore a VirtualMachine to a previous snapshot state",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the virtual machine",
						},
						"vm_name": {
							Type:        "string",
							Description: "The name of the virtual machine to restore",
						},
						"snapshot_name": {
							Type:        "string",
							Description: "The name of the snapshot to restore from",
						},
					},
					Required: []string{"namespace", "vm_name", "snapshot_name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Restore Snapshot",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: restoreSnapshot,
		},
	}
}

func restoreSnapshot(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	vmName, err := api.RequiredString(params, "vm_name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	snapshotName, err := api.RequiredString(params, "snapshot_name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	dynamicClient := params.DynamicClient()

	restore, err := kubevirt.RestoreSnapshot(params.Context, dynamicClient, namespace, vmName, snapshotName)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to restore snapshot: %w", err)), nil
	}

	marshalledYaml, err := output.MarshalYaml([]*unstructured.Unstructured{restore})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal restore: %w", err)), nil
	}

	message := fmt.Sprintf("# VirtualMachineRestore created successfully to restore VM '%s' from snapshot '%s'\n", vmName, snapshotName)
	return api.NewToolCallResult(message+marshalledYaml, nil), nil
}
