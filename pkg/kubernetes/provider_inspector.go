package kubernetes

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// NewClusterInspector inspects targets with the identity of the current
// request (the derived client).
func NewClusterInspector(provider Provider) api.ClusterInspector {
	return &providerInspector{provider: provider, clients: provider.GetDerivedKubernetes}
}

// NewServerClusterInspector inspects targets with the server's own
// credentials: what toolsets use when they are applied, when no caller
// exists (startup, reload, watch), so that a require_oauth server can still
// read the resources its surface comes from.
func NewServerClusterInspector(provider Provider) api.ClusterInspector {
	return &providerInspector{provider: provider, clients: provider.GetServerKubernetes}
}

type providerInspector struct {
	provider Provider
	clients  func(ctx context.Context, target string) (*Kubernetes, error)
}

type providerNamespaceableResource struct {
	provider  Provider
	clients   func(ctx context.Context, target string) (*Kubernetes, error)
	resource  schema.GroupVersionResource
	namespace string
}

// clientFor returns the client source of the inspector: the one it was
// built with, or the derived (request identity) client for an inspector
// constructed without one.
func clientFor(provider Provider, clients func(ctx context.Context, target string) (*Kubernetes, error)) func(ctx context.Context, target string) (*Kubernetes, error) {
	if clients != nil {
		return clients
	}
	return provider.GetDerivedKubernetes
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
		clients:  p.clients,
		resource: resource,
	}
}

func (p *providerInspector) ServerResourcesForGroupVersion(
	ctx context.Context,
	groupVersion string,
) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, api.TargetProvider(p.provider), func(ctx context.Context, target string) (*metav1.APIResourceList, error) {
		client, err := clientFor(p.provider, p.clients)(ctx, target)
		if err != nil {
			return nil, err
		}

		// client-go discovery does not accept a context, so an Any/All short-circuit
		// cannot cancel a ServerResourcesForGroupVersion request already in flight.
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
			client, err := clientFor(p.provider, p.clients)(ctx, target)
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
			client, err := clientFor(p.provider, p.clients)(ctx, target)
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
		clients:   p.clients,
		resource:  p.resource,
		namespace: namespace,
	}
}
