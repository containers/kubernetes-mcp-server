package kubevirt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

type SnapshotSuite struct {
	suite.Suite
}

func (s *SnapshotSuite) TestCreateSnapshot() {
	s.Run("creates snapshot successfully", func() {
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme)
		ctx := context.Background()

		snapshot, err := CreateSnapshot(ctx, client, "default", "test-vm", "test-snapshot")

		s.NoError(err)
		s.NotNil(snapshot)
		s.Equal("test-snapshot", snapshot.GetName())
		s.Equal("default", snapshot.GetNamespace())
		s.Equal("snapshot.kubevirt.io/v1beta1", snapshot.GetAPIVersion())
		s.Equal("VirtualMachineSnapshot", snapshot.GetKind())

		// Verify spec.source
		sourceName, found, err := unstructured.NestedString(snapshot.Object, "spec", "source", "name")
		s.NoError(err)
		s.True(found)
		s.Equal("test-vm", sourceName)

		sourceKind, found, err := unstructured.NestedString(snapshot.Object, "spec", "source", "kind")
		s.NoError(err)
		s.True(found)
		s.Equal("VirtualMachine", sourceKind)

		sourceAPIGroup, found, err := unstructured.NestedString(snapshot.Object, "spec", "source", "apiGroup")
		s.NoError(err)
		s.True(found)
		s.Equal("kubevirt.io", sourceAPIGroup)
	})

	s.Run("handles different namespaces", func() {
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme)
		ctx := context.Background()

		snapshot, err := CreateSnapshot(ctx, client, "custom-ns", "my-vm", "my-snapshot")

		s.NoError(err)
		s.Equal("custom-ns", snapshot.GetNamespace())
		s.Equal("my-snapshot", snapshot.GetName())
	})
}

func (s *SnapshotSuite) TestListSnapshots() {
	s.Run("lists all snapshots when vm_name is empty", func() {
		snapshot1 := createTestSnapshot("snapshot-1", "default", "vm-1")
		snapshot2 := createTestSnapshot("snapshot-2", "default", "vm-2")

		gvrToListKind := map[schema.GroupVersionResource]string{
			VirtualMachineSnapshotGVR: "VirtualMachineSnapshotList",
		}
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, snapshot1, snapshot2)
		ctx := context.Background()

		snapshots, err := ListSnapshots(ctx, client, "default", "")

		s.NoError(err)
		s.Len(snapshots, 2)
	})

	s.Run("filters snapshots by vm_name", func() {
		snapshot1 := createTestSnapshot("snapshot-1", "default", "vm-1")
		snapshot2 := createTestSnapshot("snapshot-2", "default", "vm-2")
		snapshot3 := createTestSnapshot("snapshot-3", "default", "vm-1")

		gvrToListKind := map[schema.GroupVersionResource]string{
			VirtualMachineSnapshotGVR: "VirtualMachineSnapshotList",
		}
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, snapshot1, snapshot2, snapshot3)
		ctx := context.Background()

		snapshots, err := ListSnapshots(ctx, client, "default", "vm-1")

		s.NoError(err)
		s.Len(snapshots, 2)
		for _, snap := range snapshots {
			sourceName, _, _ := unstructured.NestedString(snap.Object, "spec", "source", "name")
			s.Equal("vm-1", sourceName)
		}
	})

	s.Run("returns empty list when no snapshots exist", func() {
		gvrToListKind := map[schema.GroupVersionResource]string{
			VirtualMachineSnapshotGVR: "VirtualMachineSnapshotList",
		}
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind)
		ctx := context.Background()

		snapshots, err := ListSnapshots(ctx, client, "default", "vm-1")

		s.NoError(err)
		s.Len(snapshots, 0)
	})

	s.Run("handles snapshots with missing source name", func() {
		validSnapshot := createTestSnapshot("valid-snapshot", "default", "vm-1")
		invalidSnapshot := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "snapshot.kubevirt.io/v1beta1",
				"kind":       "VirtualMachineSnapshot",
				"metadata": map[string]any{
					"name":      "invalid-snapshot",
					"namespace": "default",
				},
				"spec": map[string]any{
					"source": map[string]any{
						// Missing "name" field
						"kind": "VirtualMachine",
					},
				},
			},
		}

		gvrToListKind := map[schema.GroupVersionResource]string{
			VirtualMachineSnapshotGVR: "VirtualMachineSnapshotList",
		}
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, validSnapshot, invalidSnapshot)
		ctx := context.Background()

		snapshots, err := ListSnapshots(ctx, client, "default", "vm-1")

		s.NoError(err)
		s.Len(snapshots, 1, "should skip snapshot with missing source name")
		s.Equal("valid-snapshot", snapshots[0].GetName())
	})

	s.Run("filters by namespace", func() {
		snapshot1 := createTestSnapshot("snapshot-1", "namespace-1", "vm-1")
		snapshot2 := createTestSnapshot("snapshot-2", "namespace-2", "vm-1")

		gvrToListKind := map[schema.GroupVersionResource]string{
			VirtualMachineSnapshotGVR: "VirtualMachineSnapshotList",
		}
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, snapshot1, snapshot2)
		ctx := context.Background()

		snapshots, err := ListSnapshots(ctx, client, "namespace-1", "vm-1")

		s.NoError(err)
		s.Len(snapshots, 1)
		s.Equal("namespace-1", snapshots[0].GetNamespace())
	})
}

