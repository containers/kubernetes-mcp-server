package create

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"
)

//go:embed plan.tmpl
var planTemplate string

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_create",
				Description: "Generate a comprehensive creation plan for a VirtualMachine, including pre-creation checks for instance types, preferences, and container disk images",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace for the virtual machine",
						},
						"name": {
							Type:        "string",
							Description: "The name of the virtual machine",
						},
						"workload": {
							Type:        "string",
							Description: "The workload for the VM. Accepts OS names (e.g., 'fedora', 'ubuntu', 'centos', 'centos-stream', 'debian', 'rhel', 'opensuse', 'opensuse-tumbleweed', 'opensuse-leap') or full container disk image URLs",
							Examples:    []interface{}{"fedora", "ubuntu", "centos", "debian", "rhel", "quay.io/containerdisks/fedora:latest"},
						},
						"instancetype": {
							Type:        "string",
							Description: "Optional instance type name for the VM (e.g., 'u1.small', 'u1.medium', 'u1.large')",
						},
						"preference": {
							Type:        "string",
							Description: "Optional preference name for the VM",
						},
						"size": {
							Type:        "string",
							Description: "Optional workload size hint for the VM (e.g., 'small', 'medium', 'large', 'xlarge'). Used to auto-select an appropriate instance type if not explicitly specified.",
							Examples:    []interface{}{"small", "medium", "large"},
						},
						"performance": {
							Type:        "string",
							Description: "Optional performance family hint for the VM instance type (e.g., 'u1' for general-purpose, 'o1' for overcommitted, 'c1' for compute-optimized, 'm1' for memory-optimized). Defaults to 'u1' (general-purpose) if not specified.",
							Examples:    []interface{}{"general-purpose", "overcommitted", "compute-optimized", "memory-optimized"},
						},
					},
					Required: []string{"namespace", "name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Create",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: create,
		},
	}
}

type vmParams struct {
	Namespace           string
	Name                string
	ContainerDisk       string
	Instancetype        string
	Preference          string
	UseDataSource       bool
	DataSourceName      string
	DataSourceNamespace string
}

type DataSourceInfo struct {
	Name                string
	Namespace           string
	Source              string
	DefaultInstancetype string
	DefaultPreference   string
}

type PreferenceInfo struct {
	Name string
}

type InstancetypeInfo struct {
	Name   string
	Labels map[string]string
}

