package snapshot

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
)

// FeatureGateAction represents the action to perform on the snapshot feature gate
type FeatureGateAction string

const (
	FeatureGateActionEnable  FeatureGateAction = "enable"
	FeatureGateActionDisable FeatureGateAction = "disable"
)

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_snapshot_create",
				Description: "Create a snapshot of a VirtualMachine. The VM can be running or stopped when creating a snapshot.",
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
							Description: "The name for the new snapshot",
						},
					},
					Required: []string{"namespace", "vm_name", "snapshot_name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Create Snapshot",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: createSnapshot,
		},
		{
			Tool: api.Tool{
				Name:        "vm_snapshot_restore",
				Description: "Restore a VirtualMachine from a snapshot. This creates a VirtualMachineRestore resource that restores the VM to the state captured in the snapshot.",
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
						"restore_name": {
							Type:        "string",
							Description: "Optional name for the restore operation. If not provided, defaults to '<snapshot_name>-restore'",
						},
					},
					Required: []string{"namespace", "vm_name", "snapshot_name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Restore from Snapshot",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: restoreSnapshot,
		},
		{
			Tool: api.Tool{
				Name:        "vm_snapshot_list",
				Description: "List all VirtualMachineSnapshots in a namespace",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace to list snapshots from",
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
			Handler: listSnapshots,
		},
		{
			Tool: api.Tool{
				Name:        "vm_snapshot_get",
				Description: "Get details of a specific VirtualMachineSnapshot",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the snapshot",
						},
						"snapshot_name": {
							Type:        "string",
							Description: "The name of the snapshot",
						},
					},
					Required: []string{"namespace", "snapshot_name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Get Snapshot",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: getSnapshot,
		},
		{
			Tool: api.Tool{
				Name:        "vm_snapshot_feature_gate",
				Description: "Enable or disable the Snapshot feature gate in the KubeVirt CR. The Snapshot feature must be enabled before creating snapshots.",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the KubeVirt CR",
						},
						"name": {
							Type:        "string",
							Description: "The name of the KubeVirt CR (usually 'kubevirt')",
						},
						"action": {
							Type:        "string",
							Enum:        []any{string(FeatureGateActionEnable), string(FeatureGateActionDisable)},
							Description: "The action to perform: 'enable' to enable the Snapshot feature gate, 'disable' to disable it",
						},
					},
					Required: []string{"namespace", "name", "action"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Manage Snapshot Feature Gate",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: manageFeatureGate,
		},
	}
}

func createSnapshot(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
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

	snapshot, err := kubevirt.CreateVMSnapshot(
		params.Context,
		params.DynamicClient(),
		namespace,
		vmName,
		snapshotName,
	)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	marshalledYaml, err := output.MarshalYaml([]*unstructured.Unstructured{snapshot})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal snapshot: %w", err)), nil
	}

	return api.NewToolCallResult(
		fmt.Sprintf("# VirtualMachineSnapshot '%s' created successfully\n%s", snapshotName, marshalledYaml),
		nil,
	), nil
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

	// Use snapshot_name-restore as default if restore_name not provided
	restoreName := api.OptionalString(params, "restore_name", snapshotName+"-restore")

	restore, err := kubevirt.RestoreVMSnapshot(
		params.Context,
		params.DynamicClient(),
		namespace,
		vmName,
		snapshotName,
		restoreName,
	)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	marshalledYaml, err := output.MarshalYaml([]*unstructured.Unstructured{restore})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal restore: %w", err)), nil
	}

	return api.NewToolCallResult(
		fmt.Sprintf("# VirtualMachineRestore '%s' created successfully\n%s", restoreName, marshalledYaml),
		nil,
	), nil
}

func listSnapshots(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	snapshots, err := kubevirt.ListVMSnapshots(
		params.Context,
		params.DynamicClient(),
		namespace,
	)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	if len(snapshots) == 0 {
		return api.NewToolCallResult(
			fmt.Sprintf("# No VirtualMachineSnapshots found in namespace '%s'\n", namespace),
			nil,
		), nil
	}

	// Convert []unstructured.Unstructured to []*unstructured.Unstructured
	snapshotPtrs := make([]*unstructured.Unstructured, len(snapshots))
	for i := range snapshots {
		snapshotPtrs[i] = &snapshots[i]
	}

	marshalledYaml, err := output.MarshalYaml(snapshotPtrs)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal snapshots: %w", err)), nil
	}

	return api.NewToolCallResult(
		fmt.Sprintf("# VirtualMachineSnapshots in namespace '%s'\n%s", namespace, marshalledYaml),
		nil,
	), nil
}

func getSnapshot(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	snapshotName, err := api.RequiredString(params, "snapshot_name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	snapshot, err := kubevirt.GetVMSnapshot(
		params.Context,
		params.DynamicClient(),
		namespace,
		snapshotName,
	)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	marshalledYaml, err := output.MarshalYaml([]*unstructured.Unstructured{snapshot})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal snapshot: %w", err)), nil
	}

	return api.NewToolCallResult(
		fmt.Sprintf("# VirtualMachineSnapshot '%s'\n%s", snapshotName, marshalledYaml),
		nil,
	), nil
}

func manageFeatureGate(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	action, err := api.RequiredString(params, "action")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	var kubevirtCR *unstructured.Unstructured
	var message string

	switch FeatureGateAction(action) {
	case FeatureGateActionEnable:
		kubevirtCR, err = kubevirt.EnableSnapshotFeatureGate(
			params.Context,
			params.DynamicClient(),
			namespace,
			name,
		)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		message = "# Snapshot feature gate enabled successfully\n"

	case FeatureGateActionDisable:
		kubevirtCR, err = kubevirt.DisableSnapshotFeatureGate(
			params.Context,
			params.DynamicClient(),
			namespace,
			name,
		)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		message = "# Snapshot feature gate disabled successfully\n"

	default:
		return api.NewToolCallResult("", fmt.Errorf("invalid action '%s': must be 'enable' or 'disable'", action)), nil
	}

	marshalledYaml, err := output.MarshalYaml([]*unstructured.Unstructured{kubevirtCR})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal KubeVirt CR: %w", err)), nil
	}

	return api.NewToolCallResult(message+marshalledYaml, nil), nil
}
