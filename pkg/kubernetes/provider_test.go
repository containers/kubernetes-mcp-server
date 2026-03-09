package kubernetes

import (
	"os"
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
	originalEnv             []string
	originalInClusterConfig func() (*rest.Config, error)
	mockServer              *test.MockServer
	kubeconfigPath          string
}

func (s *ProviderTestSuite) SetupTest() {
	s.BaseProviderSuite.SetupTest()
	s.originalEnv = os.Environ()
	s.originalInClusterConfig = InClusterConfig
	s.mockServer = test.NewMockServer()
	s.kubeconfigPath = strings.ReplaceAll(s.mockServer.KubeconfigFile(s.T()), `\`, `\\`)
}

func (s *ProviderTestSuite) TearDownTest() {
	s.BaseProviderSuite.TearDownTest()
	test.RestoreEnv(s.originalEnv)
	InClusterConfig = s.originalInClusterConfig
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *ProviderTestSuite) TestNewProviderInCluster() {
	InClusterConfig = func() (*rest.Config, error) {
		return &rest.Config{}, nil
	}
	s.Run("With no cluster_provider_strategy, returns single-cluster provider", func() {
		cfg := test.Must(config.ReadToml([]byte{}))
		provider, err := NewProvider(cfg)
		s.Require().NoError(err, "Expected no error for in-cluster provider")
		s.NotNil(provider, "Expected provider instance")
		s.IsType(&singleClusterProvider{}, provider, "Expected singleClusterProvider type")
	})
	s.Run("With cluster_provider_strategy=in-cluster, returns single-cluster provider", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "in-cluster"
		`)))
		provider, err := NewProvider(cfg)
		s.Require().NoError(err, "Expected no error for single-cluster strategy")
		s.NotNil(provider, "Expected provider instance")
		s.IsType(&singleClusterProvider{}, provider, "Expected singleClusterProvider type")
	})
	s.Run("With cluster_provider_strategy=kubeconfig, returns error", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "kubeconfig"
		`)))
		provider, err := NewProvider(cfg)
		s.Require().Error(err, "Expected error for kubeconfig strategy")
		s.ErrorContains(err, "kubeconfig ClusterProviderStrategy is invalid for in-cluster deployments")
		s.ErrorContains(err, "--kubeconfig /path/to/kubeconfig --cluster-provider kubeconfig")
		s.ErrorContains(err, "docs/configuration.md#cross-cluster-access-from-a-pod")
		s.Nilf(provider, "Expected no provider instance, got %v", provider)
	})
	s.Run("With cluster_provider_strategy=kubeconfig and kubeconfig set to valid path, returns kubeconfig provider", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "kubeconfig"
			kubeconfig = "` + s.kubeconfigPath + `"
		`)))
		provider, err := NewProvider(cfg)
		s.Require().NoError(err, "Expected no error for kubeconfig strategy")
		s.NotNil(provider, "Expected provider instance")
		s.IsType(&kubeConfigClusterProvider{}, provider, "Expected kubeConfigClusterProvider type")
	})
	s.Run("With cluster_provider_strategy=non-existent, returns error", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "i-do-not-exist"
		`)))
		provider, err := NewProvider(cfg)
		s.Require().Error(err, "Expected error for non-existent strategy")
		s.ErrorContains(err, "no provider registered for strategy 'i-do-not-exist'")
		s.Nilf(provider, "Expected no provider instance, got %v", provider)
	})
}

func (s *ProviderTestSuite) TestNewProviderLocal() {
	InClusterConfig = func() (*rest.Config, error) {
		return nil, rest.ErrNotInCluster
	}
	s.Require().NoError(os.Setenv("KUBECONFIG", s.kubeconfigPath))
	s.Run("With no cluster_provider_strategy, returns kubeconfig provider", func() {
		cfg := test.Must(config.ReadToml([]byte{}))
		provider, err := NewProvider(cfg)
		s.Require().NoError(err, "Expected no error for kubeconfig provider")
		s.NotNil(provider, "Expected provider instance")
		s.IsType(&kubeConfigClusterProvider{}, provider, "Expected kubeConfigClusterProvider type")
	})
	s.Run("With cluster_provider_strategy=kubeconfig, returns kubeconfig provider", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "kubeconfig"
		`)))
		provider, err := NewProvider(cfg)
		s.Require().NoError(err, "Expected no error for kubeconfig provider")
		s.NotNil(provider, "Expected provider instance")
		s.IsType(&kubeConfigClusterProvider{}, provider, "Expected kubeConfigClusterProvider type")
	})
	s.Run("With cluster_provider_strategy=disabled, returns single-cluster provider", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "disabled"
		`)))
		provider, err := NewProvider(cfg)
		s.Require().NoError(err, "Expected no error for disabled strategy")
		s.NotNil(provider, "Expected provider instance")
		s.IsType(&singleClusterProvider{}, provider, "Expected singleClusterProvider type")
	})
	s.Run("With cluster_provider_strategy=in-cluster, returns error", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "in-cluster"
		`)))
		provider, err := NewProvider(cfg)
		s.Require().Error(err, "Expected error for in-cluster strategy")
		s.ErrorContains(err, "server must be deployed in cluster for the in-cluster ClusterProviderStrategy")
		s.Nilf(provider, "Expected no provider instance, got %v", provider)
	})
	s.Run("With cluster_provider_strategy=in-cluster and kubeconfig set to valid path, returns error", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + s.kubeconfigPath + `"
			cluster_provider_strategy = "in-cluster"
		`)))
		provider, err := NewProvider(cfg)
		s.Require().Error(err, "Expected error for in-cluster strategy")
		s.Regexp("kubeconfig file .+ cannot be used with the in-cluster ClusterProviderStrategy", err.Error())
		s.Nilf(provider, "Expected no provider instance, got %v", provider)
	})
	s.Run("With cluster_provider_strategy=non-existent, returns error", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			cluster_provider_strategy = "i-do-not-exist"
		`)))
		provider, err := NewProvider(cfg)
		s.Require().Error(err, "Expected error for non-existent strategy")
		s.ErrorContains(err, "no provider registered for strategy 'i-do-not-exist'")
		s.Nilf(provider, "Expected no provider instance, got %v", provider)
	})
}

func TestProvider(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}
