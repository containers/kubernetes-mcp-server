package kubevirt

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	Name string
}

// InstancetypeInfo contains information about a VirtualMachineInstancetype
type InstancetypeInfo struct {
	Name   string
	Labels map[string]string
}

// SearchDataSources searches for DataSource resources in the cluster
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
	gvr := schema.GroupVersionResource{
		Group:    "cdi.kubevirt.io",
		Version:  "v1beta1",
		Resource: "datasources",
	}

	// Try to list DataSources from well-known namespaces first
	wellKnownNamespaces := []string{
		"openshift-virtualization-os-images",
		"kubevirt-os-images",
	}

	var items []unstructured.Unstructured
	for _, ns := range wellKnownNamespaces {
		list, err := dynamicClient.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err == nil {
			items = append(items, list.Items...)
		}
	}

	// List DataSources from all namespaces
	list, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err == nil {
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

// SearchPreferences searches for both cluster-wide and namespaced VirtualMachinePreference resources
func SearchPreferences(ctx context.Context, dynamicClient dynamic.Interface, namespace string) []PreferenceInfo {
	// Search for cluster-wide VirtualMachineClusterPreferences
	clusterPreferenceGVR := schema.GroupVersionResource{
		Group:    "instancetype.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachineclusterpreferences",
	}

	var results []PreferenceInfo
	clusterList, err := dynamicClient.Resource(clusterPreferenceGVR).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, item := range clusterList.Items {
			results = append(results, PreferenceInfo{
				Name: item.GetName(),
			})
		}
	}

	// Search for namespaced VirtualMachinePreferences
	namespacedPreferenceGVR := schema.GroupVersionResource{
		Group:    "instancetype.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachinepreferences",
	}

	namespacedList, err := dynamicClient.Resource(namespacedPreferenceGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, item := range namespacedList.Items {
			results = append(results, PreferenceInfo{
				Name: item.GetName(),
			})
		}
	}

	return results
}

// SearchInstancetypes searches for both cluster-wide and namespaced VirtualMachineInstancetype resources
func SearchInstancetypes(ctx context.Context, dynamicClient dynamic.Interface, namespace string) []InstancetypeInfo {
	// Search for cluster-wide VirtualMachineClusterInstancetypes
	clusterInstancetypeGVR := schema.GroupVersionResource{
		Group:    "instancetype.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachineclusterinstancetypes",
	}

	var results []InstancetypeInfo
	clusterList, err := dynamicClient.Resource(clusterInstancetypeGVR).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, item := range clusterList.Items {
			results = append(results, InstancetypeInfo{
				Name:   item.GetName(),
				Labels: item.GetLabels(),
			})
		}
	}

	// Search for namespaced VirtualMachineInstancetypes
	namespacedInstancetypeGVR := schema.GroupVersionResource{
		Group:    "instancetype.kubevirt.io",
		Version:  "v1beta1",
		Resource: "virtualmachineinstancetypes",
	}

	namespacedList, err := dynamicClient.Resource(namespacedInstancetypeGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, item := range namespacedList.Items {
			results = append(results, InstancetypeInfo{
				Name:   item.GetName(),
				Labels: item.GetLabels(),
			})
		}
	}

	return results
}

// MatchDataSource finds a DataSource that matches the workload input
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

// MatchInstancetypeBySize finds an instancetype that matches the size and performance hints
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

// FilterInstancetypesBySize filters instancetypes that contain the size hint in their name
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

// ResolvePreference determines the preference to use from DataSource defaults or cluster resources
func ResolvePreference(preferences []PreferenceInfo, explicitPreference, workload string, matchedDataSource *DataSourceInfo) string {
	if explicitPreference != "" {
		return explicitPreference
	}

	if matchedDataSource != nil && matchedDataSource.DefaultPreference != "" {
		return matchedDataSource.DefaultPreference
	}

	// Try to match preference name against the workload input
	normalizedInput := strings.ToLower(strings.TrimSpace(workload))
	for i := range preferences {
		pref := &preferences[i]
		// Common patterns: "fedora", "rhel.9", "ubuntu", etc.
		if strings.Contains(strings.ToLower(pref.Name), normalizedInput) {
			return pref.Name
		}
	}
	return ""
}

// ResolveInstancetype determines the instancetype to use from DataSource defaults or size/performance hints
func ResolveInstancetype(instancetypes []InstancetypeInfo, explicitInstancetype, size, performance string, matchedDataSource *DataSourceInfo) string {
	// Use explicitly specified instancetype if provided
	if explicitInstancetype != "" {
		return explicitInstancetype
	}

	// Use DataSource default instancetype if available (when size not specified)
	if size == "" && matchedDataSource != nil && matchedDataSource.DefaultInstancetype != "" {
		return matchedDataSource.DefaultInstancetype
	}

	// Match instancetype based on size and performance hints
	if size != "" {
		return MatchInstancetypeBySize(instancetypes, size, performance)
	}

	return ""
}

// ExtractDataSourceInfo extracts source information from a DataSource object
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
