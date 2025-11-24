package kubernetes

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/containers/kubernetes-mcp-server/pkg/version"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	AppKubernetesComponent = "app.kubernetes.io/component"
	AppKubernetesManagedBy = "app.kubernetes.io/managed-by"
	AppKubernetesName      = "app.kubernetes.io/name"
	AppKubernetesPartOf    = "app.kubernetes.io/part-of"
)

type ResourceListOptions struct {
	metav1.ListOptions
	AsTable bool
}

func (k *Kubernetes) ResourcesList(ctx context.Context, gvk *schema.GroupVersionKind, namespace string, options ResourceListOptions) (runtime.Unstructured, error) {
	gvr, err := k.resourceFor(gvk)
	if err != nil {
		return nil, err
	}

	// Check if operation is allowed for all namespaces (applicable for namespaced resources)
	isNamespaced, _ := k.isNamespaced(gvk)
	if isNamespaced && !k.canIUse(ctx, gvr, namespace, "list") && namespace == "" {
		namespace = k.manager.configuredNamespace()
	}
	if options.AsTable {
		return k.resourcesListAsTable(ctx, gvk, gvr, namespace, options)
	}
	return k.manager.dynamicClient.Resource(*gvr).Namespace(namespace).List(ctx, options.ListOptions)
}

func (k *Kubernetes) ResourcesGet(ctx context.Context, gvk *schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	gvr, err := k.resourceFor(gvk)
	if err != nil {
		return nil, err
	}

	// If it's a namespaced resource and namespace wasn't provided, try to use the default configured one
	if namespaced, nsErr := k.isNamespaced(gvk); nsErr == nil && namespaced {
		namespace = k.NamespaceOrDefault(namespace)
	}
	return k.manager.dynamicClient.Resource(*gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (k *Kubernetes) ResourcesCreateOrUpdate(ctx context.Context, resource string) ([]*unstructured.Unstructured, error) {
	separator := regexp.MustCompile(`\r?\n---\r?\n`)
	resources := separator.Split(resource, -1)
	var parsedResources []*unstructured.Unstructured
	for _, r := range resources {
		var obj unstructured.Unstructured
		if err := yaml.NewYAMLToJSONDecoder(strings.NewReader(r)).Decode(&obj); err != nil {
			return nil, err
		}
		parsedResources = append(parsedResources, &obj)
	}
	return k.resourcesCreateOrUpdate(ctx, parsedResources)
}

func (k *Kubernetes) ResourcesDelete(ctx context.Context, gvk *schema.GroupVersionKind, namespace, name string) error {
	gvr, err := k.resourceFor(gvk)
	if err != nil {
		return err
	}

	// If it's a namespaced resource and namespace wasn't provided, try to use the default configured one
	if namespaced, nsErr := k.isNamespaced(gvk); nsErr == nil && namespaced {
		namespace = k.NamespaceOrDefault(namespace)
	}
	return k.manager.dynamicClient.Resource(*gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// resourcesListAsTable retrieves a list of resources in a table format.
// It's almost identical to the dynamic.DynamicClient implementation, but it uses a specific Accept header to request the table format.
// dynamic.DynamicClient does not provide a way to set the HTTP header (TODO: create an issue to request this feature)
func (k *Kubernetes) resourcesListAsTable(ctx context.Context, gvk *schema.GroupVersionKind, gvr *schema.GroupVersionResource, namespace string, options ResourceListOptions) (runtime.Unstructured, error) {
	var url []string
	if len(gvr.Group) == 0 {
		url = append(url, "api")
	} else {
		url = append(url, "apis", gvr.Group)
	}
	url = append(url, gvr.Version)
	if len(namespace) > 0 {
		url = append(url, "namespaces", namespace)
	}
	url = append(url, gvr.Resource)
	var table metav1.Table
	err := k.manager.discoveryClient.RESTClient().
		Get().
		SetHeader("Accept", strings.Join([]string{
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName),
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1beta1.SchemeGroupVersion.Version, metav1beta1.GroupName),
			"application/json",
		}, ",")).
		AbsPath(url...).
		SpecificallyVersionedParams(&options.ListOptions, ParameterCodec, schema.GroupVersion{Version: "v1"}).
		Do(ctx).Into(&table)
	if err != nil {
		return nil, err
	}
	// Add metav1.Table apiVersion and kind to the unstructured object (server may not return these fields)
	table.SetGroupVersionKind(metav1.SchemeGroupVersion.WithKind("Table"))
	// Add additional columns for fields that aren't returned by the server
	table.ColumnDefinitions = append([]metav1.TableColumnDefinition{
		{Name: "apiVersion", Type: "string"},
		{Name: "kind", Type: "string"},
	}, table.ColumnDefinitions...)
	for i := range table.Rows {
		row := &table.Rows[i]
		row.Cells = append([]interface{}{
			gvr.GroupVersion().String(),
			gvk.Kind,
		}, row.Cells...)
	}
	unstructuredObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&table)
	return &unstructured.Unstructured{Object: unstructuredObject}, err
}

func (k *Kubernetes) resourcesCreateOrUpdate(ctx context.Context, resources []*unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
	for i, obj := range resources {
		gvk := obj.GroupVersionKind()
		gvr, rErr := k.resourceFor(&gvk)
		if rErr != nil {
			return nil, rErr
		}

		namespace := obj.GetNamespace()
		// If it's a namespaced resource and namespace wasn't provided, try to use the default configured one
		if namespaced, nsErr := k.isNamespaced(&gvk); nsErr == nil && namespaced {
			namespace = k.NamespaceOrDefault(namespace)
		}
		resources[i], rErr = k.manager.dynamicClient.Resource(*gvr).Namespace(namespace).Apply(ctx, obj.GetName(), obj, metav1.ApplyOptions{
			FieldManager: version.BinaryName,
		})
		if rErr != nil {
			return nil, rErr
		}
		// Clear the cache to ensure the next operation is performed on the latest exposed APIs (will change after the CRD creation)
		if gvk.Kind == "CustomResourceDefinition" {
			k.manager.accessControlRESTMapper.Reset()
		}
	}
	return resources, nil
}

func (k *Kubernetes) resourceFor(gvk *schema.GroupVersionKind) (*schema.GroupVersionResource, error) {
	m, err := k.manager.accessControlRESTMapper.RESTMapping(schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
	if err != nil {
		return nil, err
	}
	return &m.Resource, nil
}

func (k *Kubernetes) isNamespaced(gvk *schema.GroupVersionKind) (bool, error) {
	apiResourceList, err := k.manager.discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return false, err
	}
	for _, apiResource := range apiResourceList.APIResources {
		if apiResource.Kind == gvk.Kind {
			return apiResource.Namespaced, nil
		}
	}
	return false, nil
}

func (k *Kubernetes) supportsGroupVersion(groupVersion string) bool {
	if _, err := k.manager.discoveryClient.ServerResourcesForGroupVersion(groupVersion); err != nil {
		return false
	}
	return true
}

func (k *Kubernetes) canIUse(ctx context.Context, gvr *schema.GroupVersionResource, namespace, verb string) bool {
	accessReviews, err := k.manager.accessControlClientSet.SelfSubjectAccessReviews()
	if err != nil {
		return false
	}
	response, err := accessReviews.Create(ctx, &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authv1.ResourceAttributes{
			Namespace: namespace,
			Verb:      verb,
			Group:     gvr.Group,
			Version:   gvr.Version,
			Resource:  gvr.Resource,
		}},
	}, metav1.CreateOptions{})
	if err != nil {
		// TODO: maybe return the error too
		return false
	}
	return response.Status.Allowed
}