func create(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse required parameters
	namespace, err := getRequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	name, err := getRequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	// Parse optional parameters
	osInput := getOptionalString(params, "workload")
	if osInput == "" {
		osInput = "fedora" // Default to fedora if not specified
	}
	instancetype := getOptionalString(params, "instancetype")
	preference := getOptionalString(params, "preference")
	size := getOptionalString(params, "size")
	performance := getOptionalString(params, "performance")

	// Normalize performance parameter to instance type prefix
	performance = normalizePerformance(performance)

	// Search for DataSources in the cluster
	dataSources, err := searchDataSources(params, osInput)
	if err != nil {
		// Don't fail completely, continue without DataSources
		dataSources = []DataSourceInfo{}
	}

	// Check if the operating_system input matches any DataSource
	normalizedInput := strings.ToLower(strings.TrimSpace(osInput))
	var matchedDataSource *DataSourceInfo

	// First try exact match
	for i := range dataSources {
		ds := &dataSources[i]
		if strings.EqualFold(ds.Name, normalizedInput) || strings.EqualFold(ds.Name, osInput) {
			matchedDataSource = ds
			break
		}
	}

	// If no exact match, try partial matching (e.g., "rhel" matches "rhel9")
	// Only match against real DataSources with namespaces, not built-in containerdisks
	if matchedDataSource == nil {
		for i := range dataSources {
			ds := &dataSources[i]
			// Only do partial matching for real DataSources (those with namespaces)
			if ds.Namespace != "" && strings.Contains(strings.ToLower(ds.Name), normalizedInput) {
				matchedDataSource = ds
				break
			}
		}
	}

	// Use DataSource default preference if available and preference not specified
	if preference == "" && matchedDataSource != nil && matchedDataSource.DefaultPreference != "" {
		preference = matchedDataSource.DefaultPreference
	}

	// Check if the operating_system input matches any Preference when preference is not provided
	if preference == "" {
		preferences := searchPreferences(params)
		for i := range preferences {
			pref := &preferences[i]
			// Try to match preference name against the OS input
			// Common patterns: "fedora", "rhel.9", "ubuntu", etc.
			if strings.Contains(strings.ToLower(pref.Name), normalizedInput) {
				preference = pref.Name
				break
			}
		}
	}

	// Use DataSource default instancetype if available and instancetype not specified and size not specified
	if instancetype == "" && size == "" && matchedDataSource != nil && matchedDataSource.DefaultInstancetype != "" {
		instancetype = matchedDataSource.DefaultInstancetype
	}

	// Check if the size parameter matches any instance type when instancetype is not provided
	if instancetype == "" && size != "" {
		instancetypes := searchInstancetypes(params)
		normalizedSize := strings.ToLower(strings.TrimSpace(size))
		normalizedPerformance := strings.ToLower(strings.TrimSpace(performance))

		// First, filter instance types by size
		var candidatesBySize []InstancetypeInfo
		for i := range instancetypes {
			it := &instancetypes[i]
			// Match instance types that contain the size hint in their name
			if strings.Contains(strings.ToLower(it.Name), normalizedSize) {
				candidatesBySize = append(candidatesBySize, *it)
			}
		}

		// Then, filter by performance family
		// Try exact match first (e.g., "u1.small")
		for i := range candidatesBySize {
			it := &candidatesBySize[i]
			// Check if instance type name starts with the performance family
			if strings.HasPrefix(strings.ToLower(it.Name), normalizedPerformance+".") {
				instancetype = it.Name
				break
			}
		}

		// If no exact match, check labels for performance characteristics
		if instancetype == "" {
			for i := range candidatesBySize {
				it := &candidatesBySize[i]
				// Check labels for performance family indicators
				if it.Labels != nil {
					// Check common label patterns
					if class, ok := it.Labels["instancetype.kubevirt.io/class"]; ok {
						if strings.EqualFold(class, normalizedPerformance) {
							instancetype = it.Name
							break
						}
					}
				}
			}
		}

		// If still no match, fall back to first candidate that matches size
		if instancetype == "" && len(candidatesBySize) > 0 {
			instancetype = candidatesBySize[0].Name
		}
	}

	// Prepare template parameters
	templateParams := vmParams{
		Namespace:    namespace,
		Name:         name,
		Instancetype: instancetype,
		Preference:   preference,
	}

	if matchedDataSource != nil && matchedDataSource.Namespace != "" {
		// Use the matched DataSource (real cluster DataSource with namespace)
		templateParams.UseDataSource = true
		templateParams.DataSourceName = matchedDataSource.Name
		templateParams.DataSourceNamespace = matchedDataSource.Namespace
		templateParams.ContainerDisk = "" // Not using container disk
	} else if matchedDataSource != nil {
		// Matched a built-in containerdisk (no namespace)
		templateParams.UseDataSource = false
		templateParams.ContainerDisk = matchedDataSource.Source
	} else {
		// No match, resolve container disk image from OS name
		templateParams.UseDataSource = false
		templateParams.ContainerDisk = resolveContainerDisk(osInput)
	}

	// Render template
	tmpl, err := template.New("vm").Parse(planTemplate)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to parse template: %w", err)), nil
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, templateParams); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to render template: %w", err)), nil
	}

	return api.NewToolCallResult(result.String(), nil), nil
}

// Helper functions

func normalizePerformance(performance string) string {
	// Normalize to lowercase and trim spaces
	normalized := strings.ToLower(strings.TrimSpace(performance))

	// Map natural language terms to instance type prefixes
	performanceMap := map[string]string{
		"general-purpose":   "u1",
		"generalpurpose":    "u1",
		"general":           "u1",
		"overcommitted":     "o1",
		"compute":           "c1",
		"compute-optimized": "c1",
		"computeoptimized":  "c1",
		"memory-optimized":  "m1",
		"memoryoptimized":   "m1",
		"memory":            "m1",
		"u1":                "u1",
		"o1":                "o1",
		"c1":                "c1",
		"m1":                "m1",
	}

	// Look up the mapping
	if prefix, exists := performanceMap[normalized]; exists {
		return prefix
	}

	// Default to "u1" (general-purpose) if not recognized or empty
	return "u1"
}

func getRequiredString(params api.ToolHandlerParams, key string) (string, error) {
	args := params.GetArguments()
	val, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s parameter required", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("%s parameter must be a string", key)
	}
	return str, nil
}

func getOptionalString(params api.ToolHandlerParams, key string) string {
	args := params.GetArguments()
	val, ok := args[key]
	if !ok {
		return ""
	}
	str, ok := val.(string)
	if !ok {
		return ""
	}
	return str
}

