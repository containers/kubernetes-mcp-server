package kubernetes

import (
	"fmt"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ProviderKubeconfigTestSuite struct {
	BaseProviderSuite
	mockServer *test.MockServer
	provider   Provider
}

func (s *ProviderKubeconfigTestSuite) SetupTest() {
	// Kubeconfig provider is used when the multi-cluster feature is enabled with the kubeconfig strategy.
	// For this test suite we simulate a kubeconfig with multiple contexts.
	s.mockServer = test.NewMockServer()
	kubeconfig := s.mockServer.Kubeconfig()
	for i := 0; i < 10; i++ {
		// Add multiple fake contexts to force multi-cluster behavior
		kubeconfig.Contexts[fmt.Sprintf("context-%d", i)] = clientcmdapi.NewContext()
	}
	provider, err := NewProvider(&config.StaticConfig{KubeConfig: test.KubeconfigFile(s.T(), kubeconfig)})
	s.Require().NoError(err, "Expected no error creating provider with kubeconfig")
	s.provider = provider
}

func (s *ProviderKubeconfigTestSuite) TearDownTest() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *ProviderKubeconfigTestSuite) TestType() {
	s.IsType(&kubeConfigClusterProvider{}, s.provider)
}

func (s *ProviderKubeconfigTestSuite) TestWithNonOpenShiftCluster() {
	s.Run("IsOpenShift returns false", func() {
		inOpenShift := s.provider.IsOpenShift(s.T().Context())
		s.False(inOpenShift, "Expected InOpenShift to return false")
	})
}

func (s *ProviderKubeconfigTestSuite) TestWithOpenShiftCluster() {
	s.mockServer.Handle(test.NewInOpenShiftHandler())
	s.Run("IsOpenShift returns true", func() {
		inOpenShift := s.provider.IsOpenShift(s.T().Context())
		s.True(inOpenShift, "Expected InOpenShift to return true")
	})
}

func (s *ProviderKubeconfigTestSuite) TestGetTargets() {
	s.Run("GetTargets returns all contexts defined in kubeconfig", func() {
		targets, err := s.provider.GetTargets(s.T().Context())
		s.Require().NoError(err, "Expected no error from GetTargets")
		s.Len(targets, 11, "Expected 11 targets from GetTargets")
		s.Contains(targets, "fake-context", "Expected fake-context in targets from GetTargets")
		for i := 0; i < 10; i++ {
			s.Contains(targets, fmt.Sprintf("context-%d", i), "Expected context-%d in targets from GetTargets", i)
		}
	})
}

func (s *ProviderKubeconfigTestSuite) TestGetDerivedKubernetes() {
	s.Run("GetDerivedKubernetes returns Kubernetes for valid context", func() {
		k8s, err := s.provider.GetDerivedKubernetes(s.T().Context(), "fake-context")
		s.Require().NoError(err, "Expected no error from GetDerivedKubernetes with valid context")
		s.NotNil(k8s, "Expected Kubernetes from GetDerivedKubernetes with valid context")
	})
	s.Run("GetDerivedKubernetes returns Kubernetes for empty context (default)", func() {
		k8s, err := s.provider.GetDerivedKubernetes(s.T().Context(), "")
		s.Require().NoError(err, "Expected no error from GetDerivedKubernetes with empty context")
		s.NotNil(k8s, "Expected Kubernetes from GetDerivedKubernetes with empty context")
	})
	s.Run("GetDerivedKubernetes returns error for invalid context", func() {
		k8s, err := s.provider.GetDerivedKubernetes(s.T().Context(), "invalid-context")
		s.Require().Error(err, "Expected error from GetDerivedKubernetes with invalid context")
		s.ErrorContainsf(err, `context "invalid-context" does not exist`, "Expected context does not exist error, got: %v", err)
		s.Nil(k8s, "Expected no Kubernetes from GetDerivedKubernetes with invalid context")
	})
}

func (s *ProviderKubeconfigTestSuite) TestGetDefaultTarget() {
	s.Run("GetDefaultTarget returns current-context defined in kubeconfig", func() {
		s.Equal("fake-context", s.provider.GetDefaultTarget(), "Expected fake-context as default target")
	})
}

func (s *ProviderKubeconfigTestSuite) TestGetTargetParameterName() {
	s.Equal("context", s.provider.GetTargetParameterName(), "Expected context as target parameter name")
}

func TestProviderKubeconfig(t *testing.T) {
	suite.Run(t, new(ProviderKubeconfigTestSuite))
}
