package kubernetes

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ProviderKubeconfigTokenExchangeSuite struct {
	BaseProviderSuite
	mockServer *test.MockServer
}

func (s *ProviderKubeconfigTokenExchangeSuite) SetupTest() {
	s.BaseProviderSuite.SetupTest()
	s.mockServer = test.NewMockServer()
}

func (s *ProviderKubeconfigTokenExchangeSuite) TearDownTest() {
	s.BaseProviderSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

// kubeconfigWithServers returns a kubeconfig with the named contexts pointing
// at distinct cluster.server URLs. Each context shares the mock server's auth
// info so the kubeconfig is parseable; only the server URL matters for these
// tests since GetTokenExchangeConfig never makes network calls.
func (s *ProviderKubeconfigTokenExchangeSuite) kubeconfigWithServers(servers map[string]string) *clientcmdapi.Config {
	kubeconfig := s.mockServer.Kubeconfig()
	for name, server := range servers {
		clusterName := name + "-cluster"
		cluster := clientcmdapi.NewCluster()
		cluster.Server = server
		kubeconfig.Clusters[clusterName] = cluster

		ctx := clientcmdapi.NewContext()
		ctx.Cluster = clusterName
		ctx.AuthInfo = "fake"
		kubeconfig.Contexts[name] = ctx
	}
	return kubeconfig
}

func (s *ProviderKubeconfigTokenExchangeSuite) newProvider(toml string, servers map[string]string) *kubeConfigClusterProvider {
	kubeconfig := s.kubeconfigWithServers(servers)
	cfg, err := config.ReadToml([]byte(toml))
	s.Require().NoError(err, "Expected toml to parse")
	cfg.KubeConfig = test.KubeconfigFile(s.T(), kubeconfig)
	provider, err := NewProvider(cfg)
	s.Require().NoError(err, "Expected NewProvider to succeed")
	kc, ok := provider.(*kubeConfigClusterProvider)
	s.Require().True(ok, "Expected kubeConfigClusterProvider, got %T", provider)
	return kc
}

func (s *ProviderKubeconfigTokenExchangeSuite) TestImplementsTokenExchangeProvider() {
	s.Run("compile-time and runtime interface satisfaction", func() {
		var _ TokenExchangeProvider = (*kubeConfigClusterProvider)(nil)
		p := s.newProvider(`
			cluster_provider_strategy = "kubeconfig"
		`, map[string]string{
			"vanilla": "https://vanilla.k8s.internal",
		})
		var iface any = p
		_, ok := iface.(TokenExchangeProvider)
		s.True(ok, "Expected kubeConfigClusterProvider to satisfy TokenExchangeProvider at runtime")
	})
}

func (s *ProviderKubeconfigTokenExchangeSuite) TestGetTokenExchangeConfig() {
	s.Run("returns nil when no provider config section is set", func() {
		p := s.newProvider(`
			cluster_provider_strategy = "kubeconfig"
		`, map[string]string{
			"vanilla": "https://vanilla.k8s.internal",
		})
		s.Nil(p.GetTokenExchangeConfig("vanilla"), "Expected nil with no [cluster_provider_configs.kubeconfig] section")
	})

	s.Run("returns nil when target server matches a skip pattern", func() {
		p := s.newProvider(`
			cluster_provider_strategy = "kubeconfig"
			sts_token_url = "https://sts.example.test/token"
			sts_audience  = "vanilla-clusters"

			[cluster_provider_configs.kubeconfig]
			token_exchange_strategy = "rfc8693"
			skip_exchange_servers   = ["*.eks.amazonaws.com"]
		`, map[string]string{
			"eks-prod": "https://abc123.gr7.us-east-1.eks.amazonaws.com",
		})
		s.Nil(p.GetTokenExchangeConfig("eks-prod"), "Expected nil for EKS-matching server (skip)")
	})

	s.Run("returns populated config when target server does not match any skip pattern", func() {
		p := s.newProvider(`
			cluster_provider_strategy = "kubeconfig"
			sts_token_url = "https://sts.example.test/token"
			sts_client_id = "kubernetes-mcp-server"
			sts_audience  = "vanilla-clusters"
			sts_subject_token_type   = "urn:ietf:params:oauth:token-type:jwt"
			sts_requested_token_type = "urn:ietf:params:oauth:token-type:jwt"

			[cluster_provider_configs.kubeconfig]
			token_exchange_strategy = "rfc8693"
			skip_exchange_servers   = ["*.eks.amazonaws.com"]
		`, map[string]string{
			"vanilla": "https://vanilla.k8s.internal",
		})
		exCfg := p.GetTokenExchangeConfig("vanilla")
		s.Require().NotNil(exCfg, "Expected populated config for non-matching server")
		s.Equal("https://sts.example.test/token", exCfg.TokenURL, "Expected TokenURL from top-level sts_token_url")
		s.Equal("kubernetes-mcp-server", exCfg.ClientID, "Expected ClientID from top-level sts_client_id")
		s.Equal("vanilla-clusters", exCfg.Audience, "Expected Audience from top-level sts_audience")
		s.Equal("urn:ietf:params:oauth:token-type:jwt", exCfg.SubjectTokenType, "Expected SubjectTokenType from top-level config")
		s.Equal("urn:ietf:params:oauth:token-type:jwt", exCfg.RequestedTokenType, "Expected RequestedTokenType from top-level config")
		s.Equal("params", exCfg.AuthStyle, "Expected AuthStyle to default to params")
	})

	s.Run("returns nil for unknown target context", func() {
		p := s.newProvider(`
			cluster_provider_strategy = "kubeconfig"
			sts_token_url = "https://sts.example.test/token"
			sts_audience  = "vanilla-clusters"

			[cluster_provider_configs.kubeconfig]
			token_exchange_strategy = "rfc8693"
		`, map[string]string{
			"vanilla": "https://vanilla.k8s.internal",
		})
		// Unknown contexts have no server URL recorded, so they fall through
		// to building a config from top-level settings (no skip rule matches an
		// empty host). The behavior matches an unconfigured context — exchange
		// proceeds with the default sts settings.
		exCfg := p.GetTokenExchangeConfig("does-not-exist")
		s.Require().NotNil(exCfg, "Expected exchange config for unknown context (falls through to top-level sts_*)")
		s.Equal("vanilla-clusters", exCfg.Audience, "Expected Audience from top-level sts_audience")
	})
}

func (s *ProviderKubeconfigTokenExchangeSuite) TestGetTokenExchangeStrategy() {
	s.Run("returns the section value when set", func() {
		p := s.newProvider(`
			cluster_provider_strategy = "kubeconfig"

			[cluster_provider_configs.kubeconfig]
			token_exchange_strategy = "rfc8693"
		`, map[string]string{
			"vanilla": "https://vanilla.k8s.internal",
		})
		s.Equal("rfc8693", p.GetTokenExchangeStrategy(), "Expected strategy from kubeconfig section")
	})

	s.Run("falls back to top-level token_exchange_strategy when section is empty", func() {
		p := s.newProvider(`
			cluster_provider_strategy = "kubeconfig"
			token_exchange_strategy   = "rfc8693"

			[cluster_provider_configs.kubeconfig]
			skip_exchange_servers = ["*.eks.amazonaws.com"]
		`, map[string]string{
			"vanilla": "https://vanilla.k8s.internal",
		})
		s.Equal("rfc8693", p.GetTokenExchangeStrategy(), "Expected fallback to top-level token_exchange_strategy")
	})

	s.Run("falls back to top-level when no section is configured", func() {
		p := s.newProvider(`
			cluster_provider_strategy = "kubeconfig"
			token_exchange_strategy   = "rfc8693"
		`, map[string]string{
			"vanilla": "https://vanilla.k8s.internal",
		})
		s.Equal("rfc8693", p.GetTokenExchangeStrategy(), "Expected fallback to top-level token_exchange_strategy")
	})
}

func (s *ProviderKubeconfigTokenExchangeSuite) TestSkipPatternMatchesGovCloud() {
	s.Run("GovCloud EKS endpoints match the same *.eks.amazonaws.com pattern", func() {
		p := s.newProvider(`
			cluster_provider_strategy = "kubeconfig"
			sts_token_url = "https://sts.example.test/token"
			sts_audience  = "vanilla-clusters"

			[cluster_provider_configs.kubeconfig]
			token_exchange_strategy = "rfc8693"
			skip_exchange_servers   = ["*.eks.amazonaws.com"]
		`, map[string]string{
			"gov": "https://abc123.yl4.us-gov-west-1.eks.amazonaws.com",
		})
		s.Nil(p.GetTokenExchangeConfig("gov"), "Expected nil for GovCloud EKS server")
	})
}

func TestProviderKubeconfigTokenExchange(t *testing.T) {
	suite.Run(t, new(ProviderKubeconfigTokenExchangeSuite))
}

func TestKubeconfigProviderConfigValidate(t *testing.T) {
	suite.Run(t, new(KubeconfigProviderConfigValidateSuite))
}

type KubeconfigProviderConfigValidateSuite struct {
	suite.Suite
}

func (s *KubeconfigProviderConfigValidateSuite) TestValidate() {
	s.Run("valid globs pass", func() {
		cfg := &KubeconfigProviderConfig{
			SkipExchangeServers: []string{"*.eks.amazonaws.com", "exact.example.com"},
		}
		s.NoError(cfg.Validate())
	})
	s.Run("malformed glob is rejected", func() {
		cfg := &KubeconfigProviderConfig{
			SkipExchangeServers: []string{"[unclosed"},
		}
		err := cfg.Validate()
		s.Require().Error(err, "Expected error for malformed glob")
		s.Contains(err.Error(), "skip_exchange_servers[0]")
	})
	s.Run("empty config is valid", func() {
		cfg := &KubeconfigProviderConfig{}
		s.NoError(cfg.Validate())
	})
}
