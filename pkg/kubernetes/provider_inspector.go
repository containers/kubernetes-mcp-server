package kubernetes

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func NewClusterInspector(provider Provider) api.ClusterInspector {
	return &providerInspector{provider: provider}
}

type providerInspector struct {
	provider Provider
}

type providerNamespaceableResource struct {
	provider  Provider
	resource  schema.GroupVersionResource
	namespace string
}

var _ api.ClusterInspector = &providerInspector{}
var _ api.AggregateDiscovery = &providerInspector{}
var _ api.AggregateUnstructured = &providerInspector{}

var _ api.AggregateNamespaceableResourceInterface = &providerNamespaceableResource{}

func (p *providerInspector) Discovery() api.AggregateDiscovery {
	return p
}

func (p *providerInspector) Unstructured() api.AggregateUnstructured {
	return p
}

func (p *providerInspector) Resource(resource schema.GroupVersionResource) api.AggregateNamespaceableResourceInterface {
	return &providerNamespaceableResource{
		provider: p.provider,
		resource: resource,
	}
}

func (p *providerInspector) ServerResourcesForGroupVersion(
	ctx context.Context,
	groupVersion string,
) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, api.TargetProvider(p.provider), func(ctx context.Context, target string) (*metav1.APIResourceList, error) {
		client, err := p.provider.GetDerivedKubernetes(ctx, target)
		if err != nil {
			return nil, err
		}

		return client.discoveryClient.ServerResourcesForGroupVersion(groupVersion)
	})
}

func (p *providerNamespaceableResource) Get(
	ctx context.Context,
	name string,
	options metav1.GetOptions,
	subresources ...string,
) api.Results[*unstructured.Unstructured] {
	return api.NewResults(
		ctx,
		p.provider,
		func(ctx context.Context, target string) (*unstructured.Unstructured, error) {
			client, err := p.provider.GetDerivedKubernetes(ctx, target)
			if err != nil {
				return nil, err
			}

			return client.dynamicClient.Resource(p.resource).Namespace(p.namespace).Get(ctx, name, options, subresources...)
		},
	)
}

func (p *providerNamespaceableResource) List(
	ctx context.Context,
	opts metav1.ListOptions,
) api.Results[*unstructured.UnstructuredList] {
	return api.NewResults(
		ctx,
		p.provider,
		func(ctx context.Context, target string) (*unstructured.UnstructuredList, error) {
			client, err := p.provider.GetDerivedKubernetes(ctx, target)
			if err != nil {
				return nil, err
			}

			return client.dynamicClient.Resource(p.resource).Namespace(p.namespace).List(ctx, opts)
		},
	)
}

func (p *providerNamespaceableResource) Namespace(namespace string) api.AggregateResourceInterface {
	return &providerNamespaceableResource{
		provider:  p.provider,
		resource:  p.resource,
		namespace: namespace,
	}
}
