package kubernetes

import (
	"errors"
	"slices"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type ProviderSingleTestSuite struct {
	BaseProviderSuite
	originalIsInClusterConfig func() (*rest.Config, error)
	mockServer                *test.MockServer
	provider                  Provider
}

func (s *ProviderSingleTestSuite) SetupTest() {
	// Single cluster provider is used when in-cluster or when the multi-cluster feature is disabled.
	// For this test suite we simulate an in-cluster deployment backed by a mock API server.
	s.originalIsInClusterConfig = InClusterConfig
	s.mockServer = test.NewMockServer()
	// Default discovery simulates a vanilla (non-OpenShift) cluster.
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	InClusterConfig = func() (*rest.Config, error) {
		return s.mockServer.Config(), nil
	}
	provider, err := NewProvider(s.T().Context(), config.New())
	s.Require().NoError(err, "Expected no error creating provider")
	s.provider = provider
}

func (s *ProviderSingleTestSuite) TearDownTest() {
	InClusterConfig = s.originalIsInClusterConfig
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *ProviderSingleTestSuite) TestType() {
	s.IsType(&singleClusterProvider{}, s.provider)
}

func (s *ProviderSingleTestSuite) TestWithOpenShiftCluster() {
	// Serve the OpenShift discovery document so the Project GVK is present.
	s.mockServer.ResetHandlers()
	s.mockServer.Handle(test.NewInOpenShiftHandler())
	s.Run("has OpenShift Project GVK", func() {
		resources, err := NewClusterInspector(s.provider).Discovery().ServerResourcesForGroupVersion(s.T().Context(), "project.openshift.io/v1").Default()
		s.Require().NoError(err)
		s.True(slices.ContainsFunc(resources.APIResources, func(resource metav1.APIResource) bool {
			return resource.Kind == "Project"
		}), "Expected OpenShift Project in discovery")
	})
}

func (s *ProviderSingleTestSuite) TestWithNonOpenShiftGVK() {
	s.Run("does not have non-existent GVK", func() {
		// Default (non-OpenShift) discovery returns a 404 for the missing GroupVersion.
		_, err := NewClusterInspector(s.provider).Discovery().ServerResourcesForGroupVersion(s.T().Context(), "nonexistent.example.com/v1").Default()
		s.True(api.IsNotFound(err), "Expected provider to report no nonexistent group/version: %v", err)
	})
}

func (s *ProviderSingleTestSuite) TestGetTargets() {
	s.Run("GetTargets returns single empty target", func() {
		targets, err := s.provider.GetTargets(s.T().Context())
		s.Require().NoError(err, "Expected no error from GetTargets")
		s.Len(targets, 1, "Expected 1 targets from GetTargets")
		s.Contains(targets, "", "Expected empty target from GetTargets")
	})
}

func (s *ProviderSingleTestSuite) TestGetDerivedKubernetes() {
	s.Run("GetDerivedKubernetes returns Kubernetes for empty target", func() {
		k8s, err := s.provider.GetDerivedKubernetes(s.T().Context(), "")
		s.Require().NoError(err, "Expected no error from GetDerivedKubernetes with empty target")
		s.NotNil(k8s, "Expected Kubernetes from GetDerivedKubernetes with empty target")
	})
	s.Run("GetDerivedKubernetes returns error for non-empty target", func() {
		k8s, err := s.provider.GetDerivedKubernetes(s.T().Context(), "non-empty-target")
		s.Require().Error(err, "Expected error from GetDerivedKubernetes with non-empty target")
		s.ErrorContains(err, "unable to get manager for other context/cluster with in-cluster strategy", "Expected error about trying to get other cluster")
		s.Nil(k8s, "Expected no Kubernetes from GetDerivedKubernetes with non-empty target")
	})
}

func (s *ProviderSingleTestSuite) TestGetDefaultTarget() {
	s.Run("GetDefaultTarget returns empty string", func() {
		s.Empty(s.provider.GetDefaultTarget(), "Expected empty string as default target")
	})
}

func (s *ProviderSingleTestSuite) TestGetTargetParameterName() {
	s.Empty(s.provider.GetTargetParameterName(), "Expected empty string as target parameter name")
}

func (s *ProviderSingleTestSuite) TestReloadConfigDoesNotRebuild() {
	k8s, err := s.provider.GetDerivedKubernetes(s.T().Context(), "")
	s.Require().NoError(err)
	s.Require().NotNil(k8s)

	InClusterConfig = func() (*rest.Config, error) {
		return nil, errors.New("in-cluster config unavailable")
	}
	s.Run("reload publishes config without rebuilding managers", func() {
		err := s.provider.ReloadConfig(s.T().Context(), config.New())
		s.NoError(err)
	})
	s.Run("previous manager still serves after reload", func() {
		k8s, err := s.provider.GetDerivedKubernetes(s.T().Context(), "")
		s.NoError(err)
		s.NotNil(k8s)
	})
}

func TestProviderSingle(t *testing.T) {
	suite.Run(t, new(ProviderSingleTestSuite))
}
