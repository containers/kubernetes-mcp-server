package kubevirt

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

// createTestSnapshot creates a test VirtualMachineSnapshot
func createTestSnapshot(name, namespace, vmName string) *unstructured.Unstructured {
	snapshot := &unstructured.Unstructured{}
	snapshot.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "snapshot.kubevirt.io/v1beta1",
		"kind":       "VirtualMachineSnapshot",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"source": map[string]interface{}{
				"apiGroup": "kubevirt.io",
				"kind":     "VirtualMachine",
				"name":     vmName,
			},
		},
	})
	return snapshot
}

// createTestKubeVirt creates a test KubeVirt CR with optional feature gates
func createTestKubeVirt(name, namespace string, featureGates []string) *unstructured.Unstructured {
	kv := &unstructured.Unstructured{}
	content := map[string]interface{}{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "KubeVirt",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"configuration": map[string]interface{}{
				"developerConfiguration": map[string]interface{}{},
			},
		},
	}

	if len(featureGates) > 0 {
		// Convert []string to []interface{} for proper deep copy in fake client
		gates := make([]interface{}, len(featureGates))
		for i, gate := range featureGates {
			gates[i] = gate
		}
		content["spec"].(map[string]interface{})["configuration"].(map[string]interface{})["developerConfiguration"].(map[string]interface{})["featureGates"] = gates
	}

	kv.SetUnstructuredContent(content)
	return kv
}

func TestCreateVMSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		vmName        string
		snapshotName  string
		wantError     bool
		errorContains string
	}{
		{
			name:         "Create snapshot successfully",
			namespace:    "default",
			vmName:       "test-vm",
			snapshotName: "test-snapshot",
			wantError:    false,
		},
		{
			name:         "Create snapshot with different names",
			namespace:    "test-namespace",
			vmName:       "my-vm",
			snapshotName: "my-snapshot",
			wantError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			client := fake.NewSimpleDynamicClient(scheme)
			ctx := context.Background()

			snapshot, err := CreateVMSnapshot(ctx, client, tt.namespace, tt.vmName, tt.snapshotName)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if snapshot == nil {
				t.Errorf("Expected non-nil snapshot, got nil")
				return
			}

			// Verify snapshot metadata
			if snapshot.GetName() != tt.snapshotName {
				t.Errorf("Snapshot name = %q, want %q", snapshot.GetName(), tt.snapshotName)
			}
			if snapshot.GetNamespace() != tt.namespace {
				t.Errorf("Snapshot namespace = %q, want %q", snapshot.GetNamespace(), tt.namespace)
			}

			// Verify snapshot spec
			spec, ok := snapshot.Object["spec"].(map[string]interface{})
			if !ok {
				t.Errorf("Failed to get snapshot spec")
				return
			}
			source, ok := spec["source"].(map[string]interface{})
			if !ok {
				t.Errorf("Failed to get snapshot source")
				return
			}
			if source["name"] != tt.vmName {
				t.Errorf("Snapshot source VM name = %q, want %q", source["name"], tt.vmName)
			}
			if source["kind"] != "VirtualMachine" {
				t.Errorf("Snapshot source kind = %q, want %q", source["kind"], "VirtualMachine")
			}
		})
	}
}

func TestRestoreVMSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		vmName        string
		snapshotName  string
		restoreName   string
		wantError     bool
		errorContains string
	}{
		{
			name:         "Restore snapshot successfully",
			namespace:    "default",
			vmName:       "test-vm",
			snapshotName: "test-snapshot",
			restoreName:  "test-restore",
			wantError:    false,
		},
		{
			name:         "Restore with different names",
			namespace:    "test-namespace",
			vmName:       "my-vm",
			snapshotName: "my-snapshot",
			restoreName:  "my-restore",
			wantError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			client := fake.NewSimpleDynamicClient(scheme)
			ctx := context.Background()

			restore, err := RestoreVMSnapshot(ctx, client, tt.namespace, tt.vmName, tt.snapshotName, tt.restoreName)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if restore == nil {
				t.Errorf("Expected non-nil restore, got nil")
				return
			}

			// Verify restore metadata
			if restore.GetName() != tt.restoreName {
				t.Errorf("Restore name = %q, want %q", restore.GetName(), tt.restoreName)
			}
			if restore.GetNamespace() != tt.namespace {
				t.Errorf("Restore namespace = %q, want %q", restore.GetNamespace(), tt.namespace)
			}

			// Verify restore spec
			spec, ok := restore.Object["spec"].(map[string]interface{})
			if !ok {
				t.Errorf("Failed to get restore spec")
				return
			}
			target, ok := spec["target"].(map[string]interface{})
			if !ok {
				t.Errorf("Failed to get restore target")
				return
			}
			if target["name"] != tt.vmName {
				t.Errorf("Restore target VM name = %q, want %q", target["name"], tt.vmName)
			}
			if spec["virtualMachineSnapshotName"] != tt.snapshotName {
				t.Errorf("Restore snapshot name = %q, want %q", spec["virtualMachineSnapshotName"], tt.snapshotName)
			}
		})
	}
}

