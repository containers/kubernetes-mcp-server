package kubevirt

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// CreateSnapshot creates a VirtualMachineSnapshot for the specified VM
func CreateSnapshot(ctx context.Context, client dynamic.Interface, namespace, vmName, snapshotName string) (*unstructured.Unstructured, error) {
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

	return client.Resource(VirtualMachineSnapshotGVR).
		Namespace(namespace).
		Create(ctx, snapshot, metav1.CreateOptions{})
}

// ListSnapshots lists all VirtualMachineSnapshots for a VM
func ListSnapshots(ctx context.Context, client dynamic.Interface, namespace, vmName string) ([]unstructured.Unstructured, error) {
	snapshots, err := client.Resource(VirtualMachineSnapshotGVR).
		Namespace(namespace).
		List(ctx, metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	// Filter snapshots for the specific VM if vmName is provided
	if vmName == "" {
		return snapshots.Items, nil
	}

	var result []unstructured.Unstructured
	for _, snapshot := range snapshots.Items {
		sourceName, found, err := unstructured.NestedString(snapshot.Object, "spec", "source", "name")
		if err != nil || !found {
			continue
		}
		if sourceName == vmName {
			result = append(result, snapshot)
		}
	}

	return result, nil
}

// GetSnapshot retrieves a VirtualMachineSnapshot by name
func GetSnapshot(ctx context.Context, client dynamic.Interface, namespace, snapshotName string) (*unstructured.Unstructured, error) {
	return client.Resource(VirtualMachineSnapshotGVR).
		Namespace(namespace).
		Get(ctx, snapshotName, metav1.GetOptions{})
}

// DeleteSnapshot deletes a VirtualMachineSnapshot
func DeleteSnapshot(ctx context.Context, client dynamic.Interface, namespace, snapshotName string) error {
	return client.Resource(VirtualMachineSnapshotGVR).
		Namespace(namespace).
		Delete(ctx, snapshotName, metav1.DeleteOptions{})
}

// RestoreSnapshot creates a VirtualMachineRestore to restore a VM from a snapshot
func RestoreSnapshot(ctx context.Context, client dynamic.Interface, namespace, vmName, snapshotName string) (*unstructured.Unstructured, error) {
	restoreName := fmt.Sprintf("%s-restore-%d", vmName, time.Now().Unix())

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

	return client.Resource(VirtualMachineRestoreGVR).
		Namespace(namespace).
		Create(ctx, restore, metav1.CreateOptions{})
}
