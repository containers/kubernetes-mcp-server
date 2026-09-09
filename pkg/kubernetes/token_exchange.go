package kubernetes

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

// ExchangeTokenInContext exchanges the OAuth token in the context for a token
// that can access the target cluster. Per-target configuration takes precedence
// over global configuration.
func ExchangeTokenInContext(
	ctx context.Context,
	cfg *config.Config,
	provider Provider,
	target string,
	globalConfig *tokenexchange.TargetTokenExchangeConfig,
) (context.Context, error) {
	auth, ok := ctx.Value(OAuthAuthorizationHeader).(string)
	if !ok || !strings.HasPrefix(auth, "Bearer ") {
		return ctx, nil
	}
	subjectToken := strings.TrimPrefix(auth, "Bearer ")

	if tep, ok := provider.(TokenExchangeProvider); ok {
		if targetConfig := tep.GetTokenExchangeConfig(target); targetConfig != nil {
			return exchangeToken(ctx, cfg, subjectToken, target, tep.GetTokenExchangeStrategy(), targetConfig)
		}
	}

	switch cfg.ResolveClusterAuthMode() {
	case api.ClusterAuthKubeconfig:
		return context.WithValue(ctx, OAuthAuthorizationHeader, ""), nil
	case api.ClusterAuthPassthrough:
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
