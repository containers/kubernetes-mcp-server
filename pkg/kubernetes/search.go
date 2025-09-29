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

// SearchResources searches for a query string in all resources across all namespaces.
func (k *Kubernetes) SearchResources(ctx context.Context, query string, asTable bool) (runtime.Unstructured, error) {
	// Discovery client is used to discover different supported API groups, versions and resources.
	serverResources, err := k.manager.discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get server resources: %w", err)
	}

	var matchingResources []unstructured.Unstructured
	for _, apiResourceList := range serverResources {
		for _, apiResource := range apiResourceList.APIResources {
			// Skip resources that do not support the "list" verb
			if !contains(apiResource.Verbs, "list") {
				continue
			}

			gvk := schema.GroupVersionKind{
				Group:   apiResourceList.GroupVersion,
				Version: apiResource.Version,
				Kind:    apiResource.Kind,
			}
			if gvk.Group == "" {
				gvk.Group = "core"
			}

			if _, err := k.resourceFor(&gvk); err != nil {
				// Ignore errors for resources that cannot be mapped
				continue
			}
			var namespaces []string
			if apiResource.Namespaced {
				// Get all namespaces
				nsListObj, err := k.NamespacesList(ctx, ResourceListOptions{})
				if err != nil {
					return nil, fmt.Errorf("failed to list namespaces: %w", err)
				}
				if unstructuredList, ok := nsListObj.(*unstructured.UnstructuredList); ok {
					for _, ns := range unstructuredList.Items {
						namespaces = append(namespaces, ns.GetName())
					}
				}
			} else {
				// For cluster-scoped resources, use an empty namespace
				namespaces = append(namespaces, "")
			}

			for _, ns := range namespaces {
				list, err := k.ResourcesList(ctx, &gvk, ns, ResourceListOptions{})
				if err != nil {
					// Ignore errors for resources that cannot be listed
					continue
				}

				if unstructuredList, ok := list.(*unstructured.UnstructuredList); ok {
					for _, item := range unstructuredList.Items {
						match, err := matchResource(item, query)
						if err != nil {
							// Ignore errors during matching
							continue
						}
						if match {
							matchingResources = append(matchingResources, item)
						}
					}
				}
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
