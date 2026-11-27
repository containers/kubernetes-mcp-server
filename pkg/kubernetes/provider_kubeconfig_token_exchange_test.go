package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"github.com/stretchr/testify/suite"
)

// KubeConfigProviderTokenExchangeSuite covers how the kubeconfig provider
// resolves, caches and validates per-target token exchange configuration.
type KubeConfigProviderTokenExchangeSuite struct {
	suite.Suite
	kubeconfig string
}

func (s *KubeConfigProviderTokenExchangeSuite) SetupTest() {
	s.kubeconfig = createTestKubeconfigFile(s.T(), "spoke")
}

// newProvider loads a config whose cluster connection points at the test
// kubeconfig and builds the kubeconfig provider from it.
func (s *KubeConfigProviderTokenExchangeSuite) newProvider(toml string) (*kubeConfigClusterProvider, *config.Config) {
	s.T().Helper()
	cfg := readTestConfig(s.T(), s.preamble()+toml)
	provider, err := newKubeConfigClusterProvider(context.Background(), cfg)
	s.Require().NoError(err)
	s.T().Cleanup(provider.Close)
	return provider.(*kubeConfigClusterProvider), cfg
}

func (s *KubeConfigProviderTokenExchangeSuite) preamble() string {
	return s.preambleFor(s.kubeconfig)
}

func (s *KubeConfigProviderTokenExchangeSuite) preambleFor(kubeconfig string) string {
	return `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "` + kubeconfig + `"
	`
}

// targetExchangeTOML is a complete per-target exchange block for the "spoke"
// context; extra lines are appended inside the target table.
func targetExchangeTOML(extra ...string) string {
	toml := `
	[cluster_provider_configs.kubeconfig]
	token_exchange_strategy = "rfc8693"

	[cluster_provider_configs.kubeconfig.targets.spoke]
	token_url = "https://spoke-idp.example.com/token"
	audience = "spoke"
	`
	for _, line := range extra {
		toml += "\n\t" + line
	}
	return toml
}

func (s *KubeConfigProviderTokenExchangeSuite) TestResolution() {
	s.Run("target not configured returns nil", func() {
		p, _ := s.newProvider(targetExchangeTOML())
		s.Nil(p.GetTokenExchangeConfig("other"))
	})

	s.Run("empty target resolves the default context", func() {
		p, _ := s.newProvider(targetExchangeTOML())
		s.NotNil(p.GetTokenExchangeConfig(""), "the default context is the first test kubeconfig context")
	})

	s.Run("resolves a self-contained target without a global block", func() {
		p, _ := s.newProvider(`
			[cluster_provider_configs.kubeconfig]
			token_exchange_strategy = "keycloak-v1"

			[cluster_provider_configs.kubeconfig.targets.spoke]
			token_url = "https://other-idp.example.com/token"
			audience = "spoke"
			scopes = ["mcp:spoke"]
			subject_issuer = "external"
			subject_token_type = "urn:ietf:params:oauth:token-type:id_token"
			requested_token_type = "urn:ietf:params:oauth:token-type:access_token"
			certificate_authority = "/etc/spoke-ca.crt"

			[cluster_provider_configs.kubeconfig.targets.spoke.client_auth]
			method = "client_secret_post"
			client_id = "spoke-client"
			client_secret = "spoke-secret"
		`)
		cfg := p.GetTokenExchangeConfig("spoke")
		s.Require().NotNil(cfg)
		s.Equal("https://other-idp.example.com/token", cfg.GetTokenURL())
		s.Equal("spoke", cfg.Audience)
		s.Equal([]string{"mcp:spoke"}, cfg.Scopes)
		s.Equal("external", cfg.SubjectIssuer)
		s.Equal("urn:ietf:params:oauth:token-type:id_token", cfg.SubjectTokenType)
		s.Equal("urn:ietf:params:oauth:token-type:access_token", cfg.RequestedTokenType)
		s.Equal("/etc/spoke-ca.crt", cfg.CAFile)
		s.Equal("spoke-client", cfg.ClientID)
		s.Equal("spoke-secret", cfg.ClientSecret)
		s.Equal(tokenexchange.AuthStyleParams, cfg.AuthStyle)
	})

	s.Run("GetTokenExchangeStrategy returns the provider-level strategy", func() {
		p, _ := s.newProvider(targetExchangeTOML())
		s.Equal(tokenexchange.StrategyRFC8693, p.GetTokenExchangeStrategy())
	})

	s.Run("certificate_authority falls back to the top-level option", func() {
		p, _ := s.newProvider(`certificate_authority = "/etc/hub-ca.crt"` + targetExchangeTOML())
		cfg := p.GetTokenExchangeConfig("spoke")
		s.Require().NotNil(cfg)
		s.Equal("/etc/hub-ca.crt", cfg.CAFile, "CA falls back to top-level certificate_authority when unset on the target")
	})

	s.Run("target public client_id needs no method", func() {
		p, _ := s.newProvider(targetExchangeTOML(`
			[cluster_provider_configs.kubeconfig.targets.spoke.client_auth]
			client_id = "public-spoke"
		`))
		cfg := p.GetTokenExchangeConfig("spoke")
		s.Require().NotNil(cfg)
		s.Equal("public-spoke", cfg.ClientID)
		s.Empty(cfg.ClientSecret)
		s.Empty(cfg.AuthStyle)
	})
}