// resolveContainerDisk resolves OS names to container disk images from quay.io/containerdisks
func resolveContainerDisk(input string) string {
	// If input already looks like a container image, return as-is
	if strings.Contains(input, "/") || strings.Contains(input, ":") {
		return input
	}

	// Common OS name mappings to containerdisk images
	osMap := map[string]string{
		"fedora":              "quay.io/containerdisks/fedora:latest",
		"ubuntu":              "quay.io/containerdisks/ubuntu:latest",
		"centos":              "quay.io/containerdisks/centos-stream:latest",
		"centos-stream":       "quay.io/containerdisks/centos-stream:latest",
		"debian":              "quay.io/containerdisks/debian:latest",
		"opensuse":            "quay.io/containerdisks/opensuse-tumbleweed:latest",
		"opensuse-tumbleweed": "quay.io/containerdisks/opensuse-tumbleweed:latest",
		"opensuse-leap":       "quay.io/containerdisks/opensuse-leap:latest",
		"rhel8":               "registry.redhat.io/rhel8/rhel-guest-image:latest",
		"rhel9":               "registry.redhat.io/rhel9/rhel-guest-image:latest",
		"rhel10":              "registry.redhat.io/rhel10/rhel-guest-image:latest",
	}

	// Normalize input to lowercase for lookup
	normalized := strings.ToLower(strings.TrimSpace(input))

	// Look up the OS name
	if containerDisk, exists := osMap[normalized]; exists {
		return containerDisk
	}

	// If no match found, return the input as-is (assume it's a valid container image URL)
	return input
}

// getDefaultContainerDisks returns a list of common containerdisk images
func getDefaultContainerDisks() []DataSourceInfo {
	return []DataSourceInfo{
		{
			Name:   "fedora",
			Source: "quay.io/containerdisks/fedora:latest",
		},
		{
			Name:   "ubuntu",
			Source: "quay.io/containerdisks/ubuntu:latest",
		},
		{
			Name:   "centos-stream",
			Source: "quay.io/containerdisks/centos-stream:latest",
		},
		{
			Name:   "debian",
			Source: "quay.io/containerdisks/debian:latest",
		},
		{
			Name:   "rhel8",
			Source: "registry.redhat.io/rhel8/rhel-guest-image:latest",
		},
		{
			Name:   "rhel9",
			Source: "registry.redhat.io/rhel9/rhel-guest-image:latest",
		},
		{
			Name:   "rhel10",
			Source: "registry.redhat.io/rhel10/rhel-guest-image:latest",
		},
	}
}

// searchDataSources searches for DataSource resources in the cluster
func searchDataSources(params api.ToolHandlerParams, query string) ([]DataSourceInfo, error) {
	// Try to get dynamic client to query for DataSources
	// Handle nil or invalid clients gracefully (e.g., in test environments)
	if params.Kubernetes == nil {
		// Return just the built-in containerdisk images
		return getDefaultContainerDisks(), nil
	}

	restConfig := params.RESTConfig()
	if restConfig == nil {
		// Return just the built-in containerdisk images
		return getDefaultContainerDisks(), nil
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		// Return just the built-in containerdisk images
		return getDefaultContainerDisks(), nil
	}

	// DataSource GVR for CDI
	dataSourceGVR := schema.GroupVersionResource{
		Group:    "cdi.kubevirt.io",
		Version:  "v1beta1",
		Resource: "datasources",
	}

	var results []DataSourceInfo

	// Try to list DataSources from OpenShift virtualization OS images namespace first
	openshiftNamespace := "openshift-virtualization-os-images"
	openshiftList, err := dynamicClient.Resource(dataSourceGVR).Namespace(openshiftNamespace).List(params.Context, metav1.ListOptions{})
	if err == nil {
		// Parse OpenShift DataSources
		for _, item := range openshiftList.Items {
			name := item.GetName()
			namespace := item.GetNamespace()
			labels := item.GetLabels()

			// Extract source information from the DataSource spec
			source := extractDataSourceInfo(&item)

			// Extract default instancetype and preference from labels
			defaultInstancetype := ""
			defaultPreference := ""
			if labels != nil {
				defaultInstancetype = labels["instancetype.kubevirt.io/default-instancetype"]
				defaultPreference = labels["instancetype.kubevirt.io/default-preference"]
			}

			results = append(results, DataSourceInfo{
				Name:                name,
				Namespace:           namespace,
				Source:              source,
				DefaultInstancetype: defaultInstancetype,
				DefaultPreference:   defaultPreference,
			})
		}
	}

	// List DataSources from all namespaces
	list, err := dynamicClient.Resource(dataSourceGVR).List(params.Context, metav1.ListOptions{})
	if err != nil {
		// If we found OpenShift DataSources but couldn't list all namespaces, continue
		if len(results) > 0 {
			// Add common containerdisk images as well
			results = append(results, getDefaultContainerDisks()...)
			return results, nil
		}
		// DataSources might not be available, return helpful message
		return []DataSourceInfo{
			{
				Name:      "No DataSources found",
				Namespace: "",
				Source:    "CDI may not be installed or DataSources are not available in this cluster",
			},
		}, nil
	}

	// Parse the results from all namespaces (this will include OpenShift namespace again, but we'll deduplicate)
	seen := make(map[string]bool)
	// Mark OpenShift DataSources as already seen
	for _, ds := range results {
		key := ds.Namespace + "/" + ds.Name
		seen[key] = true
	}

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()
		key := namespace + "/" + name

		// Skip if we've already added this DataSource
		if seen[key] {
			continue
		}

		labels := item.GetLabels()

		// Extract source information from the DataSource spec
		source := extractDataSourceInfo(&item)

		// Extract default instancetype and preference from labels
		defaultInstancetype := ""
		defaultPreference := ""
		if labels != nil {
			defaultInstancetype = labels["instancetype.kubevirt.io/default-instancetype"]
			defaultPreference = labels["instancetype.kubevirt.io/default-preference"]
		}

		results = append(results, DataSourceInfo{
			Name:                name,
			Namespace:           namespace,
			Source:              source,
			DefaultInstancetype: defaultInstancetype,
			DefaultPreference:   defaultPreference,
		})
	}

	// Add common containerdisk images as well
	results = append(results, getDefaultContainerDisks()...)

	if len(results) == 0 {
		return []DataSourceInfo{
			{
				Name:      "No sources available",
				Namespace: "",
				Source:    "No DataSources or containerdisks found",
			},
		}, nil
	}

	return results, nil
}

