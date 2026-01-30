package kubevirt

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const (
	DefaultInstancetypeLabel = "instancetype.kubevirt.io/default-instancetype"
	DefaultPreferenceLabel   = "instancetype.kubevirt.io/default-preference"
)

// DataSourceInfo contains information about a KubeVirt DataSource
type DataSourceInfo struct {
	Name                string
	Namespace           string
	Source              string
	DefaultInstancetype string
	DefaultPreference   string
}

// PreferenceInfo contains information about a VirtualMachinePreference
type PreferenceInfo struct {
	Name      string
	Namespace string // Empty for cluster-scoped preferences
}

// InstancetypeInfo contains information about a VirtualMachineInstancetype
type InstancetypeInfo struct {
	Name      string
	Namespace string // Empty for cluster-scoped instancetypes
	Labels    map[string]string
}

// SearchDataSources searches for DataSource resources in the cluster.
//
// It searches in well-known namespaces first (openshift-virtualization-os-images,
// kubevirt-os-images), then performs a cluster-wide search. Duplicate DataSources
// are filtered by namespace/name key.
//
// Returns a map of DataSourceInfo indexed by "namespace/name". If no DataSources
// are found, returns a placeholder entry indicating no sources are available.
func SearchDataSources(ctx context.Context, dynamicClient dynamic.Interface) map[string]DataSourceInfo {
	results := collectDataSources(ctx, dynamicClient)
	if len(results) == 0 {
		return map[string]DataSourceInfo{
			"No sources available": {
				Name:      "No sources available",
				Namespace: "",
				Source:    "No DataSources or containerdisks found",
			},
		}
	}
	return results
}

// collectDataSources collects DataSources from well-known namespaces and all namespaces
func collectDataSources(ctx context.Context, dynamicClient dynamic.Interface) map[string]DataSourceInfo {
	// Try to list DataSources from well-known namespaces first
	wellKnownNamespaces := []string{
		"openshift-virtualization-os-images",
		"kubevirt-os-images",
	}

	var items []unstructured.Unstructured
	for _, ns := range wellKnownNamespaces {
		list, err := dynamicClient.Resource(DataSourceGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			slog.Debug("failed to list DataSources in well-known namespace", "namespace", ns, "error", err)
		} else {
			items = append(items, list.Items...)
		}
	}

	// List DataSources from all namespaces
	list, err := dynamicClient.Resource(DataSourceGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Debug("failed to list DataSources cluster-wide", "error", err)
	} else {
		items = append(items, list.Items...)
	}

	results := make(map[string]DataSourceInfo)
	for _, item := range items {
		name := item.GetName()
		namespace := item.GetNamespace()
		key := namespace + "/" + name
		if _, ok := results[key]; ok {
			continue
		}

		labels := item.GetLabels()
		defaultInstancetype := ""
		defaultPreference := ""
		if labels != nil {
			defaultInstancetype = labels[DefaultInstancetypeLabel]
			defaultPreference = labels[DefaultPreferenceLabel]
		}

		source := ExtractDataSourceInfo(&item)
		results[key] = DataSourceInfo{
			Name:                name,
			Namespace:           namespace,
			Source:              source,
			DefaultInstancetype: defaultInstancetype,
			DefaultPreference:   defaultPreference,
		}
	}
	return results
}