func TestListVMSnapshots(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		existingSnaps []*unstructured.Unstructured
		wantCount     int
		wantError     bool
		errorContains string
	}{
		{
			name:      "List snapshots when none exist",
			namespace: "default",
			wantCount: 0,
			wantError: false,
		},
		{
			name:      "List snapshots with one snapshot",
			namespace: "default",
			existingSnaps: []*unstructured.Unstructured{
				createTestSnapshot("snap1", "default", "vm1"),
			},
			wantCount: 1,
			wantError: false,
		},
		{
			name:      "List snapshots with multiple snapshots",
			namespace: "default",
			existingSnaps: []*unstructured.Unstructured{
				createTestSnapshot("snap1", "default", "vm1"),
				createTestSnapshot("snap2", "default", "vm2"),
				createTestSnapshot("snap3", "default", "vm3"),
			},
			wantCount: 3,
			wantError: false,
		},
		{
			name:      "List snapshots in different namespace",
			namespace: "test-namespace",
			existingSnaps: []*unstructured.Unstructured{
				createTestSnapshot("snap1", "test-namespace", "vm1"),
				createTestSnapshot("snap2", "default", "vm2"), // Different namespace
			},
			wantCount: 1,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()

			// Convert to []runtime.Object for fake client
			objects := make([]runtime.Object, len(tt.existingSnaps))
			for i, snap := range tt.existingSnaps {
				objects[i] = snap
			}

			// Register the list kind for VirtualMachineSnapshot
			gvrToListKind := map[schema.GroupVersionResource]string{
				VirtualMachineSnapshotGVR: "VirtualMachineSnapshotList",
			}
			client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objects...)
			ctx := context.Background()

			snapshots, err := ListVMSnapshots(ctx, client, tt.namespace)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(snapshots) != tt.wantCount {
				t.Errorf("Snapshot count = %d, want %d", len(snapshots), tt.wantCount)
			}
		})
	}
}

func TestGetVMSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		snapshotName  string
		existingSnap  *unstructured.Unstructured
		wantError     bool
		errorContains string
	}{
		{
			name:         "Get existing snapshot",
			namespace:    "default",
			snapshotName: "test-snapshot",
			existingSnap: createTestSnapshot("test-snapshot", "default", "test-vm"),
			wantError:    false,
		},
		{
			name:          "Get non-existent snapshot",
			namespace:     "default",
			snapshotName:  "non-existent",
			existingSnap:  nil,
			wantError:     true,
			errorContains: "failed to get VirtualMachineSnapshot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			var client *fake.FakeDynamicClient
			if tt.existingSnap != nil {
				client = fake.NewSimpleDynamicClient(scheme, tt.existingSnap)
			} else {
				client = fake.NewSimpleDynamicClient(scheme)
			}
			ctx := context.Background()

			snapshot, err := GetVMSnapshot(ctx, client, tt.namespace, tt.snapshotName)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if snapshot == nil {
				t.Errorf("Expected non-nil snapshot, got nil")
				return
			}

			if snapshot.GetName() != tt.snapshotName {
				t.Errorf("Snapshot name = %q, want %q", snapshot.GetName(), tt.snapshotName)
			}
			if snapshot.GetNamespace() != tt.namespace {
				t.Errorf("Snapshot namespace = %q, want %q", snapshot.GetNamespace(), tt.namespace)
			}
		})
	}
}

