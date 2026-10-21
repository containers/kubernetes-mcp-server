package kubernetes

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

type resourcesDiscoverySuite struct {
	suite.Suite
}

// aggregatedDiscovery provides discovery data through the same
// ServerPreferredResources path used by Core.APIResourcesList. The upstream
// client-go implementation removes APIResource entries whose names contain
// a slash before returning the preferred lists.
type aggregatedDiscovery struct {
	discovery.DiscoveryInterface
	groups    *metav1.APIGroupList
	resources map[schema.GroupVersion]*metav1.APIResourceList
}

func (d aggregatedDiscovery) GroupsAndMaybeResources() (
	*metav1.APIGroupList,
	map[schema.GroupVersion]*metav1.APIResourceList,
	map[schema.GroupVersion]error,
	error,
) {
	return d.groups, d.resources, nil, nil
}

func (d aggregatedDiscovery) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	return discovery.ServerPreferredResourcesWithContext(
		context.Background(),
		discovery.ToAggregatedDiscoveryInterfaceWithContext(d),
	)
}

type resourcesDiscoveryClient struct {
	api.KubernetesClient
	discovery discovery.DiscoveryInterface
}

func (c resourcesDiscoveryClient) RawDiscoveryClient() discovery.DiscoveryInterface {
	return c.discovery
}

func (s *resourcesDiscoverySuite) TestAPIResourcesListUsesTopLevelDiscoveryResources() {
	client := resourcesDiscoveryClient{
		discovery: aggregatedDiscovery{
			groups: &metav1.APIGroupList{Groups: []metav1.APIGroup{
				{
					Name:             "",
					Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: "v1", Version: "v1"}},
					PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "v1", Version: "v1"},
				},
				{
					Name:             "apps",
					Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: "apps/v1", Version: "v1"}},
					PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "apps/v1", Version: "v1"},
				},
			}},
			resources: map[schema.GroupVersion]*metav1.APIResourceList{
				schema.GroupVersion{Version: "v1"}: {
					GroupVersion: "v1",
					APIResources: []metav1.APIResource{
						{Name: "pods", Kind: "Pod", Namespaced: true},
						{Name: "pods/status", Kind: "Pod", Namespaced: true},
					},
				},
				schema.GroupVersion{Group: "apps", Version: "v1"}: {
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{Name: "deployments", Kind: "Deployment", Namespaced: true},
						{Name: "deployments/status", Kind: "Deployment", Namespaced: true},
						{Name: "deployments/scale", Group: "autoscaling", Version: "v1", Kind: "Scale", Namespaced: true},
					},
				},
			},
		},
	}

	s.Run("Pod discovery omits pods/status", func() {
		resources, err := NewCore(client).APIResourcesList("pod")
		s.Require().NoError(err)
		s.Equal([]APIResourceInfo{{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       "pods",
			Namespaced: true,
		}}, resources)
	})

	s.Run("Deployment discovery omits status and scale subresources", func() {
		resources, err := NewCore(client).APIResourcesList("deployment")
		s.Require().NoError(err)
		s.Equal([]APIResourceInfo{{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "deployments",
			Namespaced: true,
		}}, resources)
	})

	s.Run("Scale is not exposed as a top-level resource kind", func() {
		resources, err := NewCore(client).APIResourcesList("scale")
		s.Require().NoError(err)
		s.Empty(resources)
	})
}

func TestResourcesDiscovery(t *testing.T) {
	suite.Run(t, new(resourcesDiscoverySuite))
}
