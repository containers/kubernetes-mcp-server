package kubernetes

import (
	"context"
	"strings"
	"sync"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

type tokenExchangingProvider struct {
	provider           Provider
	baseConfigProvider func() api.BaseConfig
	oauthState         *oauth.State
	// stsConfig is cached and reused across calls so that assertion caching
	// in TargetTokenExchangeConfig is effective. Rebuilt when the token URL or
	// any STS/TLS config field changes after a reload.
	stsConfig    *tokenexchange.TargetTokenExchangeConfig
	stsConfigMu  sync.Mutex
	stsConfigKey stsConfigCacheKey
}

var _ Provider = &tokenExchangingProvider{}

func newTokenExchangingProvider(
	provider Provider,
	baseConfigProvider func() api.BaseConfig,
	oauthState *oauth.State,
) Provider {
	return &tokenExchangingProvider{
		provider:           provider,
		baseConfigProvider: baseConfigProvider,
		oauthState:         oauthState,
	}
}

func (p *tokenExchangingProvider) GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error) {
	snap := p.oauthState.Load()
	if snap == nil {
		return p.provider.GetDerivedKubernetes(ctx, target)
	}
	baseConfig := p.baseConfig()
	if baseConfig == nil {
		return p.provider.GetDerivedKubernetes(ctx, target)
	}
	stsConfig := p.getOrBuildStsConfig(ctx, snap, baseConfig)
	ctx, err := ExchangeTokenInContext(ctx, baseConfig, snap.OIDCProvider, snap.HTTPClient, p.provider, target, stsConfig)
	if err != nil {
		return nil, err
	}
	return p.provider.GetDerivedKubernetes(ctx, target)
}

func (p *tokenExchangingProvider) baseConfig() api.BaseConfig {
	if p.baseConfigProvider == nil {
		return nil
	}
	return p.baseConfigProvider()
}

// getOrBuildStsConfig returns a cached STS config, rebuilding it when the
// OIDC provider's token URL or STS/TLS config fields change.
func (p *tokenExchangingProvider) getOrBuildStsConfig(ctx context.Context, snap *oauth.Snapshot, baseConfig api.BaseConfig) *tokenexchange.TargetTokenExchangeConfig {
	logger := klog.FromContext(ctx)

	strategy := baseConfig.GetStsStrategy()
	if strategy == "" {
		return nil
	}

	var tokenURL string
	if snap.OIDCProvider != nil {
		if endpoint := snap.OIDCProvider.Endpoint(); endpoint.TokenURL != "" {
			tokenURL = endpoint.TokenURL
		}
	}
	if tokenURL == "" {
		klogutil.LogWarn(logger, "token exchange strategy configured but OIDC provider returned empty token URL",
			klogutil.Field("token_exchange.strategy", strategy),
		)
		return nil
	}

	p.stsConfigMu.Lock()
	defer p.stsConfigMu.Unlock()

	key := newStsConfigCacheKey(tokenURL, baseConfig)
	if p.stsConfig != nil && p.stsConfigKey == key {
		return p.stsConfig
	}

	authStyle := baseConfig.GetStsAuthStyle()
	if authStyle == "" {
		authStyle = tokenexchange.AuthStyleParams
	}
	scopes := append([]string(nil), baseConfig.GetStsScopes()...)

	cfg := &tokenexchange.TargetTokenExchangeConfig{
		TokenURL:           tokenURL,
		ClientID:           baseConfig.GetStsClientId(),
		ClientSecret:       baseConfig.GetStsClientSecret(),
		Audience:           baseConfig.GetStsAudience(),
		Scopes:             scopes,
		AuthStyle:          authStyle,
		ClientCertFile:     baseConfig.GetStsClientCertFile(),
		ClientKeyFile:      baseConfig.GetStsClientKeyFile(),
		FederatedTokenFile: baseConfig.GetStsFederatedTokenFile(),
		CAFile:             baseConfig.GetCertificateAuthority(),
	}
	if err := cfg.Validate(); err != nil {
		logger.Error(
			err,
			"STS config validation failed, token exchange will be attempted per-request but will likely fail with the same error",
		)
		return nil
	}

	p.stsConfig = cfg
	p.stsConfigKey = key
	return p.stsConfig
}

type stsConfigCacheKey struct {
	TokenURL           string
	Strategy           string
	ClientID           string
	ClientSecret       string
	Audience           string
	Scopes             string
	AuthStyle          string
	ClientCertFile     string
	ClientKeyFile      string
	FederatedTokenFile string
	CAFile             string
}

func newStsConfigCacheKey(tokenURL string, cfg api.BaseConfig) stsConfigCacheKey {
	return stsConfigCacheKey{
		TokenURL:           tokenURL,
		Strategy:           cfg.GetStsStrategy(),
		ClientID:           cfg.GetStsClientId(),
		ClientSecret:       cfg.GetStsClientSecret(),
		Audience:           cfg.GetStsAudience(),
		Scopes:             strings.Join(cfg.GetStsScopes(), "\x00"),
		AuthStyle:          cfg.GetStsAuthStyle(),
		ClientCertFile:     cfg.GetStsClientCertFile(),
		ClientKeyFile:      cfg.GetStsClientKeyFile(),
		FederatedTokenFile: cfg.GetStsFederatedTokenFile(),
		CAFile:             cfg.GetCertificateAuthority(),
	}
}

func (p *tokenExchangingProvider) IsOpenShift(ctx context.Context) bool {
	return p.provider.IsOpenShift(ctx)
}

func (p *tokenExchangingProvider) IsMultiTarget() bool {
	return p.provider.IsMultiTarget()
}

func (p *tokenExchangingProvider) GetTargets(ctx context.Context) ([]string, error) {
	return p.provider.GetTargets(ctx)
}

func (p *tokenExchangingProvider) GetDefaultTarget() string {
	return p.provider.GetDefaultTarget()
}

func (p *tokenExchangingProvider) GetTargetParameterName() string {
	return p.provider.GetTargetParameterName()
}

func (p *tokenExchangingProvider) WatchTargets(ctx context.Context, reload McpReload) {
	p.provider.WatchTargets(ctx, reload)
}

func (p *tokenExchangingProvider) Close() {
	p.provider.Close()
}

func (p *tokenExchangingProvider) HasGVKs(ctx context.Context, gvks []schema.GroupVersionKind) bool {
	return p.provider.HasGVKs(ctx, gvks)
}