// SearchPreferences searches for both cluster-wide and namespaced VirtualMachinePreference resources.
//
// It queries both VirtualMachineClusterPreferences (cluster-scoped) and
// VirtualMachinePreferences (namespaced) resources. Each PreferenceInfo includes
// a Namespace field that is empty for cluster-scoped resources.
//
// Parameters:
//   - ctx: Context for the API calls
//   - dynamicClient: Kubernetes dynamic client
//   - namespace: Namespace to search for namespaced preferences
//
// Returns a list of PreferenceInfo objects. Returns empty list if no preferences found
// or if API calls fail.
func SearchPreferences(ctx context.Context, dynamicClient dynamic.Interface, namespace string) []PreferenceInfo {
	// Search for cluster-wide VirtualMachineClusterPreferences
	var results []PreferenceInfo
	clusterList, err := dynamicClient.Resource(VirtualMachineClusterPreferenceGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Debug("failed to list cluster-scoped VirtualMachineClusterPreferences", "error", err)
	} else {
		for _, item := range clusterList.Items {
			results = append(results, PreferenceInfo{
				Name:      item.GetName(),
				Namespace: "", // Cluster-scoped
			})
		}
	}

	// Search for namespaced VirtualMachinePreferences
	namespacedList, err := dynamicClient.Resource(VirtualMachinePreferenceGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Debug("failed to list namespaced VirtualMachinePreferences", "namespace", namespace, "error", err)
	} else {
		for _, item := range namespacedList.Items {
			results = append(results, PreferenceInfo{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			})
		}
	}

	return results
}

// SearchInstancetypes searches for both cluster-wide and namespaced VirtualMachineInstancetype resources.
//
// It queries both VirtualMachineClusterInstancetypes (cluster-scoped) and
// VirtualMachineInstancetypes (namespaced) resources. Each InstancetypeInfo includes
// a Namespace field that is empty for cluster-scoped resources, plus Labels for
// filtering by performance class.
//
// Parameters:
//   - ctx: Context for the API calls
//   - dynamicClient: Kubernetes dynamic client
//   - namespace: Namespace to search for namespaced instancetypes
//
// Returns a list of InstancetypeInfo objects. Returns empty list if no instancetypes found
// or if API calls fail.
func SearchInstancetypes(ctx context.Context, dynamicClient dynamic.Interface, namespace string) []InstancetypeInfo {
	// Search for cluster-wide VirtualMachineClusterInstancetypes
	var results []InstancetypeInfo
	clusterList, err := dynamicClient.Resource(VirtualMachineClusterInstancetypeGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Debug("failed to list cluster-scoped VirtualMachineClusterInstancetypes", "error", err)
	} else {
		for _, item := range clusterList.Items {
			results = append(results, InstancetypeInfo{
				Name:      item.GetName(),
				Namespace: "", // Cluster-scoped
				Labels:    item.GetLabels(),
			})
		}
	}

	// Search for namespaced VirtualMachineInstancetypes
	namespacedList, err := dynamicClient.Resource(VirtualMachineInstancetypeGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Debug("failed to list namespaced VirtualMachineInstancetypes", "namespace", namespace, "error", err)
	} else {
		for _, item := range namespacedList.Items {
			results = append(results, InstancetypeInfo{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
				Labels:    item.GetLabels(),
			})
		}
	}

	return results
}

// MatchDataSource finds a DataSource that matches the workload input.
//
// Matching strategy:
//  1. Exact name match (case-insensitive)
//  2. Partial match for DataSources with namespaces (real cluster resources)
//     e.g., "rhel" matches "rhel9"
//
// Built-in containerdisks (without namespaces) are excluded from partial matching
// to avoid ambiguous matches.
//
// Parameters:
//   - dataSources: Map of available DataSources keyed by "namespace/name"
//   - workload: User input (OS name, DataSource name, or container image)
//
// Returns a pointer to matched DataSourceInfo, or nil if no match found.
func MatchDataSource(dataSources map[string]DataSourceInfo, workload string) *DataSourceInfo {
	normalizedInput := strings.ToLower(strings.TrimSpace(workload))

	// First try exact match
	for _, ds := range dataSources {
		if strings.EqualFold(ds.Name, normalizedInput) || strings.EqualFold(ds.Name, workload) {
			return &ds
		}
	}

	// If no exact match, try partial matching (e.g., "rhel" matches "rhel9")
	// Only match against real DataSources with namespaces, not built-in containerdisks
	for _, ds := range dataSources {
		// Only do partial matching for real DataSources (those with namespaces)
		if ds.Namespace != "" && strings.Contains(strings.ToLower(ds.Name), normalizedInput) {
			return &ds
		}
	}

	return nil
}

