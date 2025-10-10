package kubernetes

import (
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"
)

type BaseProviderSuite struct {
	suite.Suite
	originalProviderFactories map[string]ProviderFactory
}

func (s *BaseProviderSuite) SetupTest() {
	s.originalProviderFactories = make(map[string]ProviderFactory)
	for k, v := range providerFactories {
		s.originalProviderFactories[k] = v
	}
}

func (s *BaseProviderSuite) TearDownTest() {
	providerFactories = make(map[string]ProviderFactory)
	for k, v := range s.originalProviderFactories {
		providerFactories[k] = v
	}
}

type ProviderTestSuite struct {
	BaseProviderSuite
}

func (s *ProviderTestSuite) TestNewManagerProviderInCluster() {
	originalIsInClusterConfig := InClusterConfig
	s.T().Cleanup(func() {
		InClusterConfig = originalIsInClusterConfig
	})
	InClusterConfig = func() (*rest.Config, error) {
		return &rest.Config{}, nil
	}
	s.Run("With no cluster_provider_strategy, returns single-cluster provider", func() {
		cfg := test.Must(config.ReadToml([]byte{}))
		provider, err := NewManagerProvider(cfg)
		s.Require().NoError(err, "Expected no error for in-cluster provider")
		s.NotNil(provider, "Expected provider instance")
		s.IsType(&singleClusterProvider{}, provider, "Expected singleClusterProvider type")
	})
	s.Run("With configured in-cluster cluster_provider_strategy, returns single-cluster provider", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "in-cluster"
		`)))
		provider, err := NewManagerProvider(cfg)
		s.Require().NoError(err, "Expected no error for single-cluster strategy")
		s.NotNil(provider, "Expected provider instance")
		s.IsType(&singleClusterProvider{}, provider, "Expected singleClusterProvider type")
	})
	s.Run("With configured kubeconfig cluster_provider_strategy, returns error", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "kubeconfig"
		`)))
		provider, err := NewManagerProvider(cfg)
		s.Require().Error(err, "Expected error for kubeconfig strategy")
		s.ErrorContains(err, "kubeconfig ClusterProviderStrategy is invalid for in-cluster deployments")
		s.Nilf(provider, "Expected no provider instance, got %v", provider)
	})
	s.Run("With configured non-existent cluster_provider_strategy, returns error", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "i-do-not-exist"
		`)))
		provider, err := NewManagerProvider(cfg)
		s.Require().Error(err, "Expected error for non-existent strategy")
		s.ErrorContains(err, "no provider registered for strategy 'i-do-not-exist'")
		s.Nilf(provider, "Expected no provider instance, got %v", provider)
	})
}

func (s *ProviderTestSuite) TestNewManagerProviderLocal() {
	mockServer := test.NewMockServer()
	s.T().Cleanup(mockServer.Close)
	kubeconfigPath := strings.ReplaceAll(mockServer.KubeconfigFile(s.T()), `\`, `\\`)
	s.Run("With no cluster_provider_strategy, returns kubeconfig provider", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + kubeconfigPath + `"
		`)))
		provider, err := NewManagerProvider(cfg)
		s.Require().NoError(err, "Expected no error for kubeconfig provider")
		s.NotNil(provider, "Expected provider instance")
		s.IsType(&kubeConfigClusterProvider{}, provider, "Expected kubeConfigClusterProvider type")
	})
	s.Run("With configured kubeconfig cluster_provider_strategy, returns kubeconfig provider", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + kubeconfigPath + `"
			cluster_provider_strategy = "kubeconfig"
		`)))
		provider, err := NewManagerProvider(cfg)
		s.Require().NoError(err, "Expected no error for kubeconfig provider")
		s.NotNil(provider, "Expected provider instance")
		s.IsType(&kubeConfigClusterProvider{}, provider, "Expected kubeConfigClusterProvider type")
	})
	s.Run("With configured in-cluster cluster_provider_strategy, returns error", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + kubeconfigPath + `"
			cluster_provider_strategy = "in-cluster"
		`)))
		provider, err := NewManagerProvider(cfg)
		s.Require().Error(err, "Expected error for in-cluster strategy")
		s.ErrorContains(err, "server must be deployed in cluster for the in-cluster ClusterProviderStrategy")
		s.Nilf(provider, "Expected no provider instance, got %v", provider)
	})
	s.Run("With configured non-existent cluster_provider_strategy, returns error", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + kubeconfigPath + `"
			cluster_provider_strategy = "i-do-not-exist"
		`)))
		provider, err := NewManagerProvider(cfg)
		s.Require().Error(err, "Expected error for non-existent strategy")
		s.ErrorContains(err, "no provider registered for strategy 'i-do-not-exist'")
		s.Nilf(provider, "Expected no provider instance, got %v", provider)
	})
}

func TestProvider(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}
