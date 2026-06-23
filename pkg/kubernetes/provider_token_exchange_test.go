package kubernetes

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type TokenExchangingProviderSuite struct {
	suite.Suite
}

type observedTokenRequest struct {
	clientID     string
	clientSecret string
	audience     string
	scope        string
}

type exchangeTestOIDCServer struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []observedTokenRequest
}

func (a *exchangeTestOIDCServer) record(req observedTokenRequest) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests = append(a.requests, req)
}

func (a *exchangeTestOIDCServer) recordedRequests() []observedTokenRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]observedTokenRequest(nil), a.requests...)
}

type fakeDerivedProvider struct{}

func (fakeDerivedProvider) IsOpenShift(context.Context) bool             { return false }
func (fakeDerivedProvider) IsMultiTarget() bool                          { return false }
func (fakeDerivedProvider) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }
func (fakeDerivedProvider) GetDefaultTarget() string                     { return "" }
func (fakeDerivedProvider) GetTargetParameterName() string               { return "" }
func (fakeDerivedProvider) WatchTargets(context.Context, McpReload)      {}
func (fakeDerivedProvider) Close()                                       {}
func (fakeDerivedProvider) GetDerivedKubernetes(context.Context, string) (*Kubernetes, error) {
	return &Kubernetes{}, nil
}
func (fakeDerivedProvider) HasGVKs(context.Context, []schema.GroupVersionKind) bool {
	return true
}

func (s *TokenExchangingProviderSuite) TestGetDerivedKubernetes() {
	s.Run("uses reloaded STS config from live config provider", func() {
		authServer := s.newExchangeTestOIDCServer()
		cfg := config.Default()
		cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
		cfg.StsClientId = "old-client"
		cfg.StsClientSecret = "old-secret"
		cfg.StsAudience = "old-audience"
		cfg.StsScopes = []string{"old-scope"}

		provider, err := oidc.NewProvider(context.Background(), authServer.server.URL)
		s.Require().NoError(err)
		oauthState := oauth.NewState(&oauth.Snapshot{OIDCProvider: provider})
		baseConfigProvider := func() api.BaseConfig {
			return cfg
		}
		wrapped := newTokenExchangingProvider(fakeDerivedProvider{}, baseConfigProvider, oauthState)

		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer original-token")
		_, err = wrapped.GetDerivedKubernetes(ctx, "")
		s.Require().NoError(err)

		cfg = config.Default()
		cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
		cfg.StsClientId = "new-client"
		cfg.StsClientSecret = "new-secret"
		cfg.StsAudience = "new-audience"
		cfg.StsScopes = []string{"new-scope"}

		_, err = wrapped.GetDerivedKubernetes(ctx, "")
		s.Require().NoError(err)

		requests := authServer.recordedRequests()
		s.Require().Len(requests, 2)
		s.Equal(observedTokenRequest{
			clientID:     "old-client",
			clientSecret: "old-secret",
			audience:     "old-audience",
			scope:        "old-scope",
		}, requests[0])
		s.Equal(observedTokenRequest{
			clientID:     "new-client",
			clientSecret: "new-secret",
			audience:     "new-audience",
			scope:        "new-scope",
		}, requests[1])
	})
}

