package create_test

import (
	"context"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	k8stesting "github.com/containers/kubernetes-mcp-server/pkg/kubernetes/testing"
	kubevirttesting "github.com/containers/kubernetes-mcp-server/pkg/kubevirt/testing"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/create"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ============================================================================
// Test Infrastructure
// ============================================================================

type mockToolCallRequest struct {
	arguments map[string]any
}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return m.arguments
}

// newTestKubernetesClient creates a test Kubernetes client with fake dynamic client
// populated with the specified objects and GVR to ListKind mappings
func newTestKubernetesClient(
	gvrToListKind map[schema.GroupVersionResource]string,
	objects ...runtime.Object,
) *internalk8s.Kubernetes {
	return k8stesting.NewFakeKubernetesClient(runtime.NewScheme(), gvrToListKind, objects...)
}

// getVMCreateHandler retrieves the vm_create handler from the Tools() function
func getVMCreateHandler(t *testing.T) api.ToolHandlerFunc {
	tools := create.Tools()
	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}
	if tools[0].Tool.Name != "vm_create" {
		t.Fatalf("Expected tool name 'vm_create', got '%s'", tools[0].Tool.Name)
	}
	return tools[0].Handler
}

// ============================================================================
// Tests for vm_create Tool Handler (via Tools() public API)
// ============================================================================

