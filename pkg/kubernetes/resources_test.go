package kubernetes

import (
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
	mockServer *test.MockServer
	core       *Core
	paths      []string
}

func (s *ResourcesTestSuite) SetupTest() {
	s.paths = nil
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
			test.WriteObject(w, &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "test-pod",
					"namespace": "test-namespace",
				},
			}})
		case "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/test.example.com":
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
		config.New(),
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
	s.assertLastPath("/apis/apiextensions.k8s.io/v1/customresourcedefinitions/test.example.com")
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