func (s *TokenExchangingProviderSuite) TestGetOrBuildStsConfig() {
	newProvider := func(cfg *config.StaticConfig) *tokenExchangingProvider {
		return &tokenExchangingProvider{
			baseConfigProvider: func() api.BaseConfig {
				return cfg
			},
		}
	}

	s.Run("reuses the cached config when nothing changes", func() {
		snap := s.newSnapshot()
		cfg := config.Default()
		cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
		cfg.StsClientId = "client"
		cfg.StsAudience = "audience"
		p := newProvider(cfg)

		first := p.getOrBuildStsConfig(context.Background(), snap, cfg)
		s.Require().NotNil(first)
		second := p.getOrBuildStsConfig(context.Background(), snap, cfg)
		s.Same(first, second, "unchanged config must reuse the cached struct so assertion caching stays effective")
	})

	s.Run("rebuilds cached config when STS fields change without token URL change", func() {
		snap := s.newSnapshot()
		cfg := config.Default()
		cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
		cfg.StsClientId = "old-client"
		cfg.StsClientSecret = "old-secret"
		cfg.StsAudience = "old-audience"
		cfg.StsScopes = []string{"old-scope"}
		cfg.CertificateAuthority = "/old-ca.pem"
		p := newProvider(cfg)

		first := p.getOrBuildStsConfig(context.Background(), snap, cfg)
		s.Require().NotNil(first)
		s.Equal("old-client", first.ClientID)

		cfg.StsClientId = "new-client"
		cfg.StsClientSecret = "new-secret"
		cfg.StsAudience = "new-audience"
		cfg.StsScopes = []string{"new-scope"}
		cfg.CertificateAuthority = "/new-ca.pem"

		second := p.getOrBuildStsConfig(context.Background(), snap, cfg)
		s.Require().NotNil(second)
		s.NotSame(first, second)
		s.Equal("new-client", second.ClientID)
		s.Equal("new-secret", second.ClientSecret)
		s.Equal("new-audience", second.Audience)
		s.Equal([]string{"new-scope"}, second.Scopes)
		s.Equal("/new-ca.pem", second.CAFile)
	})

	s.Run("rebuilds when a single rotated field changes", func() {
		s.Run("sts_auth_style", func() {
			snap := s.newSnapshot()
			cfg := config.Default()
			cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
			cfg.StsClientId = "client"
			cfg.StsAudience = "audience"
			cfg.StsAuthStyle = tokenexchange.AuthStyleParams
			p := newProvider(cfg)

			first := p.getOrBuildStsConfig(context.Background(), snap, cfg)
			s.Require().NotNil(first)

			cfg.StsAuthStyle = tokenexchange.AuthStyleHeader
			second := p.getOrBuildStsConfig(context.Background(), snap, cfg)
			s.Require().NotNil(second)
			s.NotSame(first, second)
			s.Equal(tokenexchange.AuthStyleHeader, second.AuthStyle)
		})

		s.Run("sts_client_cert_file and sts_client_key_file", func() {
			snap := s.newSnapshot()
			cfg := config.Default()
			cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
			cfg.StsClientId = "client"
			cfg.StsAudience = "audience"
			cfg.StsAuthStyle = tokenexchange.AuthStyleAssertion
			cfg.StsClientCertFile = "/old-cert.pem"
			cfg.StsClientKeyFile = "/old-key.pem"
			p := newProvider(cfg)

			first := p.getOrBuildStsConfig(context.Background(), snap, cfg)
			s.Require().NotNil(first)

			cfg.StsClientCertFile = "/new-cert.pem"
			cfg.StsClientKeyFile = "/new-key.pem"
			second := p.getOrBuildStsConfig(context.Background(), snap, cfg)
			s.Require().NotNil(second)
			s.NotSame(first, second)
			s.Equal("/new-cert.pem", second.ClientCertFile)
			s.Equal("/new-key.pem", second.ClientKeyFile)
		})

		s.Run("sts_federated_token_file", func() {
			snap := s.newSnapshot()
			cfg := config.Default()
			cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
			cfg.StsClientId = "client"
			cfg.StsAudience = "audience"
			cfg.StsAuthStyle = tokenexchange.AuthStyleFederated
			cfg.StsFederatedTokenFile = "/old-token"
			p := newProvider(cfg)

			first := p.getOrBuildStsConfig(context.Background(), snap, cfg)
			s.Require().NotNil(first)

			cfg.StsFederatedTokenFile = "/new-token"
			second := p.getOrBuildStsConfig(context.Background(), snap, cfg)
			s.Require().NotNil(second)
			s.NotSame(first, second)
			s.Equal("/new-token", second.FederatedTokenFile)
		})

		s.Run("require_tls", func() {
			snap := s.newSnapshot()
			cfg := config.Default()
			cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
			cfg.StsClientId = "client"
			cfg.StsAudience = "audience"
			cfg.RequireTLS = false
			p := newProvider(cfg)

			first := p.getOrBuildStsConfig(context.Background(), snap, cfg)
			s.Require().NotNil(first)

			cfg.RequireTLS = true
			second := p.getOrBuildStsConfig(context.Background(), snap, cfg)
			s.Require().NotNil(second)
			s.NotSame(first, second)
		})
	})

	s.Run("wires require_tls enforcement into the built config", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		snap := s.newSnapshot()
		cfg := config.Default()
		cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
		cfg.StsClientId = "client"
		cfg.RequireTLS = true
		p := newProvider(cfg)

		built := p.getOrBuildStsConfig(context.Background(), snap, cfg)
		s.Require().NotNil(built)

		// getOrBuildStsConfig must wire the enforcer into the config it returns,
		// or the http token endpoint slips through when require_tls is on.
		client, err := built.HTTPClient()
		s.Require().NoError(err)
		_, err = client.Get(server.URL)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})
}

