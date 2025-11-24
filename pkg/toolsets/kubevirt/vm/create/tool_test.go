package create

import (
	"context"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	k8stesting "github.com/containers/kubernetes-mcp-server/pkg/kubernetes/testing"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	kubevirttesting "github.com/containers/kubernetes-mcp-server/pkg/kubevirt/testing"
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

// ============================================================================
// Tests for create() - Main Tool Handler
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

// ============================================================================
// Tests for parseCreateParameters()
// ============================================================================

func TestParseCreateParameters(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		wantErr   bool
		checkFunc func(t *testing.T, params *createParameters)
	}{
		{
			name: "parses basic parameters",
			args: map[string]any{
				"namespace": "test-ns",
				"name":      "test-vm",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, params *createParameters) {
				if params.Namespace != "test-ns" {
					t.Errorf("Namespace = %q, want 'test-ns'", params.Namespace)
				}
				if params.Name != "test-vm" {
					t.Errorf("Name = %q, want 'test-vm'", params.Name)
				}
				if params.Workload != "fedora" {
					t.Errorf("Workload = %q, want 'fedora' (default)", params.Workload)
				}
				if params.Autostart != false {
					t.Errorf("Autostart = %v, want false (default)", params.Autostart)
				}
			},
		},
		{
			name: "parses autostart parameter",
			args: map[string]any{
				"namespace": "test-ns",
				"name":      "test-vm",
				"autostart": true,
			},
			wantErr: false,
			checkFunc: func(t *testing.T, params *createParameters) {
				if params.Autostart != true {
					t.Errorf("Autostart = %v, want true", params.Autostart)
				}
			},
		},
		{
			name: "normalizes performance parameter",
			args: map[string]any{
				"namespace":   "test-ns",
				"name":        "test-vm",
				"performance": "compute-optimized",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, params *createParameters) {
				if params.Performance != "c1" {
					t.Errorf("Performance = %q, want 'c1'", params.Performance)
				}
			},
		},
		{
			name: "missing namespace returns error",
			args: map[string]any{
				"name": "test-vm",
			},
			wantErr: true,
		},
		{
			name: "missing name returns error",
			args: map[string]any{
				"namespace": "test-ns",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := api.ToolHandlerParams{
				ToolCallRequest: &mockToolCallRequest{arguments: tt.args},
			}

			result, err := parseCreateParameters(params)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCreateParameters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

// ============================================================================
// Tests for buildTemplateParams()
// ============================================================================

func TestBuildTemplateParams(t *testing.T) {
	tests := []struct {
		name                  string
		createParams          *createParameters
		matchedDataSource     *kubevirt.DataSourceInfo
		instancetype          *kubevirt.InstancetypeInfo
		preference            *kubevirt.PreferenceInfo
		expectedRunStrategy   string
		expectedUseDataSource bool
		expectedContainerDisk string
	}{
		{
			name: "basic VM with autostart false",
			createParams: &createParameters{
				Namespace: "test-ns",
				Name:      "test-vm",
				Workload:  "fedora",
				Autostart: false,
			},
			matchedDataSource:     nil,
			instancetype:          nil,
			preference:            nil,
			expectedRunStrategy:   "Halted",
			expectedUseDataSource: false,
			expectedContainerDisk: "quay.io/containerdisks/fedora:latest",
		},
		{
			name: "VM with autostart true",
			createParams: &createParameters{
				Namespace: "test-ns",
				Name:      "test-vm",
				Workload:  "fedora",
				Autostart: true,
			},
			matchedDataSource:     nil,
			instancetype:          nil,
			preference:            nil,
			expectedRunStrategy:   "Always",
			expectedUseDataSource: false,
			expectedContainerDisk: "quay.io/containerdisks/fedora:latest",
		},
		{
			name: "VM with DataSource",
			createParams: &createParameters{
				Namespace: "test-ns",
				Name:      "test-vm",
				Workload:  "fedora",
				Autostart: false,
			},
			matchedDataSource: &kubevirt.DataSourceInfo{
				Name:      "fedora",
				Namespace: "os-images",
				Source:    "registry.example.com/fedora:latest",
			},
			instancetype:          nil,
			preference:            nil,
			expectedRunStrategy:   "Halted",
			expectedUseDataSource: true,
			expectedContainerDisk: "",
		},
		{
			name: "VM with built-in containerdisk (no namespace)",
			createParams: &createParameters{
				Namespace: "test-ns",
				Name:      "test-vm",
				Workload:  "fedora",
				Autostart: false,
			},
			matchedDataSource: &kubevirt.DataSourceInfo{
				Name:      "fedora",
				Namespace: "",
				Source:    "quay.io/containerdisks/fedora:latest",
			},
			instancetype:          nil,
			preference:            nil,
			expectedRunStrategy:   "Halted",
			expectedUseDataSource: false,
			expectedContainerDisk: "quay.io/containerdisks/fedora:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTemplateParams(tt.createParams, tt.matchedDataSource, tt.instancetype, tt.preference)

			if result.RunStrategy != tt.expectedRunStrategy {
				t.Errorf("RunStrategy = %q, want %q", result.RunStrategy, tt.expectedRunStrategy)
			}
			if result.UseDataSource != tt.expectedUseDataSource {
				t.Errorf("UseDataSource = %v, want %v", result.UseDataSource, tt.expectedUseDataSource)
			}
			if result.ContainerDisk != tt.expectedContainerDisk {
				t.Errorf("ContainerDisk = %q, want %q", result.ContainerDisk, tt.expectedContainerDisk)
			}
			if result.Namespace != tt.createParams.Namespace {
				t.Errorf("Namespace = %q, want %q", result.Namespace, tt.createParams.Namespace)
			}
			if result.Name != tt.createParams.Name {
				t.Errorf("Name = %q, want %q", result.Name, tt.createParams.Name)
			}
		})
	}
}

// ============================================================================
// Tests for renderVMYaml()
// ============================================================================

func TestRenderVMYaml(t *testing.T) {
	tests := []struct {
		name      string
		params    vmParams
		wantErr   bool
		checkFunc func(t *testing.T, yaml string)
	}{
		{
			name: "renders VM with Halted runStrategy",
			params: vmParams{
				Namespace:     "test-ns",
				Name:          "test-vm",
				ContainerDisk: "quay.io/containerdisks/fedora:latest",
				RunStrategy:   "Halted",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, yaml string) {
				if !strings.Contains(yaml, "runStrategy: Halted") {
					t.Error("Expected runStrategy: Halted in rendered YAML")
				}
				if !strings.Contains(yaml, "name: test-vm") {
					t.Error("Expected name: test-vm in rendered YAML")
				}
				if !strings.Contains(yaml, "namespace: test-ns") {
					t.Error("Expected namespace: test-ns in rendered YAML")
				}
			},
		},
		{
			name: "renders VM with Always runStrategy",
			params: vmParams{
				Namespace:     "test-ns",
				Name:          "test-vm",
				ContainerDisk: "quay.io/containerdisks/fedora:latest",
				RunStrategy:   "Always",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, yaml string) {
				if !strings.Contains(yaml, "runStrategy: Always") {
					t.Error("Expected runStrategy: Always in rendered YAML")
				}
			},
		},
		{
			name: "renders VM with instancetype",
			params: vmParams{
				Namespace:        "test-ns",
				Name:             "test-vm",
				ContainerDisk:    "quay.io/containerdisks/fedora:latest",
				Instancetype:     "u1.medium",
				InstancetypeKind: "VirtualMachineClusterInstancetype",
				RunStrategy:      "Halted",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, yaml string) {
				if !strings.Contains(yaml, "instancetype:") {
					t.Error("Expected instancetype section in rendered YAML")
				}
				if !strings.Contains(yaml, "name: u1.medium") {
					t.Error("Expected name: u1.medium in instancetype section")
				}
				if !strings.Contains(yaml, "kind: VirtualMachineClusterInstancetype") {
					t.Error("Expected kind: VirtualMachineClusterInstancetype in instancetype section")
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
				DataSourceNamespace: "os-images",
				RunStrategy:         "Halted",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, yaml string) {
				if !strings.Contains(yaml, "dataVolumeTemplates:") {
					t.Error("Expected dataVolumeTemplates in rendered YAML")
				}
				if !strings.Contains(yaml, "sourceRef:") {
					t.Error("Expected sourceRef in rendered YAML")
				}
				if !strings.Contains(yaml, "name: fedora") {
					t.Error("Expected DataSource name in rendered YAML")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml, err := renderVMYaml(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("renderVMYaml() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, yaml)
			}
		})
	}
}

// ============================================================================
// Tests for resolveContainerDisk()
// ============================================================================

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
		{"opensuse", "opensuse", "quay.io/containerdisks/opensuse-tumbleweed:1.0.0"},
		{"opensuse-tumbleweed", "opensuse-tumbleweed", "quay.io/containerdisks/opensuse-tumbleweed:1.0.0"},
		{"opensuse-leap", "opensuse-leap", "quay.io/containerdisks/opensuse-leap:15.6"},
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

// ============================================================================
// Tests for normalizePerformance()
// ============================================================================

func TestNormalizePerformance(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"general-purpose full", "general-purpose", "u1"},
		{"general-purpose no dash", "generalpurpose", "u1"},
		{"general", "general", "u1"},
		{"overcommitted", "overcommitted", "o1"},
		{"compute", "compute", "c1"},
		{"compute-optimized full", "compute-optimized", "c1"},
		{"compute-optimized no dash", "computeoptimized", "c1"},
		{"memory", "memory", "m1"},
		{"memory-optimized full", "memory-optimized", "m1"},
		{"memory-optimized no dash", "memoryoptimized", "m1"},
		{"u1 short form", "u1", "u1"},
		{"o1 short form", "o1", "o1"},
		{"c1 short form", "c1", "c1"},
		{"m1 short form", "m1", "m1"},
		{"uppercase", "GENERAL-PURPOSE", "u1"},
		{"with spaces", " compute ", "c1"},
		{"empty defaults to u1", "", "u1"},
		{"unknown defaults to u1", "unknown", "u1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePerformance(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePerformance(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// Tests for Parameter Helpers
// ============================================================================

func TestGetRequiredString(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		key       string
		expected  string
		wantError bool
	}{
		{
			name:      "returns string value",
			args:      map[string]any{"key": "value"},
			key:       "key",
			expected:  "value",
			wantError: false,
		},
		{
			name:      "returns error when key missing",
			args:      map[string]any{},
			key:       "missing",
			wantError: true,
		},
		{
			name:      "returns error when value is not string",
			args:      map[string]any{"key": 123},
			key:       "key",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := api.ToolHandlerParams{
				ToolCallRequest: &mockToolCallRequest{arguments: tt.args},
			}

			result, err := getRequiredString(params, tt.key)
			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("getRequiredString() = %q, want %q", result, tt.expected)
				}
			}
		})
	}
}

func TestGetOptionalString(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		key      string
		expected string
	}{
		{
			name:     "returns string value",
			args:     map[string]any{"key": "value"},
			key:      "key",
			expected: "value",
		},
		{
			name:     "returns empty when key missing",
			args:     map[string]any{},
			key:      "missing",
			expected: "",
		},
		{
			name:     "returns empty when value is not string",
			args:     map[string]any{"key": 123},
			key:      "key",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := api.ToolHandlerParams{
				ToolCallRequest: &mockToolCallRequest{arguments: tt.args},
			}

			result := getOptionalString(params, tt.key)
			if result != tt.expected {
				t.Errorf("getOptionalString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetOptionalBool(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		key      string
		expected bool
	}{
		{
			name:     "returns true when value is true",
			args:     map[string]any{"key": true},
			key:      "key",
			expected: true,
		},
		{
			name:     "returns false when value is false",
			args:     map[string]any{"key": false},
			key:      "key",
			expected: false,
		},
		{
			name:     "returns false when key missing",
			args:     map[string]any{},
			key:      "missing",
			expected: false,
		},
		{
			name:     "returns false when value is not boolean",
			args:     map[string]any{"key": "not a bool"},
			key:      "key",
			expected: false,
		},
		{
			name:     "returns false when value is number",
			args:     map[string]any{"key": 1},
			key:      "key",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := api.ToolHandlerParams{
				ToolCallRequest: &mockToolCallRequest{arguments: tt.args},
			}

			result := getOptionalBool(params, tt.key)
			if result != tt.expected {
				t.Errorf("getOptionalBool() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ============================================================================
// Tests for Tools()
// ============================================================================

func TestTools(t *testing.T) {
	tools := Tools()

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