// searchPreferences searches for VirtualMachineClusterPreference resources in the cluster
func searchPreferences(params api.ToolHandlerParams) []PreferenceInfo {
	// Handle nil or invalid clients gracefully (e.g., in test environments)
	if params.Kubernetes == nil {
		return []PreferenceInfo{}
	}

	restConfig := params.RESTConfig()
	if restConfig == nil {
		return []PreferenceInfo{}
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return []PreferenceInfo{}
	}

	// VirtualMachineClusterPreference GVR
	preferenceGVR := schema.GroupVersionResource{
		Group:    "instancetype.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachineclusterpreferences",
	}

	// List VirtualMachineClusterPreferences
	list, err := dynamicClient.Resource(preferenceGVR).List(params.Context, metav1.ListOptions{})
	if err != nil {
		// Preferences might not be available, return empty list
		return []PreferenceInfo{}
	}

	var results []PreferenceInfo
	for _, item := range list.Items {
		results = append(results, PreferenceInfo{
			Name: item.GetName(),
		})
	}

	return results
}

// searchInstancetypes searches for VirtualMachineClusterInstancetype resources in the cluster
func searchInstancetypes(params api.ToolHandlerParams) []InstancetypeInfo {
	// Handle nil or invalid clients gracefully (e.g., in test environments)
	if params.Kubernetes == nil {
		return []InstancetypeInfo{}
	}

	restConfig := params.RESTConfig()
	if restConfig == nil {
		return []InstancetypeInfo{}
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return []InstancetypeInfo{}
	}

	// VirtualMachineClusterInstancetype GVR
	instancetypeGVR := schema.GroupVersionResource{
		Group:    "instancetype.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachineclusterinstancetypes",
	}

	// List VirtualMachineClusterInstancetypes
	list, err := dynamicClient.Resource(instancetypeGVR).List(params.Context, metav1.ListOptions{})
	if err != nil {
		// Instance types might not be available, return empty list
		return []InstancetypeInfo{}
	}

	var results []InstancetypeInfo
	for _, item := range list.Items {
		results = append(results, InstancetypeInfo{
			Name:   item.GetName(),
			Labels: item.GetLabels(),
		})
	}

	return results
}

// extractDataSourceInfo extracts source information from a DataSource object
func extractDataSourceInfo(obj *unstructured.Unstructured) string {
	// Try to get the source from spec.source
	spec, found, err := unstructured.NestedMap(obj.Object, "spec", "source")
	if err != nil || !found {
		return "unknown source"
	}

	// Check for PVC source
	if pvcInfo, found, _ := unstructured.NestedMap(spec, "pvc"); found {
		if pvcName, found, _ := unstructured.NestedString(pvcInfo, "name"); found {
			if pvcNamespace, found, _ := unstructured.NestedString(pvcInfo, "namespace"); found {
				return fmt.Sprintf("PVC: %s/%s", pvcNamespace, pvcName)
			}
			return fmt.Sprintf("PVC: %s", pvcName)
		}
	}

	// Check for registry source
	if registryInfo, found, _ := unstructured.NestedMap(spec, "registry"); found {
		if url, found, _ := unstructured.NestedString(registryInfo, "url"); found {
			return fmt.Sprintf("Registry: %s", url)
		}
	}

	// Check for http source
	if url, found, _ := unstructured.NestedString(spec, "http", "url"); found {
		return fmt.Sprintf("HTTP: %s", url)
	}

	return "DataSource (type unknown)"
}
