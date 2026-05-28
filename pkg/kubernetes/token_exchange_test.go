package kubernetes

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/stretchr/testify/suite"
)

type TokenExchangeRoutingSuite struct {
	suite.Suite
}

func (s *TokenExchangeRoutingSuite) TestResolveClusterAuthMode() {
	s.Run("defaults to passthrough", func() {
		cfg := config.Default()
		s.Equal(api.ClusterAuthPassthrough, cfg.ResolveClusterAuthMode())
	})

	s.Run("defaults to passthrough regardless of require_oauth", func() {
		cfg := config.Default()
		cfg.RequireOAuth = true
		s.Equal(api.ClusterAuthPassthrough, cfg.ResolveClusterAuthMode())
	})

	s.Run("returns explicit kubeconfig when set", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = api.ClusterAuthKubeconfig
		s.Equal(api.ClusterAuthKubeconfig, cfg.ResolveClusterAuthMode())
	})
}

func (s *TokenExchangeRoutingSuite) TestStsExchangeTokenInContextRouting() {
	s.Run("kubeconfig mode clears OAuth token", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = api.ClusterAuthKubeconfig

		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer original-token")
		result, err := stsExchangeTokenInContext(ctx, cfg, nil, nil, "original-token", nil)
		s.Require().NoError(err)

		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("", auth)
	})

	s.Run("passthrough mode preserves token", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = api.ClusterAuthPassthrough

		ctx := context.Background()
		result, err := stsExchangeTokenInContext(ctx, cfg, nil, nil, "original-token", nil)
		s.Require().NoError(err)

		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("Bearer original-token", auth)
	})

	s.Run("auto-detect defaults to passthrough", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = "" // auto-detect

		ctx := context.Background()
		result, err := stsExchangeTokenInContext(ctx, cfg, nil, nil, "original-token", nil)
		s.Require().NoError(err)

		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("Bearer original-token", auth)
	})
}

func (s *TokenExchangeRoutingSuite) TestResolveStsTokenURL() {
	mockServer := test.NewMockServer()
	authServer := mockServer.Config().Host
	mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{
				"issuer": "%s",
				"authorization_endpoint": "%s/authorize",
				"token_endpoint": "%s/token"
			}`, authServer, authServer, authServer)
		}
	}))
	s.T().Cleanup(mockServer.Close)
	provider, err := oidc.NewProvider(s.T().Context(), authServer)
	s.Require().NoError(err)
	discoveredURL := provider.Endpoint().TokenURL
	s.Require().NotEmpty(discoveredURL, "test prerequisite: OIDC discovery should yield a token endpoint")

	s.Run("explicit sts_token_url wins over discovered endpoint", func() {
		cfg := config.Default()
		cfg.StsTokenURL = "https://explicit-sts.example.com/token"
		got := resolveStsTokenURL(cfg, provider)
		s.Equal("https://explicit-sts.example.com/token", got, "explicit URL must take precedence over OIDC discovery")
	})

	s.Run("falls back to OIDC provider endpoint when sts_token_url is empty", func() {
		cfg := config.Default()
		cfg.StsTokenURL = ""
		got := resolveStsTokenURL(cfg, provider)
		s.Equal(discoveredURL, got, "empty explicit URL should fall back to OIDC-discovered endpoint")
	})

	s.Run("returns empty when neither source is available", func() {
		cfg := config.Default()
		cfg.StsTokenURL = ""
		got := resolveStsTokenURL(cfg, nil)
		s.Equal("", got, "no explicit URL and no provider should yield empty string")
	})
}

func TestTokenExchangeRouting(t *testing.T) {
	suite.Run(t, new(TokenExchangeRoutingSuite))
}
