package kubevirt

import (
	"context"
	"testing"

	kubevirttesting "github.com/containers/kubernetes-mcp-server/pkg/kubevirt/testing"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestMatchDataSource(t *testing.T) {
	tests := []struct {
		name        string
		dataSources map[string]DataSourceInfo
		workload    string
		wantMatch   bool
		wantName    string
	}{
		{
			name: "exact match",
			dataSources: map[string]DataSourceInfo{
				"ns/fedora": {Name: "fedora", Namespace: "os-images"},
				"ns/ubuntu": {Name: "ubuntu", Namespace: "os-images"},
			},
			workload:  "fedora",
			wantMatch: true,
			wantName:  "fedora",
		},
		{
			name: "case insensitive match",
			dataSources: map[string]DataSourceInfo{
				"ns/fedora": {Name: "fedora", Namespace: "os-images"},
			},
			workload:  "FEDORA",
			wantMatch: true,
			wantName:  "fedora",
		},
		{
			name: "partial match with namespace",
			dataSources: map[string]DataSourceInfo{
				"ns/rhel9": {Name: "rhel9", Namespace: "os-images"},
			},
			workload:  "rhel",
			wantMatch: true,
			wantName:  "rhel9",
		},
		{
			name: "no match",
			dataSources: map[string]DataSourceInfo{
				"ns/fedora": {Name: "fedora", Namespace: "os-images"},
			},
			workload:  "ubuntu",
			wantMatch: false,
		},
		{
			name: "partial match ignores built-in containerdisks without namespace",
			dataSources: map[string]DataSourceInfo{
				"builtin": {Name: "rhel9", Namespace: ""}, // No namespace = built-in
			},
			workload:  "rhel",
			wantMatch: false, // Should not match built-ins with partial match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchDataSource(tt.dataSources, tt.workload)
			if tt.wantMatch {
				if result == nil {
					t.Error("Expected a match, got nil")
					return
				}
				if result.Name != tt.wantName {
					t.Errorf("Matched DataSource name = %q, want %q", result.Name, tt.wantName)
				}
			} else {
				if result != nil {
					t.Errorf("Expected no match, got %v", result)
				}
			}
		})
	}
}

func TestFilterInstancetypesBySize(t *testing.T) {
	tests := []struct {
		name           string
		instancetypes  []InstancetypeInfo
		normalizedSize string
		expectedCount  int
		expectedNames  []string
	}{
		{
			name: "filters by size",
			instancetypes: []InstancetypeInfo{
				{Name: "u1.small"},
				{Name: "u1.medium"},
				{Name: "u1.large"},
				{Name: "c1.small"},
			},
			normalizedSize: "small",
			expectedCount:  2,
			expectedNames:  []string{"u1.small", "c1.small"},
		},
		{
			name: "no matches",
			instancetypes: []InstancetypeInfo{
				{Name: "u1.small"},
				{Name: "u1.medium"},
			},
			normalizedSize: "xlarge",
			expectedCount:  0,
		},
		{
			name:           "empty input",
			instancetypes:  []InstancetypeInfo{},
			normalizedSize: "small",
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterInstancetypesBySize(tt.instancetypes, tt.normalizedSize)
			if len(result) != tt.expectedCount {
				t.Errorf("FilterInstancetypesBySize() returned %d results, want %d", len(result), tt.expectedCount)
				return
			}
			if tt.expectedNames != nil {
				resultNames := make(map[string]bool)
				for _, r := range result {
					resultNames[r.Name] = true
				}
				for _, expected := range tt.expectedNames {
					if !resultNames[expected] {
						t.Errorf("Expected %q in results, but not found", expected)
					}
				}
			}
		})
	}
}