func (s *KubeConfigProviderTokenExchangeSuite) TestStartupValidation() {
	newProviderErr := func(toml string) error {
		s.T().Helper()
		cfg := readTestConfig(s.T(), s.preamble()+toml)
		provider, err := newKubeConfigClusterProvider(context.Background(), cfg)
		if provider != nil {
			provider.Close()
		}
		return err
	}

	s.Run("fails when a target does not match any kubeconfig context", func() {
		err := newProviderErr(`
			[cluster_provider_configs.kubeconfig]
			token_exchange_strategy = "rfc8693"

			[cluster_provider_configs.kubeconfig.targets."spoke-typo"]
			token_url = "https://idp.example.com/token"
			audience = "spoke"
		`)
		s.Require().Error(err)
		s.Contains(err.Error(), `target "spoke-typo": no matching context in the kubeconfig`)
		s.Contains(err.Error(), "available contexts: spoke")
	})

	s.Run("reports every failing target", func() {
		kubeconfig := createTestKubeconfigFile(s.T(), "a", "b")
		cfg := readTestConfig(s.T(), s.preambleFor(kubeconfig)+`
			[cluster_provider_configs.kubeconfig]
			token_exchange_strategy = "rfc8693"

			[cluster_provider_configs.kubeconfig.targets."a-typo"]
			token_url = "https://idp.example.com/token"

			[cluster_provider_configs.kubeconfig.targets."b-typo"]
			token_url = "https://idp.example.com/token"
		`)
		provider, err := newKubeConfigClusterProvider(context.Background(), cfg)
		if provider != nil {
			provider.Close()
		}
		s.Require().Error(err)
		s.Contains(err.Error(), `target "a-typo": no matching context in the kubeconfig`)
		s.Contains(err.Error(), `target "b-typo": no matching context in the kubeconfig`)
	})
}

// TestClusterAuthValidation covers the require_oauth and cluster_auth_mode
// rules for per-target exchange. They run with config validation — the same
// checks the global [token_exchange] block follows, enforced at startup and
// on reload — not at provider construction.
func (s *KubeConfigProviderTokenExchangeSuite) TestClusterAuthValidation() {
	targetsTOML := func(extra ...string) string {
		toml := s.preamble()
		for _, line := range extra {
			toml += "\n\t" + line
		}
		return toml + `
		[cluster_provider_configs.kubeconfig]
		token_exchange_strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.spoke]
		token_url = "https://idp.example.com/token"
		audience = "spoke"
		`
	}

	s.Run("fails when require_oauth is false", func() {
		cfg := readTestConfig(s.T(), targetsTOML())
		err := cfg.ValidateClusterAuthMode()
		s.Require().Error(err)
		s.Contains(err.Error(), "cluster_provider_configs.kubeconfig: per-target token exchange requires require_oauth=true")
	})

	s.Run("fails with cluster_auth_mode kubeconfig", func() {
		cfg := readTestConfig(s.T(), targetsTOML(`require_oauth = true`, `cluster_auth_mode = "kubeconfig"`))
		err := cfg.ValidateClusterAuthMode()
		s.Require().Error(err)
		s.Contains(err.Error(), `cluster_provider_configs.kubeconfig: per-target token exchange is incompatible with cluster_auth_mode "kubeconfig"`)
	})

	s.Run("accepts require_oauth true with passthrough mode", func() {
		cfg := readTestConfig(s.T(), targetsTOML(`require_oauth = true`))
		s.NoError(cfg.ValidateClusterAuthMode())
	})

	s.Run("no per-target rules without targets", func() {
		cfg := readTestConfig(s.T(), s.preamble()+`
			cluster_auth_mode = "kubeconfig"
		`)
		s.NoError(cfg.ValidateClusterAuthMode())
	})
}

