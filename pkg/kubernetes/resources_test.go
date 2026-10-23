package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/clientcmd"
)

type ResourcesTestSuite struct {
	suite.Suite
	mockServer       *test.MockServer
	core             *Core
	paths            []string
	appliedNamespace string
	applyBodyErr     error
}

func (s *ResourcesTestSuite) SetupTest() {
	s.paths = nil
	s.appliedNamespace = ""
	s.applyBodyErr = nil
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler(metav1.APIResourceList{
		GroupVersion: "apiextensions.k8s.io/v1",
		APIResources: []metav1.APIResource{{
			Name:       "customresourcedefinitions",
			Kind:       "CustomResourceDefinition",
			Namespaced: false,
		}},
	}))
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/nodes") || strings.Contains(r.URL.Path, "/pods") || strings.Contains(r.URL.Path, "/customresourcedefinitions") {
			s.paths = append(s.paths, r.URL.Path)
		}

		switch r.URL.Path {
		case "/api/v1/nodes/test-node":
			if r.Method == http.MethodDelete {
				http.NotFound(w, r)
				return
			}
			test.WriteObject(w, &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Node",
				"metadata":   map[string]interface{}{"name": "test-node"},
			}})
		case "/api/v1/nodes":
			test.WriteObject(w, &unstructured.UnstructuredList{
				Object: map[string]interface{}{"apiVersion": "v1", "kind": "List"},
				Items: []unstructured.Unstructured{{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Node",
					"metadata":   map[string]interface{}{"name": "test-node"},
				}}},
			})
		case "/api/v1/namespaces/test-namespace/pods/test-pod":
			if r.Method == http.MethodPatch {
				s.captureAppliedObject(r)
			}
			test.WriteObject(w, &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "test-pod",
					"namespace": "test-namespace",
				},
			}})
		case "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/test.example.com":
			if r.Method == http.MethodPatch {
				s.captureAppliedObject(r)
			}
			test.WriteObject(w, &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apiextensions.k8s.io/v1",
				"kind":       "CustomResourceDefinition",
				"metadata":   map[string]interface{}{"name": "test.example.com"},
			}})
		default:
			if strings.Contains(r.URL.Path, "/nodes") || strings.Contains(r.URL.Path, "/pods") || strings.Contains(r.URL.Path, "/customresourcedefinitions") {
				http.NotFound(w, r)
			}
		}
	}))

	k, err := NewKubernetes(
		s.T().Context(),
		config.BaseDefault(),
		clientcmd.NewDefaultClientConfig(*s.mockServer.Kubeconfig(), &clientcmd.ConfigOverrides{}),
		s.mockServer.Config(),
	)
	s.Require().NoError(err)
	s.T().Cleanup(k.close)
	s.core = NewCore(k)
}

func (s *ResourcesTestSuite) TearDownTest() {
	s.mockServer.Close()
}

func (s *ResourcesTestSuite) TestResourcesGetClusterScopedResourceIgnoresNamespace() {
	node, err := s.core.ResourcesGet(
		s.T().Context(),
		&schema.GroupVersionKind{Version: "v1", Kind: "Node"},
		"test-namespace",
		"test-node",
	)

	s.Require().NoError(err)
	s.Equal("test-node", node.GetName())
	s.assertLastPath("/api/v1/nodes/test-node")
}

func (s *ResourcesTestSuite) TestResourcesListClusterScopedResourceIgnoresNamespace() {
	resources, err := s.core.ResourcesList(
		s.T().Context(),
		&schema.GroupVersionKind{Version: "v1", Kind: "Node"},
		"test-namespace",
		api.ListOptions{},
	)

	s.Require().NoError(err)
	s.NotNil(resources)
	s.assertLastPath("/api/v1/nodes")
}

func (s *ResourcesTestSuite) TestResourcesDeleteClusterScopedResourceIgnoresNamespace() {
	err := s.core.ResourcesDelete(
		s.T().Context(),
		&schema.GroupVersionKind{Version: "v1", Kind: "Node"},
		"test-namespace",
		"test-node",
		nil,
	)

	s.Require().Error(err)
	s.assertLastPath("/api/v1/nodes/test-node")
}

