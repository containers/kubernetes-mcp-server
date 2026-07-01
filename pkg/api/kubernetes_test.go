package api

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
)

type HasGVKsTestSuite struct {
	suite.Suite
	mockServer *test.MockServer
}

func TestHasGVKs(t *testing.T) {
	suite.Run(t, new(HasGVKsTestSuite))
}

func (s *HasGVKsTestSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
}

func (s *HasGVKsTestSuite) TearDownTest() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *HasGVKsTestSuite) discoveryClient() discovery.DiscoveryInterface {
	return discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config())
}

func (s *HasGVKsTestSuite) TestAllGVKsExist() {
	s.Run("returns true when all GVKs exist", func() {
		handler := test.NewDiscoveryClientHandler(
			metav1.APIResourceList{
				GroupVersion: "project.openshift.io/v1",
				APIResources: []metav1.APIResource{
					{Name: "projects", Kind: "Project"},
				},
			},
		)
		s.mockServer.Handle(handler)

		gvks := []schema.GroupVersionKind{
			{Group: "", Version: "v1", Kind: "Pod"},                         // From default handler
			{Group: "project.openshift.io", Version: "v1", Kind: "Project"}, // From added handler
		}

		hasGVKs, err := HasGVKs(s.discoveryClient(), gvks)
		s.NoError(err)
		s.True(hasGVKs)
	})
}

func (s *HasGVKsTestSuite) TestGroupVersionDoesNotExist() {
	s.Run("returns false with no error when GroupVersion returns 404", func() {
		// Default handler doesn't include project.openshift.io
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)

		gvks := []schema.GroupVersionKind{
			{Group: "project.openshift.io", Version: "v1", Kind: "Project"},
		}

		hasGVKs, err := HasGVKs(s.discoveryClient(), gvks)
		s.NoError(err, "404 should not be returned as an error")
		s.False(hasGVKs, "should return false when GV doesn't exist")
	})
}

func (s *HasGVKsTestSuite) TestMemCacheGroupVersionDoesNotExist() {
	s.Run("returns false with no error for missing GroupVersion via memcache client", func() {
		// Production derives discovery from a memory-cached client, which returns
		// memory.ErrCacheNotFound (a plain error, not a StatusError with IsNotFound)
		// for an absent GroupVersion. This guards the errors.Is(err, ErrCacheNotFound)
		// branch in HasGVKs, which the raw-discovery cases above do not exercise.
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)

		cached := memory.NewMemCacheClient(s.discoveryClient())

		gvks := []schema.GroupVersionKind{
			{Group: "project.openshift.io", Version: "v1", Kind: "Project"},
		}

		hasGVKs, err := HasGVKs(cached, gvks)
		s.NoError(err, "missing GroupVersion via memcache should map to (false, nil)")
		s.False(hasGVKs, "should return false when the GroupVersion is absent from the memcache")
	})
}

func (s *HasGVKsTestSuite) TestKindDoesNotExist() {
	s.Run("returns false with no error when Kind is not in the resource list", func() {
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)

		gvks := []schema.GroupVersionKind{
			{Group: "", Version: "v1", Kind: "ConfigMap"}, // v1 exists but ConfigMap is not in default handler
		}

		hasGVKs, err := HasGVKs(s.discoveryClient(), gvks)
		s.NoError(err)
		s.False(hasGVKs, "should return false when Kind doesn't exist in GV")
	})
}

func (s *HasGVKsTestSuite) TestProperSubsetExists() {
	s.Run("returns false when only some GVKs exist", func() {
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)

		gvks := []schema.GroupVersionKind{
			{Group: "", Version: "v1", Kind: "Pod"},                         // Exists in default handler
			{Group: "project.openshift.io", Version: "v1", Kind: "Project"}, // Doesn't exist
		}

		hasGVKs, err := HasGVKs(s.discoveryClient(), gvks)
		s.NoError(err)
		s.False(hasGVKs, "should return false when not all GVKs exist")
	})
}

func (s *HasGVKsTestSuite) TestDiscoveryError() {
	s.Run("returns error when discovery fails with non-404 error", func() {
		// Don't set up any handler - server will return empty response causing JSON parse error
		s.mockServer.ResetHandlers()

		gvks := []schema.GroupVersionKind{
			{Group: "", Version: "v1", Kind: "Pod"},
		}

		hasGVKs, err := HasGVKs(s.discoveryClient(), gvks)
		s.Error(err, "should return error for non-404 discovery failures")
		s.False(hasGVKs)
	})
}

func (s *HasGVKsTestSuite) TestEmptyGVKList() {
	s.Run("returns true for empty GVK list", func() {
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)

		hasGVKs, err := HasGVKs(s.discoveryClient(), []schema.GroupVersionKind{})
		s.NoError(err)
		s.True(hasGVKs, "should return true for empty GVK list")
	})
}

func (s *HasGVKsTestSuite) TestMultipleGVKsInSameGroupVersion() {
	s.Run("returns true when multiple GVKs in same GV all exist", func() {
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)

		gvks := []schema.GroupVersionKind{
			{Group: "", Version: "v1", Kind: "Pod"}, // Both exist in default handler
			{Group: "", Version: "v1", Kind: "Node"},
		}

		hasGVKs, err := HasGVKs(s.discoveryClient(), gvks)
		s.NoError(err)
		s.True(hasGVKs)
	})

	s.Run("returns false when one of multiple GVKs in same GV doesn't exist", func() {
		// Reset handlers and create fresh discovery client to avoid cache issues
		s.mockServer.ResetHandlers()
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)

		gvks := []schema.GroupVersionKind{
			{Group: "", Version: "v1", Kind: "Pod"},       // Exists
			{Group: "", Version: "v1", Kind: "ConfigMap"}, // Doesn't exist
		}

		hasGVKs, err := HasGVKs(s.discoveryClient(), gvks)
		s.NoError(err)
		s.False(hasGVKs)
	})
}

func (s *HasGVKsTestSuite) TestRealWorldOpenShiftScenario() {
	s.Run("OpenShift Project GVK detection", func() {
		// Simulate non-OpenShift cluster (no project.openshift.io)
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)

		gvks := []schema.GroupVersionKind{
			{Group: "project.openshift.io", Version: "v1", Kind: "Project"},
		}

		hasGVKs, err := HasGVKs(s.discoveryClient(), gvks)
		s.NoError(err, "missing GV should return false, not error")
		s.False(hasGVKs, "non-OpenShift cluster should not have Project GVK")

		// Simulate OpenShift cluster
		s.mockServer.ResetHandlers()
		openshiftHandler := test.NewInOpenShiftHandler()
		s.mockServer.Handle(openshiftHandler)

		hasGVKs, err = HasGVKs(s.discoveryClient(), gvks)
		s.NoError(err)
		s.True(hasGVKs, "OpenShift cluster should have Project GVK")
	})
}