func TestMatchInstancetypeBySize(t *testing.T) {
	tests := []struct {
		name          string
		instancetypes []InstancetypeInfo
		size          string
		performance   string
		expected      string
	}{
		{
			name: "matches by performance prefix",
			instancetypes: []InstancetypeInfo{
				{Name: "c1.medium"},
				{Name: "u1.medium"},
			},
			size:        "medium",
			performance: "c1",
			expected:    "c1.medium",
		},
		{
			name: "matches by performance label",
			instancetypes: []InstancetypeInfo{
				{Name: "memory.large", Labels: map[string]string{"instancetype.kubevirt.io/class": "m1"}},
				{Name: "u1.large", Labels: map[string]string{"instancetype.kubevirt.io/class": "general"}},
			},
			size:        "large",
			performance: "m1",
			expected:    "memory.large",
		},
		{
			name: "returns a match when no performance match",
			instancetypes: []InstancetypeInfo{
				{Name: "u1.small"},
			},
			size:        "small",
			performance: "unknown",
			expected:    "u1.small",
		},
		{
			name: "returns empty when no size match",
			instancetypes: []InstancetypeInfo{
				{Name: "u1.small"},
				{Name: "u1.medium"},
			},
			size:        "nonexistent",
			performance: "u1",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchInstancetypeBySize(tt.instancetypes, tt.size, tt.performance)
			if result != tt.expected {
				t.Errorf("MatchInstancetypeBySize() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractDataSourceInfo(t *testing.T) {
	tests := []struct {
		name     string
		obj      *unstructured.Unstructured
		expected string
	}{
		{
			name:     "extracts registry source",
			obj:      kubevirttesting.NewUnstructuredDataSource("test", "test-ns", "registry.example.com/image:tag", "", ""),
			expected: "Registry: registry.example.com/image:tag",
		},
		{
			name: "extracts PVC source with namespace",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cdi.kubevirt.io/v1beta1",
					"kind":       "DataSource",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test-ns",
					},
					"spec": map[string]interface{}{
						"source": map[string]interface{}{
							"pvc": map[string]interface{}{
								"name":      "my-pvc",
								"namespace": "pvc-ns",
							},
						},
					},
				},
			},
			expected: "PVC: pvc-ns/my-pvc",
		},
		{
			name: "extracts PVC source without namespace",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cdi.kubevirt.io/v1beta1",
					"kind":       "DataSource",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test-ns",
					},
					"spec": map[string]interface{}{
						"source": map[string]interface{}{
							"pvc": map[string]interface{}{
								"name": "my-pvc",
							},
						},
					},
				},
			},
			expected: "PVC: my-pvc",
		},
		{
			name: "extracts HTTP source",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cdi.kubevirt.io/v1beta1",
					"kind":       "DataSource",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test-ns",
					},
					"spec": map[string]interface{}{
						"source": map[string]interface{}{
							"http": map[string]interface{}{
								"url": "http://example.com/disk.img",
							},
						},
					},
				},
			},
			expected: "HTTP: http://example.com/disk.img",
		},
		{
			name: "handles missing spec",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cdi.kubevirt.io/v1beta1",
					"kind":       "DataSource",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test-ns",
					},
				},
			},
			expected: "unknown source",
		},
		{
			name: "handles unknown source type",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cdi.kubevirt.io/v1beta1",
					"kind":       "DataSource",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test-ns",
					},
					"spec": map[string]interface{}{
						"source": map[string]interface{}{
							"other": map[string]interface{}{
								"value": "something",
							},
						},
					},
				},
			},
			expected: "DataSource (type unknown)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractDataSourceInfo(tt.obj)
			if result != tt.expected {
				t.Errorf("ExtractDataSourceInfo() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResolvePreference(t *testing.T) {
	tests := []struct {
		name               string
		preferences        []PreferenceInfo
		explicitPreference string
		workload           string
		matchedDataSource  *DataSourceInfo
		expected           string
	}{
		{
			name:               "explicit preference takes priority",
			preferences:        []PreferenceInfo{{Name: "rhel.9"}, {Name: "fedora"}},
			explicitPreference: "custom.preference",
			workload:           "fedora",
			matchedDataSource:  &DataSourceInfo{DefaultPreference: "rhel.9"},
			expected:           "custom.preference",
		},
		{
			name:              "uses DataSource default when no explicit preference",
			preferences:       []PreferenceInfo{{Name: "rhel.9"}, {Name: "fedora"}},
			workload:          "fedora",
			matchedDataSource: &DataSourceInfo{DefaultPreference: "rhel.9"},
			expected:          "rhel.9",
		},
		{
			name:        "matches preference by workload name",
			preferences: []PreferenceInfo{{Name: "rhel.9"}, {Name: "fedora"}},
			workload:    "rhel",
			expected:    "rhel.9",
		},
		{
			name:        "returns empty when no match",
			preferences: []PreferenceInfo{{Name: "fedora"}},
			workload:    "ubuntu",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolvePreference(tt.preferences, tt.explicitPreference, tt.workload, tt.matchedDataSource)
			if tt.expected == "" {
				if result != nil {
					t.Errorf("ResolvePreference() = %v, want nil", result)
				}
			} else {
				if result == nil {
					t.Errorf("ResolvePreference() = nil, want %q", tt.expected)
				} else if result.Name != tt.expected {
					t.Errorf("ResolvePreference() = %q, want %q", result.Name, tt.expected)
				}
			}
		})
	}
}

