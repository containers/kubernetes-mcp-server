package kubernetes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func (s *TokenExchangeRoutingSuite) TestGlobalTokenExchangeRouting() {
	s.Run("kubeconfig mode clears OAuth token", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = api.ClusterAuthKubeconfig

		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer original-token")
		result, err := ExchangeTokenInContext(ctx, cfg, fakeDerivedProvider{}, "", nil)
		s.Require().NoError(err)

		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("", auth)
	})

	s.Run("passthrough mode preserves token", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = api.ClusterAuthPassthrough

		ctx := context.Background()
		ctx = context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer original-token")
		result, err := ExchangeTokenInContext(ctx, cfg, fakeDerivedProvider{}, "", nil)
		s.Require().NoError(err)

		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("Bearer original-token", auth)
	})

	s.Run("auto-detect defaults to passthrough", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = "" // auto-detect

		ctx := context.Background()
		ctx = context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer original-token")
		result, err := ExchangeTokenInContext(ctx, cfg, fakeDerivedProvider{}, "", nil)
		s.Require().NoError(err)

		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("Bearer original-token", auth)
	})

	s.Run("configured exchange without a token endpoint returns an error", func() {
		cfg := config.Default()
		cfg.TokenExchange = &config.TokenExchangeConfig{Strategy: tokenexchange.StrategyRFC8693}

		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer original-token")
		_, err := ExchangeTokenInContext(ctx, cfg, fakeDerivedProvider{}, "", nil)
		s.Require().Error(err)
		s.Contains(err.Error(), "no token endpoint available from OIDC provider")
		s.Contains(err.Error(), `strategy "rfc8693"`)
	})

	s.Run("ignores an orphaned built config when declarative config is absent", func() {
		cfg := config.Default()
		built := &tokenexchange.TargetTokenExchangeConfig{TokenURL: "https://example.com/token"}

		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer original-token")
		result, err := ExchangeTokenInContext(ctx, cfg, fakeDerivedProvider{}, "", built)
		s.Require().NoError(err)
		s.Equal("Bearer original-token", result.Value(OAuthAuthorizationHeader))
	})
}

func (s *TokenExchangeRoutingSuite) TestRequireTLS_BlocksHTTPTokenExchange() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "exchanged-token",
			"token_type":   "Bearer",
		})
	}))
	defer server.Close()

	s.Run("global exchange rejects http token URL when require_tls is true", func() {
		cfg := config.Default()
		cfg.RequireTLS = true

		cachedConfig := &tokenexchange.TargetTokenExchangeConfig{
			TokenURL:     server.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		}

		_, err := exchangeToken(context.Background(), cfg, "subject-token", "", tokenexchange.StrategyRFC8693, cachedConfig)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})

	s.Run("global exchange allows http token URL when require_tls is false", func() {
		cfg := config.Default()
		cfg.RequireTLS = false

		cachedConfig := &tokenexchange.TargetTokenExchangeConfig{
			TokenURL:     server.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		}

		result, err := exchangeToken(context.Background(), cfg, "subject-token", "", tokenexchange.StrategyRFC8693, cachedConfig)
		s.Require().NoError(err)
		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("Bearer exchanged-token", auth)
	})
}

// fakeTokenExchangeProvider implements the optional TokenExchangeProvider
// interface so ExchangeTokenInContext takes the per-target exCfg branch.
type fakeTokenExchangeProvider struct {
	fakeDerivedProvider
	exchangeConfig *tokenexchange.TargetTokenExchangeConfig
	strategy       string
}

func (f fakeTokenExchangeProvider) GetTokenExchangeConfig(string) *tokenexchange.TargetTokenExchangeConfig {
	return f.exchangeConfig
}
func (f fakeTokenExchangeProvider) GetTokenExchangeStrategy() string { return f.strategy }
func (f fakeTokenExchangeProvider) AnyTargetHasGVKs(context.Context, []schema.GroupVersionKind) bool {
	return true
}
func (f fakeTokenExchangeProvider) IsTargetCompatibilityToolFiltersEnabled() bool {
	return false
}

func (s *TokenExchangeRoutingSuite) TestRequireTLS_BlocksExCfgTokenExchange() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "exchanged-token",
			"token_type":   "Bearer",
		})
	}))
	defer server.Close()

	newProvider := func() fakeTokenExchangeProvider {
		return fakeTokenExchangeProvider{
			exchangeConfig: &tokenexchange.TargetTokenExchangeConfig{
				TokenURL:     server.URL,
				ClientID:     "test-client",
				ClientSecret: "test-secret",
			},
			strategy: tokenexchange.StrategyRFC8693,
		}
	}
	ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer subject-token")

	s.Run("rejects http token URL via per-target config when require_tls is true", func() {
		cfg := config.Default()
		cfg.RequireTLS = true

		_, err := ExchangeTokenInContext(ctx, cfg, newProvider(), "", nil)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})

	s.Run("allows http token URL via per-target config when require_tls is false", func() {
		cfg := config.Default()
		cfg.RequireTLS = false

		result, err := ExchangeTokenInContext(ctx, cfg, newProvider(), "", nil)
		s.Require().NoError(err)
		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("Bearer exchanged-token", auth)
	})
}

func (s *TokenExchangeRoutingSuite) TestUnknownStrategyReturnsError() {
	cfg := config.Default()
	ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer subject-token")
	provider := fakeTokenExchangeProvider{
		exchangeConfig: &tokenexchange.TargetTokenExchangeConfig{TokenURL: "https://example.com/token"},
		strategy:       "unknown",
	}

	_, err := ExchangeTokenInContext(ctx, cfg, provider, "", nil)
	s.Require().Error(err)
	s.Contains(err.Error(), `token exchange strategy "unknown" not found`)
}

func (s *TokenExchangeRoutingSuite) TestInvalidPerTargetClientAuthReturnsError() {
	cfg := config.Default()
	ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer subject-token")
	provider := fakeTokenExchangeProvider{
		exchangeConfig: &tokenexchange.TargetTokenExchangeConfig{
			TokenURL:  "https://example.com/token",
			AuthStyle: "unknown",
		},
		strategy: tokenexchange.StrategyRFC8693,
	}

	_, err := ExchangeTokenInContext(ctx, cfg, provider, "", nil)
	s.Require().Error(err)
	s.Contains(err.Error(), `invalid auth_style "unknown"`)
}

func TestTokenExchangeRouting(t *testing.T) {
	suite.Run(t, new(TokenExchangeRoutingSuite))
}
