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

func (s *TokenExchangeRoutingSuite) TestRequireTLS_BlocksHTTPTokenExchange() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "exchanged-token",
			"token_type":   "Bearer",
		})
	}))
	defer server.Close()

	s.Run("strategyBasedTokenExchange rejects http token URL when require_tls is true", func() {
		cfg := config.Default()
		cfg.RequireTLS = true

		cachedConfig := &tokenexchange.TargetTokenExchangeConfig{
			TokenURL:     server.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		}

		_, err := strategyBasedTokenExchange(
			context.Background(), cfg, nil, nil, "subject-token",
			tokenexchange.StrategyRFC8693, cachedConfig,
		)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})

	s.Run("strategyBasedTokenExchange allows http token URL when require_tls is false", func() {
		cfg := config.Default()
		cfg.RequireTLS = false

		cachedConfig := &tokenexchange.TargetTokenExchangeConfig{
			TokenURL:     server.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		}

		result, err := strategyBasedTokenExchange(
			context.Background(), cfg, nil, nil, "subject-token",
			tokenexchange.StrategyRFC8693, cachedConfig,
		)
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

		_, err := ExchangeTokenInContext(ctx, cfg, nil, nil, newProvider(), "", nil)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})

	s.Run("allows http token URL via per-target config when require_tls is false", func() {
		cfg := config.Default()
		cfg.RequireTLS = false

		result, err := ExchangeTokenInContext(ctx, cfg, nil, nil, newProvider(), "", nil)
		s.Require().NoError(err)
		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("Bearer exchanged-token", auth)
	})
}

func TestTokenExchangeRouting(t *testing.T) {
	suite.Run(t, new(TokenExchangeRoutingSuite))
}
