package kubernetes

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"k8s.io/klog/v2"
)

// ExchangeTokenInContext exchanges the OAuth token in the context for a token
// that can access the target cluster. The optional stsConfig parameter allows
// callers to reuse a TargetTokenExchangeConfig across calls to benefit from
// assertion caching (pass nil to build a fresh config each time).
func ExchangeTokenInContext(
	ctx context.Context,
	baseConfig api.BaseConfig,
	oidcProvider *oidc.Provider,
	httpClient *http.Client,
	provider Provider,
	target string,
	stsConfig *tokenexchange.TargetTokenExchangeConfig,
) (context.Context, error) {
	auth, ok := ctx.Value(OAuthAuthorizationHeader).(string)
	if !ok || !strings.HasPrefix(auth, "Bearer ") {
		return ctx, nil
	}
	subjectToken := strings.TrimPrefix(auth, "Bearer ")

	tep, ok := provider.(TokenExchangeProvider)
	if !ok {
		return stsExchangeTokenInContext(ctx, baseConfig, oidcProvider, httpClient, subjectToken, stsConfig)
	}

	exCfg := tep.GetTokenExchangeConfig(target)
	if exCfg == nil {
		return stsExchangeTokenInContext(ctx, baseConfig, oidcProvider, httpClient, subjectToken, stsConfig)
	}

	exchanger, ok := tokenexchange.GetTokenExchanger(tep.GetTokenExchangeStrategy())
	if !ok {
		klog.Warningf("token exchange strategy %q not found in registry", tep.GetTokenExchangeStrategy())
		return stsExchangeTokenInContext(ctx, baseConfig, oidcProvider, httpClient, subjectToken, stsConfig)
	}

	exchanged, err := exchanger.Exchange(ctx, exCfg, subjectToken)
	if err != nil {
		return ctx, fmt.Errorf("token exchange failed for target %q: %w", target, err)
	}
	return context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer "+exchanged.AccessToken), nil
}

func stsExchangeTokenInContext(
	ctx context.Context,
	baseConfig api.BaseConfig,
	oidcProvider *oidc.Provider,
	httpClient *http.Client,
	token string,
	stsConfig *tokenexchange.TargetTokenExchangeConfig,
) (context.Context, error) {
	switch baseConfig.ResolveClusterAuthMode() {
	case api.ClusterAuthKubeconfig:
		return context.WithValue(ctx, OAuthAuthorizationHeader, ""), nil

	case api.ClusterAuthPassthrough:
		exchangedToken, err := exchangePassthroughToken(ctx, baseConfig, oidcProvider, httpClient, token, stsConfig)
		if err != nil {
			return ctx, err
		}
		return context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer "+exchangedToken), nil

	default:
		return ctx, fmt.Errorf("unknown cluster_auth_mode %q", baseConfig.ResolveClusterAuthMode())
	}
}

// exchangePassthroughToken exchanges the user token if a strategy or STS is configured,
// otherwise returns the original token unchanged.
func exchangePassthroughToken(
	ctx context.Context,
	baseConfig api.BaseConfig,
	oidcProvider *oidc.Provider,
	httpClient *http.Client,
	token string,
	stsConfig *tokenexchange.TargetTokenExchangeConfig,
) (string, error) {
	if strategy := baseConfig.GetStsStrategy(); strategy != "" {
		return doTokenExchange(ctx, token, func(ctx context.Context) (context.Context, error) {
			return strategyBasedTokenExchange(ctx, baseConfig, oidcProvider, httpClient, token, strategy, stsConfig)
		})
	}

	sts := NewFromConfig(baseConfig, oidcProvider)
	if sts.IsEnabled() {
		return doTokenExchange(ctx, token, func(ctx context.Context) (context.Context, error) {
			if httpClient != nil {
				ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
			}
			exchangedToken, err := sts.ExternalAccountTokenExchange(ctx, &oauth2.Token{
				AccessToken: token,
				TokenType:   "Bearer",
			})
			if err != nil {
				return ctx, fmt.Errorf("built-in STS exchange: %w", err)
			}
			return context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer "+exchangedToken.AccessToken), nil
		})
	}

	return token, nil
}

// doTokenExchange runs an exchange function and extracts the Bearer token from the resulting context.
// Falls back to the original token if the exchange doesn't produce one.
func doTokenExchange(
	ctx context.Context,
	token string,
	exchangeFn func(ctx context.Context) (context.Context, error),
) (string, error) {
	exchangedCtx, err := exchangeFn(ctx)
	if err != nil {
		return "", err
	}
	if auth, ok := exchangedCtx.Value(OAuthAuthorizationHeader).(string); ok && strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer "), nil
	}
	return token, nil
}

func strategyBasedTokenExchange(
	ctx context.Context,
	baseConfig api.BaseConfig,
	oidcProvider *oidc.Provider,
	httpClient *http.Client,
	token string,
	strategy string,
	cachedConfig *tokenexchange.TargetTokenExchangeConfig,
) (context.Context, error) {
	exchanger, ok := tokenexchange.GetTokenExchanger(strategy)
	if !ok {
		return ctx, fmt.Errorf("token exchange strategy %q not found", strategy)
	}

	cfg := cachedConfig
	if cfg == nil {
		// Build token URL from OIDC provider
		var tokenURL string
		if oidcProvider != nil {
			if endpoint := oidcProvider.Endpoint(); endpoint.TokenURL != "" {
				tokenURL = endpoint.TokenURL
			}
		}
		if tokenURL == "" {
			return ctx, fmt.Errorf("token exchange failed: no token URL available from OIDC provider")
		}

		authStyle := baseConfig.GetStsAuthStyle()
		if authStyle == "" {
			authStyle = tokenexchange.AuthStyleParams
		}

		cfg = &tokenexchange.TargetTokenExchangeConfig{
			TokenURL:       tokenURL,
			ClientID:       baseConfig.GetStsClientId(),
			ClientSecret:   baseConfig.GetStsClientSecret(),
			Audience:       baseConfig.GetStsAudience(),
			Scopes:         baseConfig.GetStsScopes(),
			AuthStyle:      authStyle,
			ClientCertFile: baseConfig.GetStsClientCertFile(),
			ClientKeyFile:  baseConfig.GetStsClientKeyFile(),
			CAFile:         baseConfig.GetCertificateAuthority(),
		}
		if err := cfg.Validate(); err != nil {
			return ctx, fmt.Errorf("token exchange config validation: %w", err)
		}
	}

	if httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}

	exchanged, err := exchanger.Exchange(ctx, cfg, token)
	if err != nil {
		return ctx, fmt.Errorf("token exchange with strategy %q: %w", strategy, err)
	}
	return context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer "+exchanged.AccessToken), nil
}
