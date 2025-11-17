package kubernetes

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

const (
	verbList             = "list"
	verbDeleteCollection = "deletecollection"
	verbWatch            = "watch"

	defaultNs = ""
)

// AccessControlDynamicClient wraps dynamic.Interface and enforces access control
type AccessControlDynamicClient struct {
	delegate dynamic.Interface
	k8s      *Kubernetes
}

var _ dynamic.Interface = &AccessControlDynamicClient{}

func NewAccessControlDynamicClient(k8s *Kubernetes) dynamic.Interface {
	return &AccessControlDynamicClient{
		delegate: k8s.manager.dynamicClient,
		k8s:      k8s,
	}
}

// Resource enforces proper access control to the resource
func (a *AccessControlDynamicClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	gvk, err := a.k8s.manager.accessControlRESTMapper.KindFor(gvr)
	if err != nil {
		return &deniedNamespaceableResource{err: err}
	}

	return &AccessControlNamespaceableResource{
		gvr:      gvr,
		gvk:      gvk,
		delegate: a.delegate.Resource(gvr),
		k8s:      a.k8s,
	}
}

type AccessControlNamespaceableResource struct {
	gvr      schema.GroupVersionResource
	gvk      schema.GroupVersionKind
	delegate dynamic.NamespaceableResourceInterface
	k8s      *Kubernetes
}

var _ dynamic.NamespaceableResourceInterface = &AccessControlNamespaceableResource{}

func (a *AccessControlNamespaceableResource) Create(
	ctx context.Context,
	obj *unstructured.Unstructured,
	opts metav1.CreateOptions,
	subresources ...string,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.Namespace(defaultNs).Create(ctx, obj, opts, subresources...)
}

func (a *AccessControlNamespaceableResource) Update(
	ctx context.Context,
	obj *unstructured.Unstructured,
	opts metav1.UpdateOptions,
	subresources ...string,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.Namespace(defaultNs).Update(ctx, obj, opts, subresources...)
}

func (a *AccessControlNamespaceableResource) UpdateStatus(
	ctx context.Context,
	obj *unstructured.Unstructured,
	opts metav1.UpdateOptions,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.Namespace(defaultNs).UpdateStatus(ctx, obj, opts)
}

func (a *AccessControlNamespaceableResource) Delete(
	ctx context.Context,
	name string,
	opts metav1.DeleteOptions,
	subresources ...string,
) error {
	return a.Namespace(defaultNs).Delete(ctx, name, opts, subresources...)
}

func (a *AccessControlNamespaceableResource) DeleteCollection(
	ctx context.Context,
	opts metav1.DeleteOptions,
	listOpts metav1.ListOptions,
) error {
	return a.Namespace(defaultNs).DeleteCollection(ctx, opts, listOpts)
}

func (a *AccessControlNamespaceableResource) Get(
	ctx context.Context,
	name string,
	opts metav1.GetOptions,
	subresources ...string,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.Namespace(defaultNs).Get(ctx, name, opts, subresources...)
}

func (a *AccessControlNamespaceableResource) List(
	ctx context.Context,
	opts metav1.ListOptions,
) (
	*unstructured.UnstructuredList,
	error,
) {
	return a.Namespace(defaultNs).List(ctx, opts)
}

func (a *AccessControlNamespaceableResource) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.Namespace(defaultNs).Watch(ctx, opts)
}

func (a *AccessControlNamespaceableResource) Patch(
	ctx context.Context,
	name string,
	pt types.PatchType,
	data []byte,
	opts metav1.PatchOptions,
	subresources ...string,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.Namespace(defaultNs).Patch(ctx, name, pt, data, opts, subresources...)
}

func (a *AccessControlNamespaceableResource) Apply(
	ctx context.Context,
	name string,
	obj *unstructured.Unstructured,
	opts metav1.ApplyOptions,
	subresources ...string,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.Namespace(defaultNs).Apply(ctx, name, obj, opts, subresources...)
}

func (a *AccessControlNamespaceableResource) ApplyStatus(
	ctx context.Context,
	name string,
	obj *unstructured.Unstructured,
	opts metav1.ApplyOptions,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.Namespace(defaultNs).ApplyStatus(ctx, name, obj, opts)
}

func (a *AccessControlNamespaceableResource) Namespace(namespace string) dynamic.ResourceInterface {
	return &AccessControlResource{
		gvr:       a.gvr,
		gvk:       a.gvk,
		delegate:  a.delegate,
		k8s:       a.k8s,
		namespace: namespace,
	}
}

type AccessControlResource struct {
	gvr       schema.GroupVersionResource
	gvk       schema.GroupVersionKind
	delegate  dynamic.NamespaceableResourceInterface
	k8s       *Kubernetes
	namespace string
}

var _ dynamic.ResourceInterface = &AccessControlResource{}

func (a *AccessControlResource) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	isNamespaced, _ := a.k8s.isNamespaced(&a.gvk)

	// namespace fallback logic (in case listing across all ns fails, fallback to default ns)
	if isNamespaced && !a.k8s.canIUse(ctx, &a.gvr, a.namespace, verbList) && a.namespace == "" {
		a.namespace = a.k8s.manager.configuredNamespace()
	}

	return a.getDelegate().List(ctx, opts)
}

func (a *AccessControlResource) Get(
	ctx context.Context,
	name string,
	opts metav1.GetOptions,
	subresources ...string,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.getDelegate().Get(ctx, name, opts, subresources...)
}

func (a *AccessControlResource) Create(
	ctx context.Context,
	obj *unstructured.Unstructured,
	opts metav1.CreateOptions,
	subresources ...string,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.getDelegate().Create(ctx, obj, opts, subresources...)
}

