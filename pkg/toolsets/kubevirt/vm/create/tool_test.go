package create

import (
	"strings"
	"testing"
)

// Test the YAML rendering directly without creating resources
func TestRenderVMYaml(t *testing.T) {
	tests := []struct {
		name      string
		params    vmParams
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "renders VM with basic settings",
			params: vmParams{
				Namespace:     "test-ns",
				Name:          "test-vm",
				ContainerDisk: "quay.io/containerdisks/fedora:latest",
				RunStrategy:   "Halted",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "apiVersion: kubevirt.io/v1") {
					t.Errorf("Expected apiVersion in YAML")
				}
				if !strings.Contains(result, "kind: VirtualMachine") {
					t.Errorf("Expected kind VirtualMachine in YAML")
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
			name: "renders VM with instancetype",
			params: vmParams{
				Namespace:     "test-ns",
				Name:          "test-vm",
				ContainerDisk: "quay.io/containerdisks/ubuntu:24.04",
				Instancetype:  "u1.medium",
				RunStrategy:   "Halted",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "name: u1.medium") {
					t.Errorf("Expected instance type in YAML manifest")
				}
				if !strings.Contains(result, "kind: VirtualMachineClusterInstancetype") {
					t.Errorf("Expected VirtualMachineClusterInstancetype in YAML manifest")
				}
				// When instancetype is set, memory should not be in the YAML
				if strings.Contains(result, "guest: 2Gi") {
					t.Errorf("Should not have guest memory when instancetype is specified")
				}
			},
		},
		{
			name: "renders VM with preference",
			params: vmParams{
				Namespace:     "test-ns",
				Name:          "test-vm",
				ContainerDisk: "registry.redhat.io/rhel9/rhel-guest-image:latest",
				Preference:    "rhel.9",
				RunStrategy:   "Halted",
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
			name: "renders VM with custom container disk",
			params: vmParams{
				Namespace:     "test-ns",
				Name:          "test-vm",
				ContainerDisk: "quay.io/myrepo/myimage:v1.0",
				RunStrategy:   "Halted",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "quay.io/myrepo/myimage:v1.0") {
					t.Errorf("Expected custom container disk in YAML")
				}
			},
		},
		{
			name: "renders VM with DataSource",
			params: vmParams{
				Namespace:           "test-ns",
				Name:                "test-vm",
				UseDataSource:       true,
				DataSourceName:      "fedora",
				DataSourceNamespace: "openshift-virtualization-os-images",
				RunStrategy:         "Halted",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "dataVolumeTemplates") {
					t.Errorf("Expected dataVolumeTemplates in YAML")
				}
				if !strings.Contains(result, "kind: DataSource") {
					t.Errorf("Expected DataSource kind in YAML")
				}
				if !strings.Contains(result, "name: fedora") {
					t.Errorf("Expected DataSource name in YAML")
				}
				if !strings.Contains(result, "openshift-virtualization-os-images") {
					t.Errorf("Expected DataSource namespace in YAML")
				}
			},
		},
		{
			name: "renders VM with autostart (runStrategy Always)",
			params: vmParams{
				Namespace:     "test-ns",
				Name:          "test-vm",
				ContainerDisk: "quay.io/containerdisks/fedora:latest",
				RunStrategy:   "Always",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "runStrategy: Always") {
					t.Errorf("Expected runStrategy: Always in YAML")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderVMYaml(tt.params)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if result == "" {
					t.Error("Expected non-empty result")
				}
				if tt.checkFunc != nil {
					tt.checkFunc(t, result)
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