func TestResolveInstancetype(t *testing.T) {
	tests := []struct {
		name                 string
		instancetypes        []InstancetypeInfo
		explicitInstancetype string
		size                 string
		performance          string
		matchedDataSource    *DataSourceInfo
		expected             string
	}{
		{
			name:                 "explicit instancetype takes priority",
			instancetypes:        []InstancetypeInfo{{Name: "u1.medium"}},
			explicitInstancetype: "custom.type",
			size:                 "large",
			performance:          "u1",
			matchedDataSource:    &DataSourceInfo{DefaultInstancetype: "u1.medium"},
			expected:             "custom.type",
		},
		{
			name:              "uses DataSource default when no size specified",
			instancetypes:     []InstancetypeInfo{{Name: "u1.medium"}},
			matchedDataSource: &DataSourceInfo{DefaultInstancetype: "u1.medium"},
			expected:          "u1.medium",
		},
		{
			name:              "size overrides DataSource default",
			instancetypes:     []InstancetypeInfo{{Name: "u1.large"}},
			size:              "large",
			performance:       "u1",
			matchedDataSource: &DataSourceInfo{DefaultInstancetype: "u1.medium"},
			expected:          "u1.large",
		},
		{
			name:          "matches by size and performance",
			instancetypes: []InstancetypeInfo{{Name: "c1.large"}, {Name: "u1.large"}},
			size:          "large",
			performance:   "c1",
			expected:      "c1.large",
		},
		{
			name:     "returns empty when no match",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveInstancetype(tt.instancetypes, tt.explicitInstancetype, tt.size, tt.performance, tt.matchedDataSource)
			if tt.expected == "" {
				if result != nil {
					t.Errorf("ResolveInstancetype() = %v, want nil", result)
				}
			} else {
				if result == nil {
					t.Errorf("ResolveInstancetype() = nil, want %q", tt.expected)
				} else if result.Name != tt.expected {
					t.Errorf("ResolveInstancetype() = %q, want %q", result.Name, tt.expected)
				}
			}
		})
	}
}

func TestSearchDataSources(t *testing.T) {
	tests := []struct {
		name      string
		objects   []runtime.Object
		wantCount int
		wantKey   string
	}{
		{
			name: "finds DataSources",
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredDataSource("fedora", "kubevirt-os-images", "registry.example.com/fedora:latest", "", ""),
			},
			wantCount: 1,
			wantKey:   "kubevirt-os-images/fedora",
		},
		{
			name:      "returns placeholder when no DataSources found",
			objects:   []runtime.Object{},
			wantCount: 1,
			wantKey:   "No sources available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvrToListKind := map[schema.GroupVersionResource]string{
				{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "datasources"}: "DataSourceList",
			}
			fakeDynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, tt.objects...)

			result := SearchDataSources(context.Background(), fakeDynamicClient)
			if len(result) != tt.wantCount {
				t.Errorf("SearchDataSources() returned %d results, want %d", len(result), tt.wantCount)
			}
			if _, ok := result[tt.wantKey]; !ok {
				t.Errorf("Expected key %q not found in results", tt.wantKey)
			}
		})
	}
}

func TestSearchPreferences(t *testing.T) {
	tests := []struct {
		name      string
		objects   []runtime.Object
		namespace string
		wantNames []string
	}{
		{
			name: "finds cluster and namespaced preferences",
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredPreference("rhel.9", false),
				kubevirttesting.NewUnstructuredPreference("fedora", true),
			},
			namespace: "test-ns",
			wantNames: []string{"rhel.9", "fedora"},
		},
		{
			name:      "returns empty when no preferences found",
			objects:   []runtime.Object{},
			namespace: "test-ns",
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvrToListKind := map[schema.GroupVersionResource]string{
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterpreferences"}: "VirtualMachineClusterPreferenceList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinepreferences"}:        "VirtualMachinePreferenceList",
			}
			fakeDynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, tt.objects...)

			result := SearchPreferences(context.Background(), fakeDynamicClient, tt.namespace)
			if len(result) != len(tt.wantNames) {
				t.Errorf("SearchPreferences() returned %d results, want %d", len(result), len(tt.wantNames))
			}

			resultNames := make(map[string]bool)
			for _, pref := range result {
				resultNames[pref.Name] = true
			}
			for _, expected := range tt.wantNames {
				if !resultNames[expected] {
					t.Errorf("Expected preference %q not found in results", expected)
				}
			}
		})
	}
}

func TestSearchInstancetypes(t *testing.T) {
	tests := []struct {
		name      string
		objects   []runtime.Object
		namespace string
		wantNames []string
	}{
		{
			name: "finds cluster and namespaced instancetypes",
			objects: []runtime.Object{
				kubevirttesting.NewUnstructuredInstancetype("u1.medium", map[string]string{}),
				kubevirttesting.NewUnstructuredInstancetype("c1.large", map[string]string{"instancetype.kubevirt.io/class": "compute"}),
			},
			namespace: "test-ns",
			wantNames: []string{"u1.medium", "c1.large"},
		},
		{
			name:      "returns empty when no instancetypes found",
			objects:   []runtime.Object{},
			namespace: "test-ns",
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvrToListKind := map[schema.GroupVersionResource]string{
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterinstancetypes"}: "VirtualMachineClusterInstancetypeList",
				{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineinstancetypes"}:        "VirtualMachineInstancetypeList",
			}
			fakeDynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, tt.objects...)

			result := SearchInstancetypes(context.Background(), fakeDynamicClient, tt.namespace)
			if len(result) != len(tt.wantNames) {
				t.Errorf("SearchInstancetypes() returned %d results, want %d", len(result), len(tt.wantNames))
			}

			resultNames := make(map[string]bool)
			for _, it := range result {
				resultNames[it.Name] = true
			}
			for _, expected := range tt.wantNames {
				if !resultNames[expected] {
					t.Errorf("Expected instancetype %q not found in results", expected)
				}
			}
		})
	}
}
