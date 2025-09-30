package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SearchResources searches for a query string in resources, with optional filters for resource type and namespace.
func (k *Kubernetes) SearchResources(ctx context.Context, query, apiVersion, kind, namespaceLabelSelector string, asTable bool) (runtime.Unstructured, error) {
	var matchingResources []unstructured.Unstructured

	if apiVersion != "" && kind != "" {
		// Search in a specific resource type
		gv, err := schema.ParseGroupVersion(apiVersion)
		if err != nil {
			return nil, fmt.Errorf("invalid apiVersion: %w", err)
		}
		gvk := gv.WithKind(kind)
		apiResource, err := k.getAPIResource(&gvk)
		if err != nil {
			return nil, fmt.Errorf("failed to get API resource: %w", err)
		}
		resources, err := k.searchInGVK(ctx, query, &gvk, apiResource, namespaceLabelSelector)
		if err != nil {
			return nil, err
		}
		matchingResources = append(matchingResources, resources...)
	} else {
		// Search in all resources
		serverResources, err := k.manager.discoveryClient.ServerPreferredResources()
		if err != nil {
			return nil, fmt.Errorf("failed to get server resources: %w", err)
		}

		for _, apiResourceList := range serverResources {
			for _, apiResource := range apiResourceList.APIResources {
				gvk := schema.GroupVersionKind{
					Group:   apiResourceList.GroupVersion,
					Version: apiResource.Version,
					Kind:    apiResource.Kind,
				}
				if gvk.Group == "" {
					gvk.Group = "core"
				}
				resources, err := k.searchInGVK(ctx, query, &gvk, &apiResource, namespaceLabelSelector)
				if err != nil {
					// Ignore errors for resources that cannot be searched
					continue
				}
				matchingResources = append(matchingResources, resources...)
			}
		}
	}

	if asTable {
		return k.createTable(matchingResources)
	}

	return &unstructured.UnstructuredList{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "List",
		},
		Items: matchingResources,
	}, nil
}

func (k *Kubernetes) searchInGVK(ctx context.Context, query string, gvk *schema.GroupVersionKind, apiResource *metav1.APIResource, namespaceLabelSelector string) ([]unstructured.Unstructured, error) {
	if !contains(apiResource.Verbs, "list") {
		return nil, nil // Skip resources that do not support the "list" verb
	}

	var matchingResources []unstructured.Unstructured
	var namespaces []string
	if apiResource.Namespaced {
		nsListOptions := ResourceListOptions{}
		if namespaceLabelSelector != "" {
			nsListOptions.LabelSelector = namespaceLabelSelector
		}
		nsListObj, err := k.NamespacesList(ctx, nsListOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list namespaces: %w", err)
		}
		if unstructuredList, ok := nsListObj.(*unstructured.UnstructuredList); ok {
			for _, ns := range unstructuredList.Items {
				namespaces = append(namespaces, ns.GetName())
			}
		}
	} else {
		namespaces = append(namespaces, "") // For cluster-scoped resources
	}

	for _, ns := range namespaces {
		list, err := k.ResourcesList(ctx, gvk, ns, ResourceListOptions{})
		if err != nil {
			continue // Ignore errors for resources that cannot be listed
		}

		if unstructuredList, ok := list.(*unstructured.UnstructuredList); ok {
			for _, item := range unstructuredList.Items {
				match, err := matchResource(item, query)
				if err != nil {
					continue // Ignore errors during matching
				}
				if match {
					matchingResources = append(matchingResources, item)
				}
			}
		}
	}
	return matchingResources, nil
}

func (k *Kubernetes) getAPIResource(gvk *schema.GroupVersionKind) (*metav1.APIResource, error) {
	apiResourceList, err := k.manager.discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return nil, err
	}
	for _, apiResource := range apiResourceList.APIResources {
		if apiResource.Kind == gvk.Kind {
			return &apiResource, nil
		}
	}
	return nil, fmt.Errorf("resource not found for GVK: %s", gvk)
}

func (k *Kubernetes) createTable(resources []unstructured.Unstructured) (runtime.Unstructured, error) {
	table := &metav1.Table{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "meta.k8s.io/v1",
			Kind:       "Table",
		},
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Namespace", Type: "string"},
			{Name: "Kind", Type: "string"},
			{Name: "Name", Type: "string"},
		},
	}

	for _, res := range resources {
		row := metav1.TableRow{
			Cells: []interface{}{
				res.GetNamespace(),
				res.GetKind(),
				res.GetName(),
			},
			Object: runtime.RawExtension{Object: &res},
		}
		table.Rows = append(table.Rows, row)
	}

	unstructuredObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(table)
	if err != nil {
		return nil, fmt.Errorf("failed to convert table to unstructured: %w", err)
	}
	return &unstructured.Unstructured{Object: unstructuredObject}, nil
}

func matchResource(resource unstructured.Unstructured, query string) (bool, error) {
	data, err := json.Marshal(resource.Object)
	if err != nil {
		return false, fmt.Errorf("failed to marshal resource: %w", err)
	}
	return strings.Contains(strings.ToLower(string(data)), strings.ToLower(query)), nil
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
