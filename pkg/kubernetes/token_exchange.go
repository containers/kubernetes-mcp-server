package kubernetes

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

// ExchangeTokenInContext exchanges the OAuth token in the context for a token
// that can access the target cluster. In passthrough mode a per-target exchange
// from a TokenExchangeProvider takes precedence over the global configuration;
// in kubeconfig mode the token is dropped and no exchange is performed.
// discoveredTokenURL is the token endpoint discovered from authorization_url,
// or empty when unavailable.
func ExchangeTokenInContext(
	ctx context.Context,
	cfg *config.Config,
	provider Provider,
	target string,
	globalConfig *tokenexchange.TargetTokenExchangeConfig,
	discoveredTokenURL string,
) (context.Context, error) {
	auth, ok := ctx.Value(OAuthAuthorizationHeader).(string)
	if !ok || !strings.HasPrefix(auth, "Bearer ") {
		return ctx, nil
	}
	subjectToken := strings.TrimPrefix(auth, "Bearer ")

	switch cfg.ResolveClusterAuthMode() {
	case config.ClusterAuthKubeconfig:
		return context.WithValue(ctx, OAuthAuthorizationHeader, ""), nil
	case config.ClusterAuthPassthrough:
		if tep, ok := provider.(TokenExchangeProvider); ok {
			te, err := tep.GetTargetTokenExchange(target, cfg, discoveredTokenURL)
			if err != nil {
				return ctx, err
			}
			if te != nil {
				if te.Config.GetTokenURL() == "" {
					return ctx, fmt.Errorf("token exchange failed for target %q: no token endpoint available from OIDC provider", target)
				}
				return exchangeToken(ctx, cfg, subjectToken, target, te.Strategy, te.Config)
			}
		}
		global := cfg.GetTokenExchangeConfig()
		if global == nil {
			return ctx, nil
		}
		if globalConfig == nil {
			return ctx, fmt.Errorf("token exchange failed using strategy %q: no token endpoint available from OIDC provider", global.Strategy.Get())
		}
		return exchangeToken(ctx, cfg, subjectToken, target, global.Strategy.Get(), globalConfig)
	default:
		return ctx, fmt.Errorf("unknown cluster_auth_mode %q", cfg.ResolveClusterAuthMode())
	}
}

func exchangeToken(
	ctx context.Context,
	cfg *config.Config,
	subjectToken string,
	target, strategy string,
	teCfg *tokenexchange.TargetTokenExchangeConfig,
) (context.Context, error) {
	if err := teCfg.Validate(); err != nil {
		return ctx, fmt.Errorf("invalid token exchange configuration for strategy %q: %w", strategy, err)
	}
	exchanger, ok := tokenexchange.GetTokenExchanger(strategy)
	if !ok {
		return ctx, fmt.Errorf("token exchange strategy %q not found", strategy)
	}
	teCfg.SetRequireTLS(func() bool { return cfg.RequireTLS.Get() })
	exchanged, err := exchanger.Exchange(ctx, teCfg, subjectToken)
	if err != nil {
		if target == "" {
			return ctx, fmt.Errorf("token exchange failed using strategy %q: %w", strategy, err)
		}
		return ctx, fmt.Errorf("token exchange failed for target %q: %w", target, err)
	}
	return context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer "+exchanged.AccessToken), nil
}