func TestCreate(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "creates VM with basic settings",
			args: map[string]any{
				"namespace": "test-ns",
				"name":      "test-vm",
				"workload":  "fedora",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "VirtualMachine created successfully") {
					t.Errorf("Expected 'VirtualMachine created successfully' header in result")
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
				if !strings.Contains(result, "runStrategy: Halted") {
					t.Errorf("Expected runStrategy: Halted in YAML manifest (default when autostart is false)")
				}
			},
		},
		{
			name: "creates VM with instancetype",
			args: map[string]any{
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
			},
		},
		{
			name: "creates VM with preference",
			args: map[string]any{
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
			args: map[string]any{
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
			args: map[string]any{
				"name":     "test-vm",
				"workload": "fedora",
			},
			wantErr: true,
		},
		{
			name: "missing name",
			args: map[string]any{
				"namespace": "test-ns",
				"workload":  "fedora",
			},
			wantErr: true,
		},
		{
			name: "missing workload defaults to fedora",
			args: map[string]any{
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
			gvrToListKind := map[schema.GroupVersionResource]string{
				{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "datasources"}:                                 "DataSourceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterpreferences"}:   "VirtualMachineClusterPreferenceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinepreferences"}:          "VirtualMachinePreferenceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterinstancetypes"}: "VirtualMachineClusterInstancetypeList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineinstancetypes"}:        "VirtualMachineInstancetypeList",
			}

			params := api.ToolHandlerParams{
				Context:         context.Background(),
				Kubernetes:      newTestKubernetesClient(gvrToListKind),
				ToolCallRequest: &mockToolCallRequest{arguments: tt.args},
			}

			handler := getVMCreateHandler(t)
			result, err := handler(params)
			if err != nil {
				t.Errorf("handler() unexpected Go error: %v", err)
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

func TestCreateWithSize(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		objects   []runtime.Object
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "creates VM with size hint that matches instancetype",
			args: map[string]any{
				"namespace": "test-ns",
				"name":      "test-vm",
				"workload":  "fedora",
				"size":      "medium",
			},
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredInstancetype("u1.medium", map[string]string{}),
				kubevirttesting.NewUnstructuredInstancetype("u1.small", map[string]string{}),
				kubevirttesting.NewUnstructuredInstancetype("u1.large", map[string]string{}),
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "name: u1.medium") {
					t.Errorf("Expected instancetype u1.medium to be selected, result: %s", result)
				}
			},
		},
		{
			name: "creates VM with size and performance hints",
			args: map[string]any{
				"namespace":   "test-ns",
				"name":        "test-vm",
				"workload":    "fedora",
				"size":        "large",
				"performance": "compute-optimized",
			},
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredInstancetype("c1.large", map[string]string{"instancetype.kubevirt.io/class": "compute"}),
				kubevirttesting.NewUnstructuredInstancetype("u1.large", map[string]string{"instancetype.kubevirt.io/class": "general"}),
				kubevirttesting.NewUnstructuredInstancetype("m1.large", map[string]string{"instancetype.kubevirt.io/class": "memory"}),
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "name: c1.large") {
					t.Errorf("Expected compute instancetype c1.large to be selected, result: %s", result)
				}
			},
		},
		{
			name: "creates VM with size but no matching instancetype",
			args: map[string]any{
				"namespace": "test-ns",
				"name":      "test-vm",
				"workload":  "fedora",
				"size":      "xlarge",
			},
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredInstancetype("u1.small", map[string]string{}),
				kubevirttesting.NewUnstructuredInstancetype("u1.medium", map[string]string{}),
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if strings.Contains(result, "instancetype:") {
					t.Errorf("Should not have instancetype when no match found")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvrToListKind := map[schema.GroupVersionResource]string{
				{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "datasources"}:                                 "DataSourceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterpreferences"}:   "VirtualMachineClusterPreferenceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinepreferences"}:          "VirtualMachinePreferenceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterinstancetypes"}: "VirtualMachineClusterInstancetypeList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineinstancetypes"}:        "VirtualMachineInstancetypeList",
			}

			params := api.ToolHandlerParams{
				Context:         context.Background(),
				Kubernetes:      newTestKubernetesClient(gvrToListKind, tt.objects...),
				ToolCallRequest: &mockToolCallRequest{arguments: tt.args},
			}

			handler := getVMCreateHandler(t)
			result, err := handler(params)
			if err != nil {
				t.Errorf("handler() unexpected Go error: %v", err)
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

func TestCreateWithDataSources(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		objects   []runtime.Object
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "creates VM using DataSource with default instancetype and preference",
			args: map[string]any{
				"namespace": "test-ns",
				"name":      "test-vm",
				"workload":  "fedora",
			},
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredDataSource("fedora", "openshift-virtualization-os-images", "registry.redhat.io/fedora:latest", "u1.medium", "fedora"),
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "sourceRef:") {
					t.Errorf("Expected sourceRef in result")
				}
				if !strings.Contains(result, "dataVolumeTemplates:") {
					t.Errorf("Expected dataVolumeTemplates when using DataSource")
				}
				lines := strings.Split(result, "\n")
				foundSourceRef := false
				foundDataSourceName := false
				for i, line := range lines {
					if strings.Contains(line, "sourceRef:") {
						foundSourceRef = true
						for j := i; j < i+5 && j < len(lines); j++ {
							if strings.Contains(lines[j], "name: fedora") {
								foundDataSourceName = true
								break
							}
						}
					}
				}
				if !foundSourceRef {
					t.Errorf("Expected sourceRef in result")
				}
				if !foundDataSourceName {
					t.Errorf("Expected DataSource name 'fedora' in sourceRef section")
				}
				if !strings.Contains(result, "name: u1.medium") {
					t.Errorf("Expected default instancetype u1.medium from DataSource")
				}
			},
		},
		{
			name: "creates VM using DataSource partial name match",
			args: map[string]any{
				"namespace": "test-ns",
				"name":      "test-vm",
				"workload":  "rhel",
			},
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredDataSource("rhel9", "openshift-virtualization-os-images", "registry.redhat.io/rhel9:latest", "", "rhel.9"),
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "name: rhel9") {
					t.Errorf("Expected DataSource name 'rhel9' to match 'rhel' input")
				}
			},
		},
		{
			name: "explicit size overrides DataSource default instancetype",
			args: map[string]any{
				"namespace": "test-ns",
				"name":      "test-vm",
				"workload":  "fedora",
				"size":      "large",
			},
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredDataSource("fedora", "openshift-virtualization-os-images", "registry.redhat.io/fedora:latest", "u1.medium", "fedora"),
				kubevirttesting.NewUnstructuredInstancetype("u1.large", map[string]string{}),
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "name: u1.large") {
					t.Errorf("Expected explicit size to override DataSource default, got result without u1.large")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvrToListKind := map[schema.GroupVersionResource]string{
				{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "datasources"}:                                 "DataSourceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterpreferences"}:   "VirtualMachineClusterPreferenceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinepreferences"}:          "VirtualMachinePreferenceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterinstancetypes"}: "VirtualMachineClusterInstancetypeList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineinstancetypes"}:        "VirtualMachineInstancetypeList",
			}

			params := api.ToolHandlerParams{
				Context:         context.Background(),
				Kubernetes:      newTestKubernetesClient(gvrToListKind, tt.objects...),
				ToolCallRequest: &mockToolCallRequest{arguments: tt.args},
			}

			handler := getVMCreateHandler(t)
			result, err := handler(params)
			if err != nil {
				t.Errorf("handler() unexpected Go error: %v", err)
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

func TestCreateWithPreferences(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		objects   []runtime.Object
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "auto-selects preference matching workload name",
			args: map[string]any{
				"namespace": "test-ns",
				"name":      "test-vm",
				"workload":  "rhel",
			},
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredPreference("rhel.9", false),
				kubevirttesting.NewUnstructuredPreference("fedora", false),
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "name: rhel.9") {
					t.Errorf("Expected preference 'rhel.9' to be auto-selected for workload 'rhel'")
				}
			},
		},
		{
			name: "explicit preference overrides auto-selection",
			args: map[string]any{
				"namespace":  "test-ns",
				"name":       "test-vm",
				"workload":   "fedora",
				"preference": "custom.preference",
			},
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredPreference("fedora", false),
				kubevirttesting.NewUnstructuredPreference("custom.preference", false),
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "name: custom.preference") {
					t.Errorf("Expected explicit preference 'custom.preference' to be used")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvrToListKind := map[schema.GroupVersionResource]string{
				{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "datasources"}:                                 "DataSourceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterpreferences"}:   "VirtualMachineClusterPreferenceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinepreferences"}:          "VirtualMachinePreferenceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterinstancetypes"}: "VirtualMachineClusterInstancetypeList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineinstancetypes"}:        "VirtualMachineInstancetypeList",
			}

			params := api.ToolHandlerParams{
				Context:         context.Background(),
				Kubernetes:      newTestKubernetesClient(gvrToListKind, tt.objects...),
				ToolCallRequest: &mockToolCallRequest{arguments: tt.args},
			}

			handler := getVMCreateHandler(t)
			result, err := handler(params)
			if err != nil {
				t.Errorf("handler() unexpected Go error: %v", err)
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

// ============================================================================
// Tests for Tools() - Public API
// ============================================================================

func TestTools(t *testing.T) {
	tools := create.Tools()

	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
		return
	}

	tool := tools[0]

	t.Run("tool has correct name", func(t *testing.T) {
		if tool.Tool.Name != "vm_create" {
			t.Errorf("Expected tool name 'vm_create', got '%s'", tool.Tool.Name)
		}
	})

	t.Run("tool has description", func(t *testing.T) {
		if tool.Tool.Description == "" {
			t.Error("Expected non-empty description")
		}
	})

	t.Run("tool has input schema", func(t *testing.T) {
		if tool.Tool.InputSchema == nil {
			t.Fatal("Expected InputSchema to be non-nil")
		}
		if tool.Tool.InputSchema.Type != "object" {
			t.Errorf("Expected InputSchema type 'object', got '%s'", tool.Tool.InputSchema.Type)
		}
	})

	t.Run("tool has required parameters", func(t *testing.T) {
		if tool.Tool.InputSchema == nil {
			t.Fatal("Expected InputSchema to be non-nil")
		}
		required := tool.Tool.InputSchema.Required
		if len(required) != 2 {
			t.Errorf("Expected 2 required parameters, got %d", len(required))
		}
		expectedRequired := map[string]bool{"namespace": true, "name": true}
		for _, req := range required {
			if !expectedRequired[req] {
				t.Errorf("Unexpected required parameter: %s", req)
			}
		}
	})

	t.Run("tool has handler function", func(t *testing.T) {
		if tool.Handler == nil {
			t.Error("Expected Handler to be non-nil")
		}
	})

	t.Run("tool has correct annotations", func(t *testing.T) {
		if tool.Tool.Annotations.Title == "" {
			t.Error("Expected non-empty Title annotation")
		}
		if tool.Tool.Annotations.ReadOnlyHint == nil || *tool.Tool.Annotations.ReadOnlyHint != false {
			t.Error("Expected ReadOnlyHint to be false")
		}
		if tool.Tool.Annotations.DestructiveHint == nil || *tool.Tool.Annotations.DestructiveHint != true {
			t.Error("Expected DestructiveHint to be true")
		}
		if tool.Tool.Annotations.IdempotentHint == nil || *tool.Tool.Annotations.IdempotentHint != true {
			t.Error("Expected IdempotentHint to be true")
		}
	})
}
