package kubernetes

import (
	"context"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

type tokenExchangingProvider struct {
	provider           Provider
	baseConfigProvider func() api.BaseConfig
	oauthState         *oauth.State
	tokenExchangeCache tokenExchangeConfigCache
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
	tokenExchangeConfig := p.getOrBuildTokenExchangeConfig(ctx, snap, baseConfig)
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

func (p *tokenExchangingProvider) getOrBuildTokenExchangeConfig(ctx context.Context, snap *oauth.Snapshot, baseConfig api.BaseConfig) *tokenexchange.TargetTokenExchangeConfig {
	global := baseConfig.GetTokenExchangeConfig()
	if global == nil {
		p.tokenExchangeCache.clear()
		return nil
	}

	var tokenURL string
	if snap.OIDCProvider != nil {
		if endpoint := snap.OIDCProvider.Endpoint(); endpoint.TokenURL != "" {
			tokenURL = endpoint.TokenURL
		}
	}
	if tokenURL == "" {
		p.tokenExchangeCache.clear()
		klogutil.LogWarn(klogutil.FromContext(ctx), "OIDC provider returned no token endpoint; token exchange is unavailable",
			klogutil.Field("strategy", global.GetStrategy()))
		return nil
	}

	key := newTokenExchangeConfigCacheKey(tokenURL, baseConfig)
	return p.tokenExchangeCache.getOrReplace(key, func() *tokenexchange.TargetTokenExchangeConfig {
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
		return cfg
	})
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
	default:
		// Preserve invalid methods so TargetTokenExchangeConfig.Validate rejects
		// them instead of treating an empty style as form-body authentication.
		cfg.AuthStyle = string(auth.GetMethod())
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
	p.tokenExchangeCache.clear()
	p.provider.Close()
}

func (p *tokenExchangingProvider) AnyTargetHasGVKs(ctx context.Context, gvks []schema.GroupVersionKind) bool {
	return p.provider.AnyTargetHasGVKs(ctx, gvks)
}

func (p *tokenExchangingProvider) IsTargetCompatibilityToolFiltersEnabled() bool {
	return p.provider.IsTargetCompatibilityToolFiltersEnabled()
}

// tokenExchangeConfigCache owns synchronization and lifecycle management for
// the memoized config and its HTTP client's idle connections.
type tokenExchangeConfigCache struct {
	mu     sync.Mutex
	config *tokenexchange.TargetTokenExchangeConfig
	key    tokenExchangeConfigCacheKey
}

func (c *tokenExchangeConfigCache) getOrReplace(key tokenExchangeConfigCacheKey, build func() *tokenexchange.TargetTokenExchangeConfig) *tokenexchange.TargetTokenExchangeConfig {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config != nil && c.key == key {
		return c.config
	}
	next := build()
	if c.config != nil {
		c.config.CloseIdleConnections()
	}
	c.config = next
	c.key = key
	return c.config
}

func (c *tokenExchangeConfigCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config != nil {
		c.config.CloseIdleConnections()
	}
	c.config = nil
	c.key = tokenExchangeConfigCacheKey{}
}
