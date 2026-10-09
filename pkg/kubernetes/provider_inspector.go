package kubernetes

import (
	"context"
	"errors"

	"k8s.io/client-go/discovery"

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
var _ api.AggregateGroupDiscovery = &providerInspector{}
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

		// client-go discovery does not accept a context, so an Any/All short-circuit
		// cannot cancel a ServerResourcesForGroupVersion request already in flight.
		return client.discoveryClient.ServerResourcesForGroupVersion(groupVersion)
	})
}

// ServerResourcesForGroup inspects every served version on each target.
func (p *providerInspector) ServerResourcesForGroup(ctx context.Context, group string) api.Results[[]*metav1.APIResourceList] {
	return api.NewResults(ctx, api.TargetProvider(p.provider), func(ctx context.Context, target string) ([]*metav1.APIResourceList, error) {
		client, err := p.provider.GetDerivedKubernetes(ctx, target)
		if err != nil {
			return nil, err
		}
		return serverResourcesForGroup(ctx, client.discoveryClient, group)
	})
}

func serverResourcesForGroup(ctx context.Context, dc discovery.DiscoveryInterface, requestedGroup string) ([]*metav1.APIResourceList, error) {
	var groups *metav1.APIGroupList
	var resources map[schema.GroupVersion]*metav1.APIResourceList
	var discoveryErr error
	client := discovery.ToDiscoveryInterfaceWithContext(dc)
	aggregated, ok := dc.(discovery.AggregatedDiscoveryInterfaceWithContext)
	if !ok {
		if legacy, supportsAggregated := dc.(discovery.AggregatedDiscoveryInterface); supportsAggregated {
			aggregated = discovery.ToAggregatedDiscoveryInterfaceWithContext(legacy)
			ok = true
		}
	}
	if ok {
		var failedVersions map[schema.GroupVersion]error
		groups, resources, failedVersions, discoveryErr = aggregated.GroupsAndMaybeResourcesWithContext(ctx)
		// ServerGroups alone discards stale-version errors. Preserve relevant
		// failures so uncertain discovery is not mistaken for confirmed absence.
		for version, err := range failedVersions {
			if version.Group == requestedGroup {
				discoveryErr = errors.Join(discoveryErr, err)
			}
		}
	} else {
		groups, discoveryErr = client.ServerGroupsWithContext(ctx)
	}
	if groups == nil {
		return nil, errors.Join(discoveryErr, errors.New("API discovery returned no group list"))
	}
	var lists []*metav1.APIResourceList
	for _, group := range groups.Groups {
		if group.Name != requestedGroup {
			continue
		}
		for _, version := range group.Versions {
			var list *metav1.APIResourceList
			var err error
			if resources != nil {
				list = resources[schema.GroupVersion{Group: group.Name, Version: version.Version}]
			} else {
				list, err = client.ServerResourcesForGroupVersionWithContext(ctx, version.GroupVersion)
			}
			if err != nil {
				if !api.IsNotFound(err) {
					discoveryErr = errors.Join(discoveryErr, err)
				}
				continue
			}
			if list == nil {
				discoveryErr = errors.Join(discoveryErr, errors.New("API discovery returned no resource list"))
				continue
			}
			lists = append(lists, list)
		}
	}
	return lists, discoveryErr
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