func (s *ResourcesTestSuite) TestResourcesGetNamespacedResourceRetainsNamespace() {
	pod, err := s.core.ResourcesGet(
		s.T().Context(),
		&schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		"test-namespace",
		"test-pod",
	)

	s.Require().NoError(err)
	s.Equal("test-pod", pod.GetName())
	s.assertLastPath("/api/v1/namespaces/test-namespace/pods/test-pod")
}

func (s *ResourcesTestSuite) TestResourcesCreateOrUpdateClusterScopedResourceIgnoresNamespace() {
	_, err := s.core.ResourcesCreateOrUpdate(s.T().Context(), `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: test.example.com
  namespace: test-namespace
spec:
  group: example.com
  names:
    kind: Test
    plural: tests
  scope: Cluster
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
`)

	s.Require().NoError(err)
	s.Require().NoError(s.applyBodyErr)
	s.Empty(s.appliedNamespace)
	s.assertLastPath("/apis/apiextensions.k8s.io/v1/customresourcedefinitions/test.example.com")
}

func (s *ResourcesTestSuite) TestResourcesCreateOrUpdateNamespacedResourceUsesNamespace() {
	_, err := s.core.ResourcesCreateOrUpdate(s.T().Context(), `apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-namespace
`)

	s.Require().NoError(err)
	s.Require().NoError(s.applyBodyErr)
	s.Equal("test-namespace", s.appliedNamespace)
	s.assertLastPath("/api/v1/namespaces/test-namespace/pods/test-pod")
}

func (s *ResourcesTestSuite) captureAppliedObject(r *http.Request) {
	var obj unstructured.Unstructured
	if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
		s.applyBodyErr = err
		return
	}
	s.appliedNamespace = obj.GetNamespace()
}

func (s *ResourcesTestSuite) TestScopeDiscoveryErrorIsReturned() {
	scopeErr := errors.New("scope discovery failed")
	tests := []struct {
		name string
		call func(*Core) error
	}{
		{
			name: "get",
			call: func(core *Core) error {
				_, err := core.ResourcesGet(s.T().Context(), nodeGVKPtr(), "test-namespace", "test-node")
				return err
			},
		},
		{
			name: "list",
			call: func(core *Core) error {
				_, err := core.ResourcesList(s.T().Context(), nodeGVKPtr(), "test-namespace", api.ListOptions{})
				return err
			},
		},
		{
			name: "delete",
			call: func(core *Core) error {
				return core.ResourcesDelete(s.T().Context(), nodeGVKPtr(), "test-namespace", "test-node", nil)
			},
		},
		{
			name: "scale",
			call: func(core *Core) error {
				_, err := core.ResourcesScale(s.T().Context(), nodeGVKPtr(), "test-namespace", "test-node", 1, false)
				return err
			},
		},
		{
			name: "create or update",
			call: func(core *Core) error {
				_, err := core.ResourcesCreateOrUpdate(s.T().Context(), `apiVersion: v1
kind: Node
metadata:
  name: test-node
  namespace: test-namespace
`)
				return err
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := tt.call(newCoreWithDiscoveryError(scopeErr))
			s.ErrorIs(err, scopeErr)
		})
	}
}

func (s *ResourcesTestSuite) assertLastPath(expected string) {
	s.Require().NotEmpty(s.paths)
	s.Equal(expected, s.paths[len(s.paths)-1])
}

func nodeGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Version: "v1", Kind: "Node"}
}

func nodeGVKPtr() *schema.GroupVersionKind {
	gvk := nodeGVK()
	return &gvk
}

type discoveryErrorClient struct {
	discovery.CachedDiscoveryInterface
	err error
}

func (d discoveryErrorClient) ServerResourcesForGroupVersion(string) (*metav1.APIResourceList, error) {
	return nil, d.err
}

type resettableRESTMapper struct {
	meta.RESTMapper
}

func (resettableRESTMapper) Reset() {}

func newCoreWithDiscoveryError(err error) *Core {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(nodeGVK(), meta.RESTScopeRoot)
	k := &Kubernetes{
		discoveryClient: discoveryErrorClient{err: err},
		dynamicClient: fake.NewSimpleDynamicClientWithCustomListKinds(
			runtime.NewScheme(),
			map[schema.GroupVersionResource]string{
				{Version: "v1", Resource: "nodes"}: "NodeList",
			},
		),
		restMapper: resettableRESTMapper{RESTMapper: mapper},
	}
	return NewCore(k)
}

func TestResources(t *testing.T) {
	suite.Run(t, new(ResourcesTestSuite))
}

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
