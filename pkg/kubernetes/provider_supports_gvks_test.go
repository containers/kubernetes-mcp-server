package kubernetes

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

var sampleGVKs = []schema.GroupVersionKind{
	{Group: "apps", Version: "v1", Kind: "Deployment"},
	{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine"},
}

type ProviderSupportsGVKsTestSuite struct {
	BaseProviderSuite
	mockServer *test.MockServer
	providers  map[string]Provider
}

func (s *ProviderSupportsGVKsTestSuite) SetupTest() {
	s.BaseProviderSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewACMHubHandler(
		test.ManagedCluster{Name: "cluster-a"},
		test.ManagedCluster{Name: "hub", Labels: map[string]string{"local-cluster": "true"}},
	))
	s.providers = make(map[string]Provider)

	kubeconfigPath := s.mockServer.KubeconfigFile(s.T())

	singleProvider, err := newSingleClusterProvider(api.ClusterProviderDisabled)(&config.StaticConfig{
		KubeConfig: kubeconfigPath,
	})
	s.Require().NoError(err)
	s.providers["single"] = singleProvider

	kubeconfigProvider, err := newKubeConfigClusterProvider(&config.StaticConfig{
		KubeConfig: kubeconfigPath,
	})
	s.Require().NoError(err)
	s.providers["kubeconfig"] = kubeconfigProvider

	originalInClusterConfig := InClusterConfig
	InClusterConfig = func() (*rest.Config, error) {
		return s.mockServer.Config(), nil
	}
	s.T().Cleanup(func() { InClusterConfig = originalInClusterConfig })

	acmProvider, err := NewProvider(test.Must(config.ReadToml([]byte(`
		cluster_provider_strategy = "acm"
		[cluster_provider_configs.acm]
		cluster_proxy_addon_host = "proxy.example.com"
		cluster_proxy_addon_skip_tls_verify = true
	`))))
	s.Require().NoError(err)
	s.providers["acm"] = acmProvider
}

func (s *ProviderSupportsGVKsTestSuite) TearDownTest() {
	for _, provider := range s.providers {
		provider.Close()
	}
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	s.BaseProviderSuite.TearDownTest()
}

func (s *ProviderSupportsGVKsTestSuite) TestSupportsGVKsNoopReturnsTrue() {
	for name, provider := range s.providers {
		s.Run(name, func() {
			s.True(provider.SupportsGVKs(nil), "nil GVK list should be supported")
			s.True(provider.SupportsGVKs([]schema.GroupVersionKind{}), "empty GVK list should be supported")
			s.True(provider.SupportsGVKs(sampleGVKs), "noop provider should report all GVKs supported")
		})
	}
}

type supportsGVKsStubProvider struct {
	result bool
}

func (s *supportsGVKsStubProvider) IsOpenShift(context.Context) bool { return false }
func (s *supportsGVKsStubProvider) IsMultiTarget() bool              { return false }
func (s *supportsGVKsStubProvider) GetTargets(context.Context) ([]string, error) {
	return nil, nil
}
func (s *supportsGVKsStubProvider) GetDerivedKubernetes(context.Context, string) (*Kubernetes, error) {
	return nil, nil
}
func (s *supportsGVKsStubProvider) GetDefaultTarget() string       { return "" }
func (s *supportsGVKsStubProvider) GetTargetParameterName() string { return "" }
func (s *supportsGVKsStubProvider) WatchTargets(McpReload)         {}
func (s *supportsGVKsStubProvider) Close()                         {}
func (s *supportsGVKsStubProvider) SupportsGVKs(_ []schema.GroupVersionKind) bool {
	return s.result
}

func (s *ProviderSupportsGVKsTestSuite) TestTokenExchangingProviderDelegatesSupportsGVKs() {
	s.Run("delegates true from wrapped provider", func() {
		wrapped := &tokenExchangingProvider{
			provider: &supportsGVKsStubProvider{result: true},
		}
		s.True(wrapped.SupportsGVKs(sampleGVKs))
	})

	s.Run("delegates false from wrapped provider", func() {
		wrapped := &tokenExchangingProvider{
			provider:   &supportsGVKsStubProvider{result: false},
			baseConfig: &config.StaticConfig{},
			oauthState: oauth.NewState(&oauth.Snapshot{}),
		}
		s.False(wrapped.SupportsGVKs(sampleGVKs))
	})
}

func TestProviderSupportsGVKs(t *testing.T) {
	suite.Run(t, new(ProviderSupportsGVKsTestSuite))
}
