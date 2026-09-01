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
		if globalConfig == nil {
			if baseConfig.GetTokenExchangeConfig() != nil {
				return ctx, fmt.Errorf("token exchange failed for target %q: no token endpoint available from OIDC provider", target)
			}
			return ctx, nil
		}
		global := baseConfig.GetTokenExchangeConfig()
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
	exchanger, ok := tokenexchange.GetTokenExchanger(strategy)
	if !ok {
		return ctx, fmt.Errorf("token exchange strategy %q not found", strategy)
	}
	cfg.SetRequireTLS(baseConfig.IsRequireTLS)
	exchanged, err := exchanger.Exchange(ctx, cfg, subjectToken)
	if err != nil {
		return ctx, fmt.Errorf("token exchange failed for target %q: %w", target, err)
	}
	return context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer "+exchanged.AccessToken), nil
}