func TestEnableSnapshotFeatureGate(t *testing.T) {
	tests := []struct {
		name             string
		namespace        string
		kvName           string
		initialGates     []string
		wantGatesEnabled bool
		wantError        bool
		errorContains    string
	}{
		{
			name:             "Enable Snapshot feature gate when none exist",
			namespace:        "kubevirt",
			kvName:           "kubevirt",
			initialGates:     []string{},
			wantGatesEnabled: true,
			wantError:        false,
		},
		{
			name:             "Enable Snapshot feature gate when other gates exist",
			namespace:        "kubevirt",
			kvName:           "kubevirt",
			initialGates:     []string{"FeatureGate1", "FeatureGate2"},
			wantGatesEnabled: true,
			wantError:        false,
		},
		{
			name:             "Enable Snapshot feature gate when already enabled (idempotent)",
			namespace:        "kubevirt",
			kvName:           "kubevirt",
			initialGates:     []string{"Snapshot", "OtherFeature"},
			wantGatesEnabled: true,
			wantError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			kv := createTestKubeVirt(tt.kvName, tt.namespace, tt.initialGates)
			client := fake.NewSimpleDynamicClient(scheme, kv)
			ctx := context.Background()

			result, err := EnableSnapshotFeatureGate(ctx, client, tt.namespace, tt.kvName)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("Expected non-nil result, got nil")
				return
			}

			// Check if Snapshot feature gate is enabled
			gates, found, err := unstructured.NestedStringSlice(result.Object, "spec", "configuration", "developerConfiguration", "featureGates")
			if err != nil {
				t.Errorf("Failed to read feature gates: %v", err)
				return
			}

			if !found && tt.wantGatesEnabled {
				t.Errorf("Feature gates not found in result")
				return
			}

			snapshotEnabled := false
			for _, gate := range gates {
				if gate == "Snapshot" {
					snapshotEnabled = true
					break
				}
			}

			if snapshotEnabled != tt.wantGatesEnabled {
				t.Errorf("Snapshot feature gate enabled = %v, want %v", snapshotEnabled, tt.wantGatesEnabled)
			}
		})
	}
}

func TestDisableSnapshotFeatureGate(t *testing.T) {
	tests := []struct {
		name              string
		namespace         string
		kvName            string
		initialGates      []string
		wantGatesDisabled bool
		wantError         bool
		errorContains     string
	}{
		{
			name:              "Disable Snapshot feature gate when enabled",
			namespace:         "kubevirt",
			kvName:            "kubevirt",
			initialGates:      []string{"Snapshot", "OtherFeature"},
			wantGatesDisabled: true,
			wantError:         false,
		},
		{
			name:              "Disable Snapshot feature gate when not enabled (idempotent)",
			namespace:         "kubevirt",
			kvName:            "kubevirt",
			initialGates:      []string{"OtherFeature"},
			wantGatesDisabled: true,
			wantError:         false,
		},
		{
			name:              "Disable Snapshot feature gate when no gates exist",
			namespace:         "kubevirt",
			kvName:            "kubevirt",
			initialGates:      []string{},
			wantGatesDisabled: true,
			wantError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			kv := createTestKubeVirt(tt.kvName, tt.namespace, tt.initialGates)
			client := fake.NewSimpleDynamicClient(scheme, kv)
			ctx := context.Background()

			result, err := DisableSnapshotFeatureGate(ctx, client, tt.namespace, tt.kvName)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("Expected non-nil result, got nil")
				return
			}

			// Check if Snapshot feature gate is disabled
			gates, _, err := unstructured.NestedStringSlice(result.Object, "spec", "configuration", "developerConfiguration", "featureGates")
			if err != nil {
				t.Errorf("Failed to read feature gates: %v", err)
				return
			}

			snapshotEnabled := false
			for _, gate := range gates {
				if gate == "Snapshot" {
					snapshotEnabled = true
					break
				}
			}

			if snapshotEnabled {
				t.Errorf("Snapshot feature gate should be disabled but was enabled")
			}
		})
	}
}

func TestEnableSnapshotFeatureGateNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewSimpleDynamicClient(scheme)
	ctx := context.Background()

	_, err := EnableSnapshotFeatureGate(ctx, client, "kubevirt", "non-existent")
	if err == nil {
		t.Errorf("Expected error for non-existent KubeVirt CR, got nil")
		return
	}
	if !strings.Contains(err.Error(), "failed to get KubeVirt CR") {
		t.Errorf("Error = %v, want to contain 'failed to get KubeVirt CR'", err)
	}
}

func TestDisableSnapshotFeatureGateNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewSimpleDynamicClient(scheme)
	ctx := context.Background()

	_, err := DisableSnapshotFeatureGate(ctx, client, "kubevirt", "non-existent")
	if err == nil {
		t.Errorf("Expected error for non-existent KubeVirt CR, got nil")
		return
	}
	if !strings.Contains(err.Error(), "failed to get KubeVirt CR") {
		t.Errorf("Error = %v, want to contain 'failed to get KubeVirt CR'", err)
	}
}
