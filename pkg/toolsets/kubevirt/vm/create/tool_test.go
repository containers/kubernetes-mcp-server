package create

import (
	"context"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type mockToolCallRequest struct {
	arguments map[string]interface{}
}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return m.arguments
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]interface{}
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "creates VM with basic settings",
			args: map[string]interface{}{
				"namespace": "test-ns",
				"name":      "test-vm",
				"workload":  "fedora",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "VirtualMachine Creation Plan") {
					t.Errorf("Expected 'VirtualMachine Creation Plan' header in result")
				}
				if !strings.Contains(result, "name: test-vm") {
					t.Errorf("Expected VM name test-vm in YAML")
				}
				if !strings.Contains(result, "namespace: test-ns") {
					t.Errorf("Expected namespace test-ns in YAML")
				}
				if !strings.Contains(result, "quay.io/containerdisks/fedora:latest") {
					t.Errorf("Expected fedora container disk in result")
				}
				if !strings.Contains(result, "guest: 2Gi") {
					t.Errorf("Expected guest: 2Gi in YAML manifest")
				}
			},
		},
		{
			name: "creates VM with instancetype",
			args: map[string]interface{}{
				"namespace":    "test-ns",
				"name":         "test-vm",
				"workload":     "ubuntu",
				"instancetype": "u1.medium",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "name: u1.medium") {
					t.Errorf("Expected instance type in YAML manifest")
				}
				if !strings.Contains(result, "kind: VirtualMachineClusterInstancetype") {
					t.Errorf("Expected VirtualMachineClusterInstancetype in YAML manifest")
				}
				// When instancetype is set, memory should not be in the YAML resources section
				if strings.Contains(result, "resources:\n          requests:\n            memory:") {
					t.Errorf("Should not have memory resources when instancetype is specified")
				}
			},
		},
		{
			name: "creates VM with preference",
			args: map[string]interface{}{
				"namespace":  "test-ns",
				"name":       "test-vm",
				"workload":   "rhel",
				"preference": "rhel.9",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "name: rhel.9") {
					t.Errorf("Expected preference in YAML manifest")
				}
				if !strings.Contains(result, "kind: VirtualMachineClusterPreference") {
					t.Errorf("Expected VirtualMachineClusterPreference in YAML manifest")
				}
			},
		},
		{
			name: "creates VM with custom container disk",
			args: map[string]interface{}{
				"namespace": "test-ns",
				"name":      "test-vm",
				"workload":  "quay.io/myrepo/myimage:v1.0",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "quay.io/myrepo/myimage:v1.0") {
					t.Errorf("Expected custom container disk in YAML")
				}
			},
		},
		{
			name: "missing namespace",
			args: map[string]interface{}{
				"name":     "test-vm",
				"workload": "fedora",
			},
			wantErr: true,
		},
		{
			name: "missing name",
			args: map[string]interface{}{
				"namespace": "test-ns",
				"workload":  "fedora",
			},
			wantErr: true,
		},
		{
			name: "missing workload defaults to fedora",
			args: map[string]interface{}{
				"namespace": "test-ns",
				"name":      "test-vm",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "quay.io/containerdisks/fedora:latest") {
					t.Errorf("Expected default fedora container disk in result")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := api.ToolHandlerParams{
				Context:         context.Background(),
				Kubernetes:      &internalk8s.Kubernetes{},
				ToolCallRequest: &mockToolCallRequest{arguments: tt.args},
			}

			result, err := create(params)
			if err != nil {
				t.Errorf("create() unexpected Go error: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected non-nil result")
				return
			}

			if tt.wantErr {
				if result.Error == nil {
					t.Error("Expected error in result.Error, got nil")
				}
			} else {
				if result.Error != nil {
					t.Errorf("Expected no error in result, got: %v", result.Error)
				}
				if result.Content == "" {
					t.Error("Expected non-empty result content")
				}
				if tt.checkFunc != nil {
					tt.checkFunc(t, result.Content)
				}
			}
		})
	}
}

func TestResolveContainerDisk(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"fedora", "fedora", "quay.io/containerdisks/fedora:latest"},
		{"ubuntu", "ubuntu", "quay.io/containerdisks/ubuntu:24.04"},
		{"rhel8", "rhel8", "registry.redhat.io/rhel8/rhel-guest-image:latest"},
		{"rhel9", "rhel9", "registry.redhat.io/rhel9/rhel-guest-image:latest"},
		{"rhel10", "rhel10", "registry.redhat.io/rhel10/rhel-guest-image:latest"},
		{"centos", "centos", "quay.io/containerdisks/centos-stream:9-latest"},
		{"centos-stream", "centos-stream", "quay.io/containerdisks/centos-stream:9-latest"},
		{"debian", "debian", "quay.io/containerdisks/debian:latest"},
		{"case insensitive", "FEDORA", "quay.io/containerdisks/fedora:latest"},
		{"with whitespace", " ubuntu ", "quay.io/containerdisks/ubuntu:24.04"},
		{"custom image", "quay.io/myrepo/myimage:v1", "quay.io/myrepo/myimage:v1"},
		{"with tag", "myimage:latest", "myimage:latest"},
		{"unknown OS", "customos", "customos"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveContainerDisk(tt.input)
			if result != tt.expected {
				t.Errorf("resolveContainerDisk(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}
