package kubernetes

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

var (
	podGVR  = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	nodeGVR = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
)

type ProviderInspectorTestSuite struct {
	suite.Suite
	originalInClusterConfig func() (*rest.Config, error)
	mockServer              *test.MockServer
	provider                Provider
	inspector               api.ClusterInspector
}

func (s *ProviderInspectorTestSuite) SetupTest() {
	s.originalInClusterConfig = InClusterConfig
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	s.mockServer.Handle(http.HandlerFunc(inspectorResourceHandler))
	InClusterConfig = func() (*rest.Config, error) {
		return s.mockServer.Config(), nil
	}

	provider, err := NewProvider(s.T().Context(), config.New())
	s.Require().NoError(err)
	s.provider = provider
	s.inspector = &providerInspector{provider: provider}
}

func (s *ProviderInspectorTestSuite) TearDownTest() {
	InClusterConfig = s.originalInClusterConfig
	if s.provider != nil {
		s.provider.Close()
	}
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *ProviderInspectorTestSuite) TestDiscovery() {
	s.Run("returns resources for a group version", func() {
		results := s.inspector.Discovery().ServerResourcesForGroupVersion(s.T().Context(), "v1")

		resources, err := results.Default()

		s.Require().NoError(err)
		s.Equal("v1", resources.GroupVersion)
		s.Len(resources.APIResources, 2)
	})

	s.Run("returns an error for an unavailable group version", func() {
		results := s.inspector.Discovery().ServerResourcesForGroupVersion(s.T().Context(), "missing.example.com/v1")

		resources, err := results.Default()

		s.Error(err)
		s.Nil(resources)
	})
}

func (s *ProviderInspectorTestSuite) TestGroupDiscoveryCancellation() {
	s.mockServer.ResetHandlers()
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" {
			close(started)
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}
	}))
	ctx, cancel := context.WithCancel(s.T().Context())
	defer cancel()
	results := s.inspector.Discovery().(api.AggregateGroupDiscovery).ServerResourcesForGroup(ctx, "flows.netobserv.io")
	finished := make(chan error, 1)
	go func() {
		_, err := results.Default()
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		s.FailNow("group discovery did not start")
	}
	cancel()
	select {
	case err := <-finished:
		s.ErrorIs(err, context.Canceled)
	case <-time.After(time.Second):
		s.Fail("group discovery ignored context cancellation")
	}
}

func (s *ProviderInspectorTestSuite) TestUnstructured() {
	s.Run("gets a namespaced resource", func() {
		results := s.inspector.Unstructured().
			Resource(podGVR).
			Namespace("default").
			Get(s.T().Context(), "demo", metav1.GetOptions{})

		pod, err := results.Default()

		s.Require().NoError(err)
		s.Equal("demo", pod.GetName())
		s.Equal("default", pod.GetNamespace())
	})

	s.Run("lists a cluster scoped resource", func() {
		results := s.inspector.Unstructured().
			Resource(nodeGVR).
			List(s.T().Context(), metav1.ListOptions{})

		nodes, err := results.Default()

		s.Require().NoError(err)
		s.Require().Len(nodes.Items, 1)
		s.Equal("worker-0", nodes.Items[0].GetName())
	})

	s.Run("returns an error for a missing resource", func() {
		results := s.inspector.Unstructured().
			Resource(podGVR).
			Namespace("default").
			Get(s.T().Context(), "missing", metav1.GetOptions{})

		pod, err := results.Default()

		s.Error(err)
		s.Nil(pod)
	})
}

func inspectorResourceHandler(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/api/v1/namespaces/default/pods/demo":
		test.WriteObject(w, &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "demo",
				"namespace": "default",
			},
		}})
	case "/api/v1/namespaces/default/pods/missing":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		test.WriteObject(w, &metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusNotFound,
			Reason:  metav1.StatusReasonNotFound,
			Message: "pods \"missing\" not found",
		})
	case "/api/v1/nodes":
		test.WriteObject(w, &unstructured.UnstructuredList{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "NodeList",
			},
			Items: []unstructured.Unstructured{{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Node",
				"metadata": map[string]interface{}{
					"name": "worker-0",
				},
			}}},
		})
	}
}

func TestProviderInspector(t *testing.T) {
	suite.Run(t, new(ProviderInspectorTestSuite))
}
