package kubernetes

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

// ExchangeTokenInContext exchanges the OAuth token in the context for a token
// that can access the target cluster. Per-target configuration takes precedence
// over global configuration.
func ExchangeTokenInContext(
	ctx context.Context,
	baseConfig api.BaseConfig,
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
			return exchangeToken(ctx, baseConfig, subjectToken, target, tep.GetTokenExchangeStrategy(), targetConfig)
		}
	}

	switch baseConfig.ResolveClusterAuthMode() {
	case api.ClusterAuthKubeconfig:
		return context.WithValue(ctx, OAuthAuthorizationHeader, ""), nil
	case api.ClusterAuthPassthrough:
		global := baseConfig.GetTokenExchangeConfig()
		if global == nil {
			return ctx, nil
		}
		if globalConfig == nil {
			return ctx, fmt.Errorf("token exchange failed using strategy %q: no token endpoint available from OIDC provider", global.GetStrategy())
		}
		return exchangeToken(ctx, baseConfig, subjectToken, target, global.GetStrategy(), globalConfig)
	default:
		return ctx, fmt.Errorf("unknown cluster_auth_mode %q", baseConfig.ResolveClusterAuthMode())
	}
}

func exchangeToken(
	ctx context.Context,
	baseConfig api.BaseConfig,
	subjectToken string,
	target, strategy string,
	cfg *tokenexchange.TargetTokenExchangeConfig,
) (context.Context, error) {
	if err := cfg.Validate(); err != nil {
		return ctx, fmt.Errorf("invalid token exchange configuration for strategy %q: %w", strategy, err)
	}
	exchanger, ok := tokenexchange.GetTokenExchanger(strategy)
	if !ok {
		return ctx, fmt.Errorf("token exchange strategy %q not found", strategy)
	}
	cfg.SetRequireTLS(baseConfig.IsRequireTLS)
	exchanged, err := exchanger.Exchange(ctx, cfg, subjectToken)
	if err != nil {
		if target == "" {
			return ctx, fmt.Errorf("token exchange failed using strategy %q: %w", strategy, err)
		}
		return ctx, fmt.Errorf("token exchange failed for target %q: %w", target, err)
	}
	return context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer "+exchanged.AccessToken), nil
}
