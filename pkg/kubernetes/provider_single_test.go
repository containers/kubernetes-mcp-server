package kubernetes

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"
)

type ProviderSingleTestSuite struct {
	BaseProviderSuite
	mockServer                *test.MockServer
	originalIsInClusterConfig func() (*rest.Config, error)
	provider                  Provider
}

func (s *ProviderSingleTestSuite) SetupTest() {
	// Single cluster provider is used when in-cluster or when the multi-cluster feature is disabled.
	// For this test suite we simulate an in-cluster deployment.
	s.originalIsInClusterConfig = InClusterConfig
	s.mockServer = test.NewMockServer()
	InClusterConfig = func() (*rest.Config, error) {
		return s.mockServer.Config(), nil
	}
	provider, err := NewProvider(&config.StaticConfig{})
	s.Require().NoError(err, "Expected no error creating provider with kubeconfig")
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

func (s *ProviderSingleTestSuite) TestWithNonOpenShiftCluster() {
	s.Run("IsOpenShift returns false", func() {
		inOpenShift := s.provider.IsOpenShift(s.T().Context())
		s.False(inOpenShift, "Expected InOpenShift to return false")
	})
}

func (s *ProviderSingleTestSuite) TestWithOpenShiftCluster() {
	s.mockServer.Handle(test.NewInOpenShiftHandler())

	s.Run("IsOpenShift returns true", func() {
		inOpenShift := s.provider.IsOpenShift(s.T().Context())
		s.True(inOpenShift, "Expected InOpenShift to return true")
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
		s.Empty(s.provider.GetDefaultTarget(), "Expected fake-context as default target")
	})
}

func (s *ProviderSingleTestSuite) TestGetTargetParameterName() {
	s.Empty(s.provider.GetTargetParameterName(), "Expected empty string as target parameter name")
}

func TestProviderSingle(t *testing.T) {
	suite.Run(t, new(ProviderSingleTestSuite))
}
