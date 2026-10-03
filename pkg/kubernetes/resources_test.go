package kubernetes

import (
	"net/http"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
		&config.StaticConfig{},
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

func (s *ResourcesTestSuite) assertLastPath(expected string) {
	s.Require().NotEmpty(s.paths)
	s.Equal(expected, s.paths[len(s.paths)-1])
}

func TestResources(t *testing.T) {
	suite.Run(t, new(ResourcesTestSuite))
}
