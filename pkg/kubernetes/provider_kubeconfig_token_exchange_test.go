package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	return `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "` + s.kubeconfig + `"
	`
}

// globalExchangeTOML is a complete global [token_exchange] block; extra lines
// are appended inside the [token_exchange] table.
func globalExchangeTOML(extra ...string) string {
	toml := `
	authorization_url = "https://idp.example.com/realms/hub"

	[token_exchange]
	strategy = "rfc8693"
	audience = "hub"
	scopes = ["mcp:hub"]
	`
	for _, line := range extra {
		toml += "\n\t" + line
	}
	return toml + `

	[token_exchange.client_auth]
	method = "client_secret_basic"
	client_id = "global-client"
	client_secret = "global-secret"
`
}

func (s *KubeConfigProviderTokenExchangeSuite) TestResolution() {
	s.Run("target not configured returns nil", func() {
		p, cfg := s.newProvider(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		te, err := p.GetTargetTokenExchange("other", cfg, "")
		s.Require().NoError(err)
		s.Nil(te)
	})

	s.Run("inherits global defaults and overrides only what the target sets", func() {
		p, cfg := s.newProvider(`certificate_authority = "/etc/hub-ca.crt"` + globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		te, err := p.GetTargetTokenExchange("spoke", cfg, "https://idp.example.com/token")
		s.Require().NoError(err)
		s.Require().NotNil(te)
		s.Equal(tokenexchange.StrategyRFC8693, te.Strategy)
		s.Equal("spoke", te.Config.Audience)
		s.Equal([]string{"mcp:hub"}, te.Config.Scopes, "scopes inherit from global when unset")
		s.Equal("https://idp.example.com/token", te.Config.GetTokenURL(), "token URL falls back to OIDC discovery")
		s.Equal("global-client", te.Config.ClientID)
		s.Equal("global-secret", te.Config.ClientSecret)
		s.Equal(tokenexchange.AuthStyleHeader, te.Config.AuthStyle)
		s.Equal("/etc/hub-ca.crt", te.Config.CAFile, "CA inherits top-level certificate_authority")
	})

	s.Run("self-contained target works without a global block", func() {
		p, cfg := s.newProvider(`
			[cluster_provider_configs.kubeconfig.targets.spoke]
			strategy = "keycloak-v1"
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
		s.Nil(cfg.GetTokenExchangeConfig(), "precondition: no global block")

		te, err := p.GetTargetTokenExchange("spoke", cfg, "")
		s.Require().NoError(err)
		s.Require().NotNil(te)
		s.Equal("keycloak-v1", te.Strategy)
		s.Equal("https://other-idp.example.com/token", te.Config.GetTokenURL())
		s.Equal("spoke", te.Config.Audience)
		s.Equal([]string{"mcp:spoke"}, te.Config.Scopes)
		s.Equal("external", te.Config.SubjectIssuer)
		s.Equal("urn:ietf:params:oauth:token-type:id_token", te.Config.SubjectTokenType)
		s.Equal("urn:ietf:params:oauth:token-type:access_token", te.Config.RequestedTokenType)
		s.Equal("/etc/spoke-ca.crt", te.Config.CAFile)
		s.Equal("spoke-client", te.Config.ClientID)
		s.Equal("spoke-secret", te.Config.ClientSecret)
		s.Equal(tokenexchange.AuthStyleParams, te.Config.AuthStyle)
	})

	s.Run("target token_url wins over global and discovered", func() {
		p, cfg := s.newProvider(globalExchangeTOML(`token_url = "https://global-sts.example.com/token"`) + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			token_url = "https://spoke-sts.example.com/token"
		`)
		te, err := p.GetTargetTokenExchange("spoke", cfg, "https://discovered.example.com/token")
		s.Require().NoError(err)
		s.Equal("https://spoke-sts.example.com/token", te.Config.GetTokenURL())
	})

	s.Run("global token_url wins over discovered", func() {
		p, cfg := s.newProvider(globalExchangeTOML(`token_url = "https://global-sts.example.com/token"`) + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		te, err := p.GetTargetTokenExchange("spoke", cfg, "https://discovered.example.com/token")
		s.Require().NoError(err)
		s.Equal("https://global-sts.example.com/token", te.Config.GetTokenURL())
	})

	s.Run("target client_auth replaces the global block as a whole", func() {
		cert, key := writeTempFile(s.T(), "cert.pem"), writeTempFile(s.T(), "key.pem")
		p, cfg := s.newProvider(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke.client_auth]
			method = "private_key_jwt"
			client_id = "spoke-client"
			certificate_file = "` + cert + `"
			private_key_file = "` + key + `"
		`)
		te, err := p.GetTargetTokenExchange("spoke", cfg, "")
		s.Require().NoError(err)
		s.Equal("spoke-client", te.Config.ClientID)
		s.Empty(te.Config.ClientSecret, "global client_secret must not leak into a target with its own client_auth")
		s.Equal(tokenexchange.AuthStyleAssertion, te.Config.AuthStyle)
		s.Equal(cert, te.Config.ClientCertFile)
		s.Equal(key, te.Config.ClientKeyFile)
	})

	s.Run("target public client_id replaces global secret credentials", func() {
		p, cfg := s.newProvider(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke.client_auth]
			client_id = "public-spoke"
		`)
		te, err := p.GetTargetTokenExchange("spoke", cfg, "")
		s.Require().NoError(err)
		s.Equal("public-spoke", te.Config.ClientID)
		s.Empty(te.Config.ClientSecret)
		s.Empty(te.Config.AuthStyle)
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

	s.Run("fails without a strategy anywhere", func() {
		err := newProviderErr(`
			[cluster_provider_configs.kubeconfig.targets.spoke]
			token_url = "https://idp.example.com/token"
			audience = "spoke"
		`)
		s.Require().Error(err)
		s.Contains(err.Error(), `target "spoke": no token exchange strategy configured`)
	})

	s.Run("fails without a token URL or authorization_url", func() {
		err := newProviderErr(`
			[cluster_provider_configs.kubeconfig.targets.spoke]
			strategy = "rfc8693"
			audience = "spoke"
		`)
		s.Require().Error(err)
		s.Contains(err.Error(), `target "spoke": no token URL configured`)
	})

	s.Run("accepts a missing token URL when authorization_url can discover it", func() {
		s.NoError(newProviderErr(`
			authorization_url = "https://idp.example.com/realms/hub"

			[cluster_provider_configs.kubeconfig.targets.spoke]
			strategy = "rfc8693"
			audience = "spoke"
		`))
	})

	s.Run("accepts a target that relies entirely on the global block", func() {
		s.NoError(newProviderErr(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`))
	})

	s.Run("reports every failing target", func() {
		err := newProviderErr(`
			[cluster_provider_configs.kubeconfig.targets.a]
			audience = "a"
			[cluster_provider_configs.kubeconfig.targets.b]
			audience = "b"
		`)
		s.Require().Error(err)
		s.Contains(err.Error(), `target "a"`)
		s.Contains(err.Error(), `target "b"`)
	})
}

func (s *KubeConfigProviderTokenExchangeSuite) TestCache() {
	s.Run("same resolved values reuse the cached exchange", func() {
		p, cfg := s.newProvider(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		first, err := p.GetTargetTokenExchange("spoke", cfg, "https://idp.example.com/token")
		s.Require().NoError(err)
		second, err := p.GetTargetTokenExchange("spoke", cfg, "https://idp.example.com/token")
		s.Require().NoError(err)
		s.Same(first, second)
	})

	s.Run("rotated discovered token URL rebuilds the exchange", func() {
		p, cfg := s.newProvider(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		old, err := p.GetTargetTokenExchange("spoke", cfg, "https://old.example.com/token")
		s.Require().NoError(err)
		rotated, err := p.GetTargetTokenExchange("spoke", cfg, "https://new.example.com/token")
		s.Require().NoError(err)
		s.NotSame(old, rotated)
		s.Equal("https://new.example.com/token", rotated.Config.GetTokenURL())
		s.Equal("https://old.example.com/token", old.Config.GetTokenURL(), "old exchange is immutable")
	})

	s.Run("reloaded global credentials rebuild the exchange", func() {
		p, cfg := s.newProvider(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		before, err := p.GetTargetTokenExchange("spoke", cfg, "")
		s.Require().NoError(err)
		s.Equal("global-secret", before.Config.ClientSecret)

		rotatedCfg := readTestConfig(s.T(), s.preamble()+strings.Replace(globalExchangeTOML(), `"global-secret"`, `"global-secret-rotated"`, 1)+`
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		after, err := p.GetTargetTokenExchange("spoke", rotatedCfg, "")
		s.Require().NoError(err)
		s.NotSame(before, after)
		s.Equal("global-secret-rotated", after.Config.ClientSecret)
	})

	s.Run("Close clears the cache", func() {
		p, cfg := s.newProvider(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		_, err := p.GetTargetTokenExchange("spoke", cfg, "")
		s.Require().NoError(err)
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
	bearer := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer subject-token")

	s.Run("hot reload removing the global block a target depends on errors instead of passing the token through", func() {
		p, _ := s.newProvider(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		// cluster_provider_configs is pinned across SIGHUP but [token_exchange]
		// is not, so the provider can outlive the global block it inherits from.
		reloaded := readTestConfig(s.T(), s.preamble()+`
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		_, err := ExchangeTokenInContext(bearer, reloaded, p, "spoke", nil, "")
		s.Require().Error(err)
		s.Contains(err.Error(), "no token exchange strategy configured")
	})

	s.Run("missing token URL at exchange time errors", func() {
		p, cfg := s.newProvider(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		_, err := ExchangeTokenInContext(bearer, cfg, p, "spoke", nil, "")
		s.Require().Error(err)
		s.Contains(err.Error(), "no token endpoint available")
	})

	s.Run("kubeconfig auth mode drops the token and skips per-target exchange", func() {
		p, _ := s.newProvider(globalExchangeTOML() + `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		cfg := readTestConfig(s.T(), s.preamble()+`
			cluster_auth_mode = "kubeconfig"
		`+globalExchangeTOML()+`
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
		`)
		// No token URL is reachable, so any attempted exchange would error.
		result, err := ExchangeTokenInContext(bearer, cfg, p, "spoke", nil, "")
		s.Require().NoError(err)
		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Empty(auth)
	})
}

func TestKubeConfigProviderTokenExchange(t *testing.T) {
	suite.Run(t, new(KubeConfigProviderTokenExchangeSuite))
}

// createTestKubeconfigFile writes a single-context kubeconfig and returns a
// TOML-safe path to it.
func createTestKubeconfigFile(t testing.TB, contextName string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	content := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: ` + contextName + `
current-context: ` + contextName + `
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