func (s *TokenExchangingProviderSuite) newExchangeTestOIDCServer() *exchangeTestOIDCServer {
	s.T().Helper()

	authServer := &exchangeTestOIDCServer{}
	authServer.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{
				"issuer": "%s",
				"authorization_endpoint": "%s/authorize",
				"token_endpoint": "%s/token"
			}`, authServer.server.URL, authServer.server.URL, authServer.server.URL)
		case "/token":
			s.Require().NoError(r.ParseForm())
			authServer.record(observedTokenRequest{
				clientID:     r.PostForm.Get(tokenexchange.FormKeyClientID),
				clientSecret: r.PostForm.Get(tokenexchange.FormKeyClientSecret),
				audience:     r.PostForm.Get(tokenexchange.FormKeyAudience),
				scope:        strings.TrimSpace(r.PostForm.Get(tokenexchange.FormKeyScope)),
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"exchanged-token","token_type":"Bearer","expires_in":3600}`))
		default:
			http.NotFound(w, r)
		}
	}))
	s.T().Cleanup(authServer.server.Close)
	return authServer
}

func (s *TokenExchangingProviderSuite) newSnapshot() *oauth.Snapshot {
	s.T().Helper()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"issuer": "%s",
			"authorization_endpoint": "%s/authorize",
			"token_endpoint": "%s/token"
		}`, server.URL, server.URL, server.URL)
	}))
	s.T().Cleanup(server.Close)

	provider, err := oidc.NewProvider(context.Background(), server.URL)
	s.Require().NoError(err)
	return &oauth.Snapshot{OIDCProvider: provider}
}

// tlsTokenServer is an HTTPS STS token endpoint backed by a self-signed
// certificate, used to observe certificate_authority propagation end to end.
type tlsTokenServer struct {
	server *httptest.Server
	mu     sync.Mutex
	count  int
}

func (t *tlsTokenServer) requestCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

func (s *TokenExchangingProviderSuite) TestGetDerivedKubernetesCAFile() {
	s.Run("certificate_authority lets the STS exchange trust the token endpoint", func() {
		tokenServer := s.newTLSTokenServer()
		caFile := s.writeCAFile(tokenServer.server.Certificate())
		oidcProvider := s.newOIDCProviderWithTokenEndpoint(tokenServer.server.URL + "/token")

		cfg := config.Default()
		cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
		cfg.StsClientId = "client"
		cfg.StsAudience = "audience"
		cfg.CertificateAuthority = caFile

		oauthState := oauth.NewState(&oauth.Snapshot{OIDCProvider: oidcProvider})
		wrapped := newTokenExchangingProvider(fakeDerivedProvider{}, func() api.BaseConfig { return cfg }, oauthState)

		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer original-token")
		_, err := wrapped.GetDerivedKubernetes(ctx, "")
		s.Require().NoError(err)
		s.Equal(1, tokenServer.requestCount(), "the exchange must reach the TLS token endpoint when the CA is trusted")
	})

	s.Run("missing certificate_authority fails against the self-signed token endpoint", func() {
		tokenServer := s.newTLSTokenServer()
		oidcProvider := s.newOIDCProviderWithTokenEndpoint(tokenServer.server.URL + "/token")

		cfg := config.Default()
		cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
		cfg.StsClientId = "client"
		cfg.StsAudience = "audience"
		// CertificateAuthority deliberately unset: the self-signed endpoint is untrusted.

		oauthState := oauth.NewState(&oauth.Snapshot{OIDCProvider: oidcProvider})
		wrapped := newTokenExchangingProvider(fakeDerivedProvider{}, func() api.BaseConfig { return cfg }, oauthState)

		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer original-token")
		_, err := wrapped.GetDerivedKubernetes(ctx, "")
		s.Require().Error(err)
		s.Contains(err.Error(), "certificate signed by unknown authority")
		s.Equal(0, tokenServer.requestCount(), "the exchange must not reach an untrusted endpoint")
	})
}

func (s *TokenExchangingProviderSuite) newTLSTokenServer() *tlsTokenServer {
	s.T().Helper()

	tokenServer := &tlsTokenServer{}
	tokenServer.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		tokenServer.mu.Lock()
		tokenServer.count++
		tokenServer.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"exchanged-token","token_type":"Bearer","expires_in":3600}`))
	}))
	s.T().Cleanup(tokenServer.server.Close)
	return tokenServer
}

func (s *TokenExchangingProviderSuite) newOIDCProviderWithTokenEndpoint(tokenEndpoint string) *oidc.Provider {
	s.T().Helper()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"issuer": "%s",
			"authorization_endpoint": "%s/authorize",
			"token_endpoint": "%s"
		}`, server.URL, server.URL, tokenEndpoint)
	}))
	s.T().Cleanup(server.Close)

	provider, err := oidc.NewProvider(context.Background(), server.URL)
	s.Require().NoError(err)
	return provider
}

func (s *TokenExchangingProviderSuite) writeCAFile(cert *x509.Certificate) string {
	s.T().Helper()

	path := filepath.Join(s.T().TempDir(), "ca.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	s.Require().NoError(os.WriteFile(path, certPEM, 0o600))
	return path
}

func TestTokenExchangingProvider(t *testing.T) {
	suite.Run(t, new(TokenExchangingProviderSuite))
}
