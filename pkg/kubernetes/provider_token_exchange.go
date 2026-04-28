package kubernetes

import (
	"context"
	"sync"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"k8s.io/klog/v2"
)

type tokenExchangingProvider struct {
	provider   Provider
	baseConfig api.BaseConfig
	oauthState *oauth.State
	// stsConfig is cached and reused across calls so that assertion caching
	// in TargetTokenExchangeConfig is effective. Rebuilt when the token URL changes
	// (e.g., after SIGHUP reloads the OIDC provider).
	stsConfig   *tokenexchange.TargetTokenExchangeConfig
	stsConfigMu sync.Mutex
	stsTokenURL string // tracks which token URL the cached config was built for
}

var _ Provider = &tokenExchangingProvider{}

func newTokenExchangingProvider(
	provider Provider,
	baseConfig api.BaseConfig,
	oauthState *oauth.State,
) Provider {
	return &tokenExchangingProvider{
		provider:   provider,
		baseConfig: baseConfig,
		oauthState: oauthState,
	}
}

func (p *tokenExchangingProvider) GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error) {
	snap := p.oauthState.Load()
	if snap == nil {
		return p.provider.GetDerivedKubernetes(ctx, target)
	}
	stsConfig := p.getOrBuildStsConfig(snap)
	ctx, err := ExchangeTokenInContext(ctx, p.baseConfig, snap.OIDCProvider, snap.HTTPClient, p.provider, target, stsConfig)
	if err != nil {
		return nil, err
	}
	return p.provider.GetDerivedKubernetes(ctx, target)
}

// getOrBuildStsConfig returns a cached STS config, rebuilding it when the
// OIDC provider's token URL changes (e.g., after SIGHUP).
func (p *tokenExchangingProvider) getOrBuildStsConfig(snap *oauth.Snapshot) *tokenexchange.TargetTokenExchangeConfig {
	strategy := p.baseConfig.GetStsStrategy()
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
		klog.Warningf("token exchange strategy %q configured but OIDC provider returned empty token URL", strategy)
		return nil
	}

	p.stsConfigMu.Lock()
	defer p.stsConfigMu.Unlock()

	// Return cached config if token URL hasn't changed
	if p.stsConfig != nil && p.stsTokenURL == tokenURL {
		return p.stsConfig
	}

	authStyle := p.baseConfig.GetStsAuthStyle()
	if authStyle == "" {
		authStyle = tokenexchange.AuthStyleParams
	}

	cfg := &tokenexchange.TargetTokenExchangeConfig{
		TokenURL:       tokenURL,
		ClientID:       p.baseConfig.GetStsClientId(),
		ClientSecret:   p.baseConfig.GetStsClientSecret(),
		Audience:       p.baseConfig.GetStsAudience(),
		Scopes:         p.baseConfig.GetStsScopes(),
		AuthStyle:      authStyle,
		ClientCertFile: p.baseConfig.GetStsClientCertFile(),
		ClientKeyFile:  p.baseConfig.GetStsClientKeyFile(),
		CAFile:         p.baseConfig.GetCertificateAuthority(),
	}
	if err := cfg.Validate(); err != nil {
		klog.Warningf("STS config validation failed, token exchange will be attempted per-request but will likely fail with the same error: %v", err)
		return nil
	}

	p.stsConfig = cfg
	p.stsTokenURL = tokenURL
	return p.stsConfig
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

func (p *tokenExchangingProvider) WatchTargets(reload McpReload) {
	p.provider.WatchTargets(reload)
}

func (p *tokenExchangingProvider) Close() {
	p.provider.Close()
}