// MatchInstancetypeBySize finds an instancetype that matches the size and performance hints.
//
// Matching strategy:
//  1. Filter instancetypes by size (e.g., "medium" matches "*.medium")
//  2. Try to match by performance family prefix (e.g., "c1" matches "c1.medium")
//  3. Try to match by performance family label (instancetype.kubevirt.io/class)
//  4. Fall back to first instancetype that matches size
//
// Parameters:
//   - instancetypes: List of available instancetypes
//   - size: Size hint (e.g., "small", "medium", "large")
//   - performance: Performance class hint (e.g., "u1", "c1", "m1")
//
// Returns the matched instancetype name, or empty string if no match found.
func MatchInstancetypeBySize(instancetypes []InstancetypeInfo, size, performance string) string {
	normalizedSize := strings.ToLower(strings.TrimSpace(size))
	normalizedPerformance := strings.ToLower(strings.TrimSpace(performance))

	// Filter instance types by size
	candidatesBySize := FilterInstancetypesBySize(instancetypes, normalizedSize)
	if len(candidatesBySize) == 0 {
		return ""
	}

	// Try to match by performance family prefix (e.g., "u1.small")
	for i := range candidatesBySize {
		it := &candidatesBySize[i]
		if strings.HasPrefix(strings.ToLower(it.Name), normalizedPerformance+".") {
			return it.Name
		}
	}

	// Try to match by performance family label
	for i := range candidatesBySize {
		it := &candidatesBySize[i]
		if it.Labels != nil {
			if class, ok := it.Labels["instancetype.kubevirt.io/class"]; ok {
				if strings.EqualFold(class, normalizedPerformance) {
					return it.Name
				}
			}
		}
	}

	// Fall back to first candidate that matches size
	return candidatesBySize[0].Name
}

// FilterInstancetypesBySize filters instancetypes that contain the size hint in their name.
//
// Parameters:
//   - instancetypes: List of available instancetypes
//   - normalizedSize: Lowercase size hint (e.g., "small", "medium", "large")
//
// Returns a filtered list of instancetypes whose names contain the size string.
// For example, "medium" matches "u1.medium", "c1.medium", etc.
func FilterInstancetypesBySize(instancetypes []InstancetypeInfo, normalizedSize string) []InstancetypeInfo {
	var candidates []InstancetypeInfo
	for i := range instancetypes {
		it := &instancetypes[i]
		if strings.Contains(strings.ToLower(it.Name), normalizedSize) {
			candidates = append(candidates, *it)
		}
	}
	return candidates
}

// ResolvePreference determines the preference to use from DataSource defaults or cluster resources.
//
// Resolution priority:
//  1. Explicit preference parameter (if provided)
//  2. DataSource default preference (if DataSource matched and has default)
//  3. Auto-match preference name against workload input
//     e.g., "rhel" matches "rhel.9"
//
// Parameters:
//   - preferences: List of available preferences from the cluster
//   - explicitPreference: User-specified preference name (may be empty)
//   - workload: Workload/OS name used for auto-matching
//   - matchedDataSource: Matched DataSource (may be nil)
//
// Returns a pointer to PreferenceInfo with name and scope, or nil if no match found.
// If a preference name is provided but not found in available preferences, assumes
// it's cluster-scoped.
func ResolvePreference(preferences []PreferenceInfo, explicitPreference, workload string, matchedDataSource *DataSourceInfo) *PreferenceInfo {
	// If explicit preference is specified, try to find it in available preferences
	if explicitPreference != "" {
		for i := range preferences {
			if preferences[i].Name == explicitPreference {
				return &preferences[i]
			}
		}
		// If not found in available preferences, assume it's cluster-scoped
		return &PreferenceInfo{Name: explicitPreference, Namespace: ""}
	}

	// Use DataSource default preference if available
	if matchedDataSource != nil && matchedDataSource.DefaultPreference != "" {
		// Try to find the default preference in available preferences
		for i := range preferences {
			if preferences[i].Name == matchedDataSource.DefaultPreference {
				return &preferences[i]
			}
		}
		// If not found, assume it's cluster-scoped
		return &PreferenceInfo{Name: matchedDataSource.DefaultPreference, Namespace: ""}
	}

	// Try to match preference name against the workload input
	normalizedInput := strings.ToLower(strings.TrimSpace(workload))
	for i := range preferences {
		pref := &preferences[i]
		// Common patterns: "fedora", "rhel.9", "ubuntu", etc.
		if strings.Contains(strings.ToLower(pref.Name), normalizedInput) {
			return pref
		}
	}
	return nil
}

