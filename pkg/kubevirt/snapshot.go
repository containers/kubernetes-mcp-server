package kubevirt

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	// VirtualMachineSnapshotGVR is the GroupVersionResource for VirtualMachineSnapshot resources
	VirtualMachineSnapshotGVR = schema.GroupVersionResource{
		Group:    "snapshot.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachinesnapshots",
	}

	// VirtualMachineRestoreGVR is the GroupVersionResource for VirtualMachineRestore resources
	VirtualMachineRestoreGVR = schema.GroupVersionResource{
		Group:    "snapshot.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachinerestores",
	}

	// KubeVirtGVR is the GroupVersionResource for KubeVirt resources
	KubeVirtGVR = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "kubevirts",
	}
)

// CreateVMSnapshot creates a VirtualMachineSnapshot for the specified VM
func CreateVMSnapshot(ctx context.Context, client dynamic.Interface, namespace, vmName, snapshotName string) (*unstructured.Unstructured, error) {
	snapshot := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "snapshot.kubevirt.io/v1beta1",
			"kind":       "VirtualMachineSnapshot",
			"metadata": map[string]any{
				"name":      snapshotName,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"source": map[string]any{
					"apiGroup": "kubevirt.io",
					"kind":     "VirtualMachine",
					"name":     vmName,
				},
			},
		},
	}

	created, err := client.Resource(VirtualMachineSnapshotGVR).
		Namespace(namespace).
		Create(ctx, snapshot, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create VirtualMachineSnapshot: %w", err)
	}

	return created, nil
}

// RestoreVMSnapshot creates a VirtualMachineRestore to restore a VM from a snapshot
func RestoreVMSnapshot(ctx context.Context, client dynamic.Interface, namespace, vmName, snapshotName, restoreName string) (*unstructured.Unstructured, error) {
	restore := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "snapshot.kubevirt.io/v1beta1",
			"kind":       "VirtualMachineRestore",
			"metadata": map[string]any{
				"name":      restoreName,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"target": map[string]any{
					"apiGroup": "kubevirt.io",
					"kind":     "VirtualMachine",
					"name":     vmName,
				},
				"virtualMachineSnapshotName": snapshotName,
			},
		},
	}
	created, err := client.Resource(VirtualMachineRestoreGVR).
		Namespace(namespace).
		Create(ctx, restore, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create VirtualMachineRestore: %w", err)
	}

	return created, nil
}

// ListVMSnapshots lists all VirtualMachineSnapshots in the specified namespace
func ListVMSnapshots(ctx context.Context, client dynamic.Interface, namespace string) ([]unstructured.Unstructured, error) {
	list, err := client.Resource(VirtualMachineSnapshotGVR).
		Namespace(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineSnapshots: %w", err)
	}

	return list.Items, nil
}

// GetVMSnapshot retrieves a specific VirtualMachineSnapshot
func GetVMSnapshot(ctx context.Context, client dynamic.Interface, namespace, snapshotName string) (*unstructured.Unstructured, error) {
	snapshot, err := client.Resource(VirtualMachineSnapshotGVR).
		Namespace(namespace).
		Get(ctx, snapshotName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get VirtualMachineSnapshot: %w", err)
	}

	return snapshot, nil
}

// EnableSnapshotFeatureGate enables the Snapshot feature gate in the KubeVirt CR
func EnableSnapshotFeatureGate(ctx context.Context, client dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
	return updateSnapshotFeatureGate(ctx, client, namespace, name, true)
}

// DisableSnapshotFeatureGate disables the Snapshot feature gate in the KubeVirt CR
func DisableSnapshotFeatureGate(ctx context.Context, client dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
	return updateSnapshotFeatureGate(ctx, client, namespace, name, false)
}

// updateSnapshotFeatureGate updates the Snapshot feature gate in the KubeVirt CR
func updateSnapshotFeatureGate(ctx context.Context, client dynamic.Interface, namespace, name string, enable bool) (*unstructured.Unstructured, error) {
	// Get the KubeVirt CR
	kubevirt, err := client.Resource(KubeVirtGVR).
		Namespace(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get KubeVirt CR: %w", err)
	}

	// Get current feature gates
	featureGates, _, err := unstructured.NestedStringSlice(kubevirt.Object, "spec", "configuration", "developerConfiguration", "featureGates")
	if err != nil {
		return nil, fmt.Errorf("failed to read feature gates: %w", err)
	}

	// Check if Snapshot feature gate is already in the list
	snapshotIndex := -1
	for i, gate := range featureGates {
		if gate == "Snapshot" {
			snapshotIndex = i
			break
		}
	}

	if enable {
		// Add Snapshot feature gate if not already present
		if snapshotIndex == -1 {
			featureGates = append(featureGates, "Snapshot")
		} else {
			// Already enabled
			return kubevirt, nil
		}
	} else {
		// Remove Snapshot feature gate if present
		if snapshotIndex != -1 {
			featureGates = append(featureGates[:snapshotIndex], featureGates[snapshotIndex+1:]...)
		} else {
			// Already disabled
			return kubevirt, nil
		}
	}

	// Update the feature gates
	if err := unstructured.SetNestedStringSlice(kubevirt.Object, featureGates, "spec", "configuration", "developerConfiguration", "featureGates"); err != nil {
		return nil, fmt.Errorf("failed to set feature gates: %w", err)
	}

	// Update the KubeVirt CR
	updated, err := client.Resource(KubeVirtGVR).
		Namespace(namespace).
		Update(ctx, kubevirt, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update KubeVirt CR: %w", err)
	}

	return updated, nil
}
