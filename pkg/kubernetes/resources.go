package kubernetes

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
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

func (c *Core) ResourcesList(ctx context.Context, gvk *schema.GroupVersionKind, namespace string, options api.ListOptions) (runtime.Unstructured, error) {
	gvr, err := c.resourceFor(gvk)
	if err != nil {
		return nil, err
	}

	// Check if operation is allowed for all namespaces (applicable for namespaced resources)
	isNamespaced, _ := c.isNamespaced(gvk)
	if isNamespaced && !c.canIUse(ctx, gvr, namespace, "list") && namespace == "" {
		namespace = c.NamespaceOrDefault("")
	}
	if options.AsTable {
		return c.resourcesListAsTable(ctx, gvk, gvr, namespace, options)
	}
	return c.DynamicClient().Resource(*gvr).Namespace(namespace).List(ctx, options.ListOptions)
}

func (c *Core) ResourcesGet(ctx context.Context, gvk *schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	gvr, err := c.resourceFor(gvk)
	if err != nil {
		return nil, err
	}

	// If it's a namespaced resource and namespace wasn't provided, try to use the default configured one
	if namespaced, nsErr := c.isNamespaced(gvk); nsErr == nil && namespaced {
		namespace = c.NamespaceOrDefault(namespace)
	}
	return c.DynamicClient().Resource(*gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Core) ResourcesCreateOrUpdate(ctx context.Context, resource string) ([]*unstructured.Unstructured, error) {
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
	return c.resourcesCreateOrUpdate(ctx, parsedResources)
}

func (c *Core) ResourcesDelete(ctx context.Context, gvk *schema.GroupVersionKind, namespace, name string, gracePeriodSeconds *int64) error {
	gvr, err := c.resourceFor(gvk)
	if err != nil {
		return err
	}

	// If it's a namespaced resource and namespace wasn't provided, try to use the default configured one
	if namespaced, nsErr := c.isNamespaced(gvk); nsErr == nil && namespaced {
		namespace = c.NamespaceOrDefault(namespace)
	}
	return c.DynamicClient().Resource(*gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: gracePeriodSeconds,
	})
}

func (c *Core) ResourcesScale(
	ctx context.Context,
	gvk *schema.GroupVersionKind,
	namespace, name string,
	desiredScale int64,
	shouldScale bool,
) (*unstructured.Unstructured, error) {
	gvr, err := c.resourceFor(gvk)
	if err != nil {
		return nil, err
	}

	var resourceClient dynamic.ResourceInterface

	if namespaced, nsErr := c.isNamespaced(gvk); nsErr == nil && namespaced {
		resourceClient = c.
			DynamicClient().
			Resource(*gvr).
			Namespace(c.NamespaceOrDefault(namespace))
	} else {
		resourceClient = c.DynamicClient().Resource(*gvr)
	}

	scale, err := resourceClient.Get(ctx, name, metav1.GetOptions{}, "scale")
	if err != nil {
		return nil, err
	}

	if shouldScale {
		if err := unstructured.SetNestedField(scale.Object, desiredScale, "spec", "replicas"); err != nil {
			return scale, fmt.Errorf("failed to set .spec.replicas on scale object %v: %w", scale, err)
		}

		scale, err = resourceClient.Update(ctx, scale, metav1.UpdateOptions{}, "scale")
		if err != nil {
			return scale, fmt.Errorf("failed to update scale: %w", err)
		}
	}

	return scale, nil
}

// resourcesListAsTable retrieves a list of resources in a table format.
// It's almost identical to the dynamic.DynamicClient implementation, but it uses a specific Accept header to request the table format.
// dynamic.DynamicClient does not provide a way to set the HTTP header (TODO: create an issue to request this feature)
func (c *Core) resourcesListAsTable(ctx context.Context, gvk *schema.GroupVersionKind, gvr *schema.GroupVersionResource, namespace string, options api.ListOptions) (runtime.Unstructured, error) {
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
	err := c.CoreV1().RESTClient().
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

func (c *Core) resourcesCreateOrUpdate(ctx context.Context, resources []*unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
	for i, obj := range resources {
		gvk := obj.GroupVersionKind()
		gvr, rErr := c.resourceFor(&gvk)
		if rErr != nil {
			return nil, rErr
		}

		namespace := obj.GetNamespace()
		// If it's a namespaced resource and namespace wasn't provided, try to use the default configured one
		if namespaced, nsErr := c.isNamespaced(&gvk); nsErr == nil && namespaced {
			namespace = c.NamespaceOrDefault(namespace)
		}
		resources[i], rErr = c.DynamicClient().Resource(*gvr).Namespace(namespace).Apply(ctx, obj.GetName(), obj, metav1.ApplyOptions{
			FieldManager: version.BinaryName,
		})
		if rErr != nil {
			return nil, rErr
		}
		// Clear the cache to ensure the next operation is performed on the latest exposed APIs (will change after the CRD creation)
		if gvk.Kind == "CustomResourceDefinition" {
			c.RESTMapper().Reset()
		}
	}
	return resources, nil
}

func (c *Core) resourceFor(gvk *schema.GroupVersionKind) (*schema.GroupVersionResource, error) {
	m, err := c.RESTMapper().RESTMapping(schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
	if err != nil {
		return nil, err
	}
	return &m.Resource, nil
}

func (c *Core) isNamespaced(gvk *schema.GroupVersionKind) (bool, error) {
	apiResourceList, err := c.DiscoveryClient().ServerResourcesForGroupVersion(gvk.GroupVersion().String())
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

func (c *Core) supportsGroupVersion(groupVersion string) bool {
	if _, err := c.DiscoveryClient().ServerResourcesForGroupVersion(groupVersion); err != nil {
		return false
	}
	return true
}

func (c *Core) canIUse(ctx context.Context, gvr *schema.GroupVersionResource, namespace, verb string) bool {
	accessReviews := c.AuthorizationV1().SelfSubjectAccessReviews()
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
