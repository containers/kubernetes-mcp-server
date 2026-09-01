package kubernetes

import (
	"context"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

type tokenExchangingProvider struct {
	provider           Provider
	baseConfigProvider func() api.BaseConfig
	oauthState         *oauth.State
	// tokenExchangeConfig is cached and reused across calls so the memoized HTTP client in
	// TargetTokenExchangeConfig (and its keep-alive connections to the IdP) is
	// reused. Rebuilt when the token URL or any token_exchange/TLS field changes after
	// a reload. Client assertions themselves are never cached — a fresh, single-use
	// jti is minted per exchange (see TargetTokenExchangeConfig.BuildAssertion).
	tokenExchangeConfig    *tokenexchange.TargetTokenExchangeConfig
	tokenExchangeConfigMu  sync.Mutex
	tokenExchangeConfigKey tokenExchangeConfigCacheKey
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
		// Defensive only: production wiring always supplies a non-nil config
		// (NewProvider defaults the provider to return cfg, and the cmd path
		// passes cfgState.Load(), which is non-nil by the StaticConfigState
		// invariant). If a caller ever omits it, fall back to the wrapped
		// provider rather than panicking; token exchange is simply skipped.
		return p.provider.GetDerivedKubernetes(ctx, target)
	}
	tokenExchangeConfig := p.getOrBuildTokenExchangeConfig(snap, baseConfig)
	ctx, err := ExchangeTokenInContext(ctx, baseConfig, p.provider, target, tokenExchangeConfig)
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

func (p *tokenExchangingProvider) getOrBuildTokenExchangeConfig(snap *oauth.Snapshot, baseConfig api.BaseConfig) *tokenexchange.TargetTokenExchangeConfig {
	global := baseConfig.GetTokenExchangeConfig()
	if global == nil {
		return nil
	}

	var tokenURL string
	if snap.OIDCProvider != nil {
		if endpoint := snap.OIDCProvider.Endpoint(); endpoint.TokenURL != "" {
			tokenURL = endpoint.TokenURL
		}
	}
	if tokenURL == "" {
		return nil
	}

	p.tokenExchangeConfigMu.Lock()
	defer p.tokenExchangeConfigMu.Unlock()

	key := newTokenExchangeConfigCacheKey(tokenURL, baseConfig)
	if p.tokenExchangeConfig != nil && p.tokenExchangeConfigKey == key {
		return p.tokenExchangeConfig
	}

	cfg := &tokenexchange.TargetTokenExchangeConfig{
		TokenURL:           tokenURL,
		Audience:           global.GetAudience(),
		SubjectTokenType:   global.GetSubjectTokenType(),
		RequestedTokenType: global.GetRequestedTokenType(),
		Scopes:             append([]string(nil), global.GetScopes()...),
		CAFile:             baseConfig.GetCertificateAuthority(),
		TLSMinVersion:      baseConfig.GetTLSMinVersionConfig(),
		TLSCipherSuites:    append([]string(nil), baseConfig.GetTLSCipherSuitesConfig()...),
	}
	applyClientAuth(cfg, global.GetClientAuth())
	cfg.SetRequireTLS(baseConfig.IsRequireTLS)

	// Release the previous client's idle connections before swapping in the
	// rebuilt config so they don't linger until garbage collection.
	if p.tokenExchangeConfig != nil {
		p.tokenExchangeConfig.CloseIdleConnections()
	}
	p.tokenExchangeConfig = cfg
	p.tokenExchangeConfigKey = key
	return p.tokenExchangeConfig
}

func applyClientAuth(cfg *tokenexchange.TargetTokenExchangeConfig, auth api.TokenExchangeClientAuth) {
	if auth == nil {
		return
	}
	cfg.ClientID = auth.GetClientID()
	cfg.ClientSecret = auth.GetClientSecret()
	switch auth.GetMethod() {
	case api.TokenExchangeClientAuthMethodSecretBasic:
		cfg.AuthStyle = tokenexchange.AuthStyleHeader
	case api.TokenExchangeClientAuthMethodSecretPost:
		cfg.AuthStyle = tokenexchange.AuthStyleParams
	case api.TokenExchangeClientAuthMethodPrivateKey:
		cfg.AuthStyle = tokenexchange.AuthStyleAssertion
		cfg.ClientCertFile = auth.GetCertificateFile()
		cfg.ClientKeyFile = auth.GetPrivateKeyFile()
	case api.TokenExchangeClientAuthMethodJWTFile:
		cfg.AuthStyle = tokenexchange.AuthStyleFederated
		cfg.FederatedTokenFile = auth.GetTokenFile()
	}
}

type tokenExchangeConfigCacheKey struct {
	TokenURL           string
	Strategy           string
	ClientID           string
	ClientSecret       string
	Audience           string
	SubjectTokenType   string
	RequestedTokenType string
	Scopes             string
	AuthStyle          string
	ClientCertFile     string
	ClientKeyFile      string
	FederatedTokenFile string
	CAFile             string
	TLSMinVersion      string
	TLSCipherSuites    string
	RequireTLS         bool
}

func newTokenExchangeConfigCacheKey(tokenURL string, cfg api.BaseConfig) tokenExchangeConfigCacheKey {
	global := cfg.GetTokenExchangeConfig()
	key := tokenExchangeConfigCacheKey{
		TokenURL:        tokenURL,
		CAFile:          cfg.GetCertificateAuthority(),
		TLSMinVersion:   cfg.GetTLSMinVersionConfig(),
		TLSCipherSuites: strings.Join(cfg.GetTLSCipherSuitesConfig(), "\x00"),
		RequireTLS:      cfg.IsRequireTLS(),
	}
	if global == nil {
		return key
	}
	key.Strategy = global.GetStrategy()
	key.Audience = global.GetAudience()
	key.SubjectTokenType = global.GetSubjectTokenType()
	key.RequestedTokenType = global.GetRequestedTokenType()
	key.Scopes = strings.Join(global.GetScopes(), "\x00")
	if auth := global.GetClientAuth(); auth != nil {
		key.ClientID = auth.GetClientID()
		key.ClientSecret = auth.GetClientSecret()
		key.AuthStyle = string(auth.GetMethod())
		key.ClientCertFile = auth.GetCertificateFile()
		key.ClientKeyFile = auth.GetPrivateKeyFile()
		key.FederatedTokenFile = auth.GetTokenFile()
	}
	return key
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

func (p *tokenExchangingProvider) AnyTargetHasGVKs(ctx context.Context, gvks []schema.GroupVersionKind) bool {
	return p.provider.AnyTargetHasGVKs(ctx, gvks)
}

func (p *tokenExchangingProvider) IsTargetCompatibilityToolFiltersEnabled() bool {
	return p.provider.IsTargetCompatibilityToolFiltersEnabled()
}