func (s *SnapshotSuite) TestGetSnapshot() {
	s.Run("retrieves snapshot successfully", func() {
		snapshot := createTestSnapshot("test-snapshot", "default", "test-vm")

		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme, snapshot)
		ctx := context.Background()

		result, err := GetSnapshot(ctx, client, "default", "test-snapshot")

		s.NoError(err)
		s.NotNil(result)
		s.Equal("test-snapshot", result.GetName())
		s.Equal("default", result.GetNamespace())
	})

	s.Run("returns error for non-existent snapshot", func() {
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme)
		ctx := context.Background()

		_, err := GetSnapshot(ctx, client, "default", "non-existent-snapshot")

		s.Error(err)
	})

	s.Run("handles different namespaces", func() {
		snapshot := createTestSnapshot("test-snapshot", "custom-ns", "test-vm")

		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme, snapshot)
		ctx := context.Background()

		result, err := GetSnapshot(ctx, client, "custom-ns", "test-snapshot")

		s.NoError(err)
		s.Equal("custom-ns", result.GetNamespace())
	})
}

func (s *SnapshotSuite) TestDeleteSnapshot() {
	s.Run("deletes snapshot successfully", func() {
		snapshot := createTestSnapshot("test-snapshot", "default", "test-vm")

		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme, snapshot)
		ctx := context.Background()

		err := DeleteSnapshot(ctx, client, "default", "test-snapshot")

		s.NoError(err)

		// Verify snapshot was deleted
		_, getErr := GetSnapshot(ctx, client, "default", "test-snapshot")
		s.Error(getErr, "snapshot should be deleted")
	})

	s.Run("handles deleting non-existent snapshot", func() {
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme)
		ctx := context.Background()

		err := DeleteSnapshot(ctx, client, "default", "non-existent-snapshot")

		s.Error(err)
	})

	s.Run("deletes from correct namespace", func() {
		snapshot1 := createTestSnapshot("same-name", "namespace-1", "test-vm")
		snapshot2 := createTestSnapshot("same-name", "namespace-2", "test-vm")

		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme, snapshot1, snapshot2)
		ctx := context.Background()

		err := DeleteSnapshot(ctx, client, "namespace-1", "same-name")

		s.NoError(err)

		// Verify only the snapshot in namespace-1 was deleted
		_, err1 := GetSnapshot(ctx, client, "namespace-1", "same-name")
		s.Error(err1, "snapshot in namespace-1 should be deleted")

		result2, err2 := GetSnapshot(ctx, client, "namespace-2", "same-name")
		s.NoError(err2, "snapshot in namespace-2 should still exist")
		s.Equal("namespace-2", result2.GetNamespace())
	})
}

func (s *SnapshotSuite) TestRestoreSnapshot() {
	s.Run("creates restore successfully", func() {
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme)
		ctx := context.Background()

		restore, err := RestoreSnapshot(ctx, client, "default", "test-vm", "test-snapshot")

		s.NoError(err)
		s.NotNil(restore)
		s.Equal("default", restore.GetNamespace())
		s.Equal("snapshot.kubevirt.io/v1beta1", restore.GetAPIVersion())
		s.Equal("VirtualMachineRestore", restore.GetKind())

		// Verify restore name includes VM name
		s.Contains(restore.GetName(), "test-vm-restore-")

		// Verify spec.target
		targetName, found, err := unstructured.NestedString(restore.Object, "spec", "target", "name")
		s.NoError(err)
		s.True(found)
		s.Equal("test-vm", targetName)

		targetKind, found, err := unstructured.NestedString(restore.Object, "spec", "target", "kind")
		s.NoError(err)
		s.True(found)
		s.Equal("VirtualMachine", targetKind)

		targetAPIGroup, found, err := unstructured.NestedString(restore.Object, "spec", "target", "apiGroup")
		s.NoError(err)
		s.True(found)
		s.Equal("kubevirt.io", targetAPIGroup)

		// Verify spec.virtualMachineSnapshotName
		snapshotName, found, err := unstructured.NestedString(restore.Object, "spec", "virtualMachineSnapshotName")
		s.NoError(err)
		s.True(found)
		s.Equal("test-snapshot", snapshotName)
	})

	s.Run("restore name contains vm name", func() {
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme)
		ctx := context.Background()

		restore, err := RestoreSnapshot(ctx, client, "default", "test-vm", "snapshot-1")

		s.NoError(err)
		s.Contains(restore.GetName(), "test-vm-restore-", "restore name should contain VM name")
		s.NotEqual("test-vm-restore-", restore.GetName(), "restore name should have timestamp suffix")
	})

	s.Run("handles different namespaces", func() {
		scheme := runtime.NewScheme()
		client := fake.NewSimpleDynamicClient(scheme)
		ctx := context.Background()

		restore, err := RestoreSnapshot(ctx, client, "custom-ns", "my-vm", "my-snapshot")

		s.NoError(err)
		s.Equal("custom-ns", restore.GetNamespace())
	})
}

// Helper function to create test snapshots
func createTestSnapshot(name, namespace, vmName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "snapshot.kubevirt.io/v1beta1",
			"kind":       "VirtualMachineSnapshot",
			"metadata": map[string]any{
				"name":      name,
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
}

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}
