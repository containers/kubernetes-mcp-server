package kubernetes

import (
	"fmt"
	"net/http"
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
	s.mockServer.Handle(&test.InOpenShiftHandler{})
	s.Run("IsOpenShift returns true", func() {
		inOpenShift := s.provider.IsOpenShift(s.T().Context())
		s.True(inOpenShift, "Expected InOpenShift to return true")
	})
}

func (s *ProviderKubeconfigTestSuite) TestVerifyToken() {
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.EscapedPath() == "/apis/authentication.k8s.io/v1/tokenreviews" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`
				{
					"kind": "TokenReview",
					"apiVersion": "authentication.k8s.io/v1",
					"spec": {"token": "the-token"},
					"status": {
						"authenticated": true,
						"user": {
							"username": "test-user",
							"groups": ["system:authenticated"]
						},
						"audiences": ["the-audience"]
					}
				}`))
		}
	}))
	s.Run("VerifyToken returns UserInfo for non-empty context", func() {
		userInfo, audiences, err := s.provider.VerifyToken(s.T().Context(), "fake-context", "some-token", "the-audience")
		s.Require().NoError(err, "Expected no error from VerifyToken with empty target")
		s.Require().NotNil(userInfo, "Expected UserInfo from VerifyToken with empty target")
		s.Equalf(userInfo.Username, "test-user", "Expected username test-user, got: %s", userInfo.Username)
		s.Containsf(userInfo.Groups, "system:authenticated", "Expected group system:authenticated in %v", userInfo.Groups)
		s.Require().NotNil(audiences, "Expected audiences from VerifyToken with empty target")
		s.Len(audiences, 1, "Expected audiences from VerifyToken with empty target")
		s.Containsf(audiences, "the-audience", "Expected audience the-audience in %v", audiences)
	})
	s.Run("VerifyToken returns UserInfo for empty context (default context", func() {
		userInfo, audiences, err := s.provider.VerifyToken(s.T().Context(), "", "the-token", "the-audience")
		s.Require().NoError(err, "Expected no error from VerifyToken with empty target")
		s.Require().NotNil(userInfo, "Expected UserInfo from VerifyToken with empty target")
		s.Equalf(userInfo.Username, "test-user", "Expected username test-user, got: %s", userInfo.Username)
		s.Containsf(userInfo.Groups, "system:authenticated", "Expected group system:authenticated in %v", userInfo.Groups)
		s.Require().NotNil(audiences, "Expected audiences from VerifyToken with empty target")
		s.Len(audiences, 1, "Expected audiences from VerifyToken with empty target")
		s.Containsf(audiences, "the-audience", "Expected audience the-audience in %v", audiences)
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