func (a *AccessControlResource) Update(
	ctx context.Context,
	obj *unstructured.Unstructured,
	opts metav1.UpdateOptions,
	subresources ...string,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.getDelegate().Update(ctx, obj, opts, subresources...)
}

func (a *AccessControlResource) UpdateStatus(
	ctx context.Context,
	obj *unstructured.Unstructured,
	opts metav1.UpdateOptions,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.getDelegate().UpdateStatus(ctx, obj, opts)
}

func (a *AccessControlResource) Delete(ctx context.Context, name string, opts metav1.DeleteOptions, subresources ...string) error {
	return a.getDelegate().Delete(ctx, name, opts, subresources...)
}

func (a *AccessControlResource) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	isNamespaced, _ := a.k8s.isNamespaced(&a.gvk)

	// namespace fallback logic (in case deleting across all ns fails, fallback to default ns)
	if isNamespaced && !a.k8s.canIUse(ctx, &a.gvr, a.namespace, verbDeleteCollection) && a.namespace == "" {
		a.namespace = a.k8s.manager.configuredNamespace()
	}

	return a.getDelegate().DeleteCollection(ctx, opts, listOpts)
}

func (a *AccessControlResource) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	isNamespaced, _ := a.k8s.isNamespaced(&a.gvk)

	// namespace fallback logic (in case watching across all ns fails, fallback to default ns)
	if isNamespaced && !a.k8s.canIUse(ctx, &a.gvr, a.namespace, verbWatch) && a.namespace == "" {
		a.namespace = a.k8s.manager.configuredNamespace()
	}

	return a.getDelegate().Watch(ctx, opts)
}

func (a *AccessControlResource) Patch(
	ctx context.Context,
	name string,
	pt types.PatchType,
	data []byte,
	opts metav1.PatchOptions,
	subresources ...string,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.getDelegate().Patch(ctx, name, pt, data, opts, subresources...)
}

func (a *AccessControlResource) Apply(
	ctx context.Context,
	name string,
	obj *unstructured.Unstructured,
	opts metav1.ApplyOptions,
	subresources ...string,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.getDelegate().Apply(ctx, name, obj, opts, subresources...)
}

func (a *AccessControlResource) ApplyStatus(
	ctx context.Context,
	name string,
	obj *unstructured.Unstructured,
	opts metav1.ApplyOptions,
) (
	*unstructured.Unstructured,
	error,
) {
	return a.getDelegate().ApplyStatus(ctx, name, obj, opts)
}

func (a *AccessControlResource) getDelegate() dynamic.ResourceInterface {
	isNamespaced, _ := a.k8s.isNamespaced(&a.gvk)

	if isNamespaced {
		ns := a.k8s.NamespaceOrDefault(a.namespace)
		return a.delegate.Namespace(ns)
	}

	// cluster resource, this must not have .Namespace() called
	return a.delegate
}

type deniedNamespaceableResource struct {
	err error
}

var _ dynamic.NamespaceableResourceInterface = (*deniedNamespaceableResource)(nil)

func (d *deniedNamespaceableResource) Namespace(string) dynamic.ResourceInterface {
	return &deniedResource{err: d.err}
}

func (d *deniedNamespaceableResource) Create(context.Context, *unstructured.Unstructured, metav1.CreateOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedNamespaceableResource) Update(context.Context, *unstructured.Unstructured, metav1.UpdateOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedNamespaceableResource) UpdateStatus(context.Context, *unstructured.Unstructured, metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedNamespaceableResource) Delete(context.Context, string, metav1.DeleteOptions, ...string) error {
	return d.err
}

func (d *deniedNamespaceableResource) DeleteCollection(context.Context, metav1.DeleteOptions, metav1.ListOptions) error {
	return d.err
}

func (d *deniedNamespaceableResource) Get(context.Context, string, metav1.GetOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedNamespaceableResource) List(context.Context, metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return nil, d.err
}

func (d *deniedNamespaceableResource) Watch(context.Context, metav1.ListOptions) (watch.Interface, error) {
	return nil, d.err
}

func (d *deniedNamespaceableResource) Patch(context.Context, string, types.PatchType, []byte, metav1.PatchOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedNamespaceableResource) Apply(context.Context, string, *unstructured.Unstructured, metav1.ApplyOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedNamespaceableResource) ApplyStatus(context.Context, string, *unstructured.Unstructured, metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return nil, d.err
}

type deniedResource struct {
	err error
}

var _ dynamic.ResourceInterface = (*deniedResource)(nil)

func (d *deniedResource) Create(context.Context, *unstructured.Unstructured, metav1.CreateOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedResource) Update(context.Context, *unstructured.Unstructured, metav1.UpdateOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedResource) UpdateStatus(context.Context, *unstructured.Unstructured, metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedResource) Delete(context.Context, string, metav1.DeleteOptions, ...string) error {
	return d.err
}

func (d *deniedResource) DeleteCollection(context.Context, metav1.DeleteOptions, metav1.ListOptions) error {
	return d.err
}

func (d *deniedResource) Get(context.Context, string, metav1.GetOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedResource) List(context.Context, metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return nil, d.err
}

func (d *deniedResource) Watch(context.Context, metav1.ListOptions) (watch.Interface, error) {
	return nil, d.err
}

func (d *deniedResource) Patch(context.Context, string, types.PatchType, []byte, metav1.PatchOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedResource) Apply(context.Context, string, *unstructured.Unstructured, metav1.ApplyOptions, ...string) (*unstructured.Unstructured, error) {
	return nil, d.err
}

func (d *deniedResource) ApplyStatus(context.Context, string, *unstructured.Unstructured, metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return nil, d.err
}