// ResolveInstancetype determines the instancetype to use from DataSource defaults or size/performance hints.
//
// Resolution priority:
//  1. Explicit instancetype parameter (if provided)
//  2. DataSource default instancetype (if DataSource matched, has default, and no size specified)
//  3. Auto-match by size and performance hints
//     e.g., size="large" + performance="c1" matches "c1.large"
//
// Parameters:
//   - instancetypes: List of available instancetypes from the cluster
//   - explicitInstancetype: User-specified instancetype name (may be empty)
//   - size: Size hint (e.g., "small", "medium", "large") - may be empty
//   - performance: Performance class hint (e.g., "u1", "c1", "m1") - may be empty
//   - matchedDataSource: Matched DataSource (may be nil)
//
// Returns a pointer to InstancetypeInfo with name and scope, or nil if no match found.
// If an instancetype name is provided but not found in available instancetypes, assumes
// it's cluster-scoped.
func ResolveInstancetype(instancetypes []InstancetypeInfo, explicitInstancetype, size, performance string, matchedDataSource *DataSourceInfo) *InstancetypeInfo {
	// If explicit instancetype is specified, try to find it in available instancetypes
	if explicitInstancetype != "" {
		for i := range instancetypes {
			if instancetypes[i].Name == explicitInstancetype {
				return &instancetypes[i]
			}
		}
		// If not found in available instancetypes, assume it's cluster-scoped
		return &InstancetypeInfo{Name: explicitInstancetype, Namespace: ""}
	}

	// Use DataSource default instancetype if available (when size not specified)
	if size == "" && matchedDataSource != nil && matchedDataSource.DefaultInstancetype != "" {
		// Try to find the default instancetype in available instancetypes
		for i := range instancetypes {
			if instancetypes[i].Name == matchedDataSource.DefaultInstancetype {
				return &instancetypes[i]
			}
		}
		// If not found, assume it's cluster-scoped
		return &InstancetypeInfo{Name: matchedDataSource.DefaultInstancetype, Namespace: ""}
	}

	// Match instancetype based on size and performance hints
	if size != "" {
		name := MatchInstancetypeBySize(instancetypes, size, performance)
		if name != "" {
			// Find the matched instancetype to get its namespace
			for i := range instancetypes {
				if instancetypes[i].Name == name {
					return &instancetypes[i]
				}
			}
		}
	}

	return nil
}

// ExtractDataSourceInfo extracts source information from a DataSource object.
//
// Supports multiple source types:
//   - PVC: Returns "PVC: namespace/name" or "PVC: name"
//   - Registry: Returns "Registry: url"
//   - HTTP: Returns "HTTP: url"
//
// Parameters:
//   - obj: Unstructured DataSource object
//
// Returns a human-readable string describing the source, or "unknown source"/"DataSource (type unknown)"
// if the source cannot be determined.
func ExtractDataSourceInfo(obj *unstructured.Unstructured) string {
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
