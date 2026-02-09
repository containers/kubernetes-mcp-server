package list

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
				Name:        "vm_snapshot_list",
				Description: "List VirtualMachine snapshots for a specific VM or all snapshots in a namespace",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace to list snapshots from",
						},
						"vm_name": {
							Type:        "string",
							Description: "Optional name of the virtual machine to filter snapshots. If not provided, lists all snapshots in the namespace",
						},
					},
					Required: []string{"namespace"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: List Snapshots",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: list,
		},
	}
}

func list(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	vmName := api.OptionalString(params, "vm_name", "")

	dynamicClient := params.DynamicClient()

	snapshots, err := kubevirt.ListSnapshots(params.Context, dynamicClient, namespace, vmName)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list snapshots: %w", err)), nil
	}

	if len(snapshots) == 0 {
		message := fmt.Sprintf("# No snapshots found in namespace '%s'", namespace)
		if vmName != "" {
			message = fmt.Sprintf("# No snapshots found for VM '%s' in namespace '%s'", vmName, namespace)
		}
		return api.NewToolCallResult(message+"\n", nil), nil
	}

	// Convert to pointer slice for MarshalYaml
	snapshotPtrs := make([]*unstructured.Unstructured, len(snapshots))
	for i := range snapshots {
		snapshotPtrs[i] = &snapshots[i]
	}

	marshalledYaml, err := output.MarshalYaml(snapshotPtrs)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal snapshots: %w", err)), nil
	}

	message := fmt.Sprintf("# Found %d snapshot(s)", len(snapshots))
	if vmName != "" {
		message = fmt.Sprintf("# Found %d snapshot(s) for VM '%s'", len(snapshots), vmName)
	}
	return api.NewToolCallResult(message+"\n"+marshalledYaml, nil), nil
}
