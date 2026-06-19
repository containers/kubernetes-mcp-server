package kubernetes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	requests []observedTokenRequest
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

		s.Require().Len(authServer.requests, 2)
		s.Equal(observedTokenRequest{
			clientID:     "old-client",
			clientSecret: "old-secret",
			audience:     "old-audience",
			scope:        "old-scope",
		}, authServer.requests[0])
		s.Equal(observedTokenRequest{
			clientID:     "new-client",
			clientSecret: "new-secret",
			audience:     "new-audience",
			scope:        "new-scope",
		}, authServer.requests[1])
	})
}

func (s *TokenExchangingProviderSuite) TestGetOrBuildStsConfig() {
	s.Run("rebuilds cached config when STS fields change without token URL change", func() {
		snap := s.newSnapshot()
		cfg := config.Default()
		cfg.TokenExchangeStrategy = tokenexchange.StrategyRFC8693
		cfg.StsClientId = "old-client"
		cfg.StsClientSecret = "old-secret"
		cfg.StsAudience = "old-audience"
		cfg.StsScopes = []string{"old-scope"}
		cfg.CertificateAuthority = "/old-ca.pem"

		p := &tokenExchangingProvider{
			baseConfigProvider: func() api.BaseConfig {
				return cfg
			},
		}

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
			authServer.requests = append(authServer.requests, observedTokenRequest{
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

func TestTokenExchangingProvider(t *testing.T) {
	suite.Run(t, new(TokenExchangingProviderSuite))
}