func (s *KubeConfigProviderTokenExchangeSuite) TestCache() {
	s.Run("same resolved values reuse the cached exchange", func() {
		p, _ := s.newProvider(targetExchangeTOML())
		first := p.GetTokenExchangeConfig("spoke")
		second := p.GetTokenExchangeConfig("spoke")
		s.Require().NotNil(first)
		s.Same(first, second)
	})

	s.Run("rotated target credentials rebuild the exchange", func() {
		p, _ := s.newProvider(targetExchangeTOML(`
			[cluster_provider_configs.kubeconfig.targets.spoke.client_auth]
			method = "client_secret_basic"
			client_id = "spoke-client"
			client_secret = "spoke-secret"
		`))
		before := p.GetTokenExchangeConfig("spoke")
		s.Require().NotNil(before)
		s.Equal("spoke-secret", before.ClientSecret)

		rotated := readTestConfig(s.T(), s.preamble()+targetExchangeTOML(`
			[cluster_provider_configs.kubeconfig.targets.spoke.client_auth]
			method = "client_secret_basic"
			client_id = "spoke-client"
			client_secret = "spoke-secret-rotated"
		`))
		s.Require().NoError(p.ReloadConfig(context.Background(), rotated))
		after := p.GetTokenExchangeConfig("spoke")
		s.Require().NotNil(after)
		s.NotSame(before, after)
		s.Equal("spoke-secret-rotated", after.ClientSecret)
	})

	s.Run("reloaded trusted CA rebuilds the exchange", func() {
		p, _ := s.newProvider(`certificate_authority = "/etc/ca-old.crt"` + targetExchangeTOML())
		before := p.GetTokenExchangeConfig("spoke")
		s.Require().NotNil(before)
		s.Equal("/etc/ca-old.crt", before.CAFile)

		rotated := readTestConfig(s.T(), `certificate_authority = "/etc/ca-new.crt"`+s.preamble()+targetExchangeTOML())
		s.Require().NoError(p.ReloadConfig(context.Background(), rotated))
		after := p.GetTokenExchangeConfig("spoke")
		s.Require().NotNil(after)
		s.NotSame(before, after)
		s.Equal("/etc/ca-new.crt", after.CAFile)
	})

	s.Run("Close clears the cache", func() {
		p, _ := s.newProvider(targetExchangeTOML())
		s.Require().NotNil(p.GetTokenExchangeConfig("spoke"))
		p.targetConfigsMu.Lock()
		s.Len(p.targetConfigs, 1)
		p.targetConfigsMu.Unlock()

		p.Close()

		p.targetConfigsMu.Lock()
		defer p.targetConfigsMu.Unlock()
		s.Empty(p.targetConfigs)
	})
}

func (s *KubeConfigProviderTokenExchangeSuite) TestExchangeRouting() {
	bearer := func() context.Context {
		return context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer subject-token")
	}

	// routingTargetExchangeTOML points the target's token endpoint at plain
	// HTTP so require_tls fails the exchange before any request is made; the
	// resulting error proves which path ExchangeTokenInContext took.
	routingTargetExchangeTOML := `
		[cluster_provider_configs.kubeconfig]
		token_exchange_strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.spoke]
		token_url = "http://spoke-idp.example.com/token"
	`

	s.Run("per-target exchange runs in passthrough mode", func() {
		p, cfg := s.newProvider(routingTargetExchangeTOML)
		cfg.RequireTLS.SetForTest(true)
		_, err := ExchangeTokenInContext(bearer(), cfg, p, "spoke", nil)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled", "the configured target must take the per-target exchange path")
	})

	s.Run("per-target exchange runs regardless of cluster auth mode", func() {
		// This documents the pre-existing TokenExchangeProvider contract,
		// shared with the downstream ACM hub provider: a configured target is
		// exchanged before the cluster auth mode is consulted. Config
		// validation rejects kubeconfig mode with targets, so this cannot
		// arise from a valid configuration.
		p, cfg := s.newProvider(routingTargetExchangeTOML)
		cfg.ClusterAuthMode.SetForTest(config.ClusterAuthKubeconfig)
		cfg.RequireTLS.SetForTest(true)
		_, err := ExchangeTokenInContext(bearer(), cfg, p, "spoke", nil)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})

	s.Run("unconfigured target in kubeconfig mode drops the token", func() {
		p, cfg := s.newProvider("")
		cfg.ClusterAuthMode.SetForTest(config.ClusterAuthKubeconfig)
		result, err := ExchangeTokenInContext(bearer(), cfg, p, "spoke", nil)
		s.Require().NoError(err)
		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Empty(auth)
	})

	s.Run("removing the global block on reload leaves targets untouched", func() {
		// Targets are self-contained: unlike the inheritance-based design,
		// a SIGHUP that drops [token_exchange] cannot break a target.
		p, cfg := s.newProvider(`
			[token_exchange]
			strategy = "rfc8693"
			audience = "hub"
		` + routingTargetExchangeTOML)
		cfg.RequireTLS.SetForTest(true)
		_, err := ExchangeTokenInContext(bearer(), cfg, p, "spoke", nil)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")

		reloaded := readTestConfig(s.T(), s.preamble()+routingTargetExchangeTOML)
		reloaded.RequireTLS.SetForTest(true)
		s.Require().NoError(p.ReloadConfig(context.Background(), reloaded))
		s.NotNil(p.GetTokenExchangeConfig("spoke"))
		_, err = ExchangeTokenInContext(bearer(), reloaded, p, "spoke", nil)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})
}

func TestKubeConfigProviderTokenExchange(t *testing.T) {
	suite.Run(t, new(KubeConfigProviderTokenExchangeSuite))
}

// createTestKubeconfigFile writes a kubeconfig with the given context names
// (the first becomes the current context) and returns a TOML-safe path to it.
func createTestKubeconfigFile(t testing.TB, contextNames ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	content := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
  name: test-cluster
contexts:
`
	for _, name := range contextNames {
		content += `- context:
    cluster: test-cluster
    user: test-user
  name: ` + name + `
`
	}
	content += `current-context: ` + contextNames[0] + `
users:
- name: test-user
  user:
    token: test-token
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return filepath.ToSlash(path)
}
