package delete

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"
)

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_snapshot_delete",
				Description: "Delete a VirtualMachine snapshot",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the snapshot",
						},
						"snapshot_name": {
							Type:        "string",
							Description: "The name of the snapshot to delete",
						},
					},
					Required: []string{"namespace", "snapshot_name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Delete Snapshot",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: deleteSnapshot,
		},
	}
}

func deleteSnapshot(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	snapshotName, err := api.RequiredString(params, "snapshot_name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	dynamicClient := params.DynamicClient()

	err = kubevirt.DeleteSnapshot(params.Context, dynamicClient, namespace, snapshotName)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete snapshot: %w", err)), nil
	}

	message := fmt.Sprintf("# VirtualMachineSnapshot '%s' in namespace '%s' deleted successfully\n", snapshotName, namespace)
	return api.NewToolCallResult(message, nil), nil
}
