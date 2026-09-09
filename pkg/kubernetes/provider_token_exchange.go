package kubernetes

import (
	"context"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

type tokenExchangingProvider struct {
	provider           Provider
	configProvider     func() *config.Config
	oauthState         *oauth.State
	tokenExchangeCache tokenExchangeConfigCache
}

var _ Provider = &tokenExchangingProvider{}

func newTokenExchangingProvider(
	provider Provider,
	configProvider func() *config.Config,
	oauthState *oauth.State,
) Provider {
	return &tokenExchangingProvider{
		provider:       provider,
		configProvider: configProvider,
		oauthState:     oauthState,
	}
}

func (p *tokenExchangingProvider) GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error) {
	snap := p.oauthState.Load()
	if snap == nil {
		return p.provider.GetDerivedKubernetes(ctx, target)
	}
	cfg := p.config()
	if cfg == nil {
		// Defensive only: production wiring always supplies a non-nil config
		// (NewProvider defaults the provider to return cfg, and the cmd path
		// passes cfgState.Load(), which is non-nil by the ConfigState
		// invariant). If a caller ever omits it, fall back to the wrapped
		// provider rather than panicking; token exchange is simply skipped.
		return p.provider.GetDerivedKubernetes(ctx, target)
	}
	tokenExchangeConfig := p.getOrBuildTokenExchangeConfig(ctx, snap, cfg)
	ctx, err := ExchangeTokenInContext(ctx, cfg, p.provider, target, tokenExchangeConfig)
	if err != nil {
		return nil, err
	}
	return p.provider.GetDerivedKubernetes(ctx, target)
}

func (p *tokenExchangingProvider) config() *config.Config {
	if p.configProvider == nil {
		return nil
	}
	return p.configProvider()
}

func (p *tokenExchangingProvider) getOrBuildTokenExchangeConfig(ctx context.Context, snap *oauth.Snapshot, cfg *config.Config) *tokenexchange.TargetTokenExchangeConfig {
	global := cfg.GetTokenExchangeConfig()
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
			klogutil.Field("strategy", global.Strategy.Get()))
		return nil
	}

	key := newTokenExchangeConfigCacheKey(tokenURL, cfg)
	return p.tokenExchangeCache.getOrReplace(key, func() *tokenexchange.TargetTokenExchangeConfig {
		te := &tokenexchange.TargetTokenExchangeConfig{
			TokenURL:           tokenURL,
			Audience:           global.Audience.Get(),
			SubjectTokenType:   global.SubjectTokenType.Get(),
			RequestedTokenType: global.RequestedTokenType.Get(),
			Scopes:             append([]string(nil), global.Scopes.Get()...),
			CAFile:             cfg.CertificateAuthority.Get(),
			TLSMinVersion:      cfg.TLSMinVersion.Get(),
			TLSCipherSuites:    append([]string(nil), cfg.TLSCipherSuites.Get()...),
		}
		applyClientAuth(te, global.GetClientAuth())
		te.SetRequireTLS(func() bool { return cfg.RequireTLS.Get() })
		return te
	})
}

func applyClientAuth(cfg *tokenexchange.TargetTokenExchangeConfig, auth *config.TokenExchangeClientAuth) {
	if auth == nil {
		return
	}
	cfg.ClientID = auth.ClientID.Get()
	cfg.ClientSecret = auth.ClientSecret.Get()
	switch api.TokenExchangeClientAuthMethod(auth.Method.Get()) {
	case api.TokenExchangeClientAuthMethodSecretBasic:
		cfg.AuthStyle = tokenexchange.AuthStyleHeader
	case api.TokenExchangeClientAuthMethodSecretPost:
		cfg.AuthStyle = tokenexchange.AuthStyleParams
	case api.TokenExchangeClientAuthMethodPrivateKey:
		cfg.AuthStyle = tokenexchange.AuthStyleAssertion
		cfg.ClientCertFile = auth.CertificateFile.Get()
		cfg.ClientKeyFile = auth.PrivateKeyFile.Get()
	case api.TokenExchangeClientAuthMethodJWTFile:
		cfg.AuthStyle = tokenexchange.AuthStyleFederated
		cfg.FederatedTokenFile = auth.TokenFile.Get()
	default:
		// Preserve invalid methods so TargetTokenExchangeConfig.Validate rejects
		// them instead of treating an empty style as form-body authentication.
		cfg.AuthStyle = auth.Method.Get()
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

func newTokenExchangeConfigCacheKey(tokenURL string, cfg *config.Config) tokenExchangeConfigCacheKey {
	global := cfg.GetTokenExchangeConfig()
	key := tokenExchangeConfigCacheKey{
		TokenURL:        tokenURL,
		CAFile:          cfg.CertificateAuthority.Get(),
		TLSMinVersion:   cfg.TLSMinVersion.Get(),
		TLSCipherSuites: strings.Join(cfg.TLSCipherSuites.Get(), "\x00"),
		RequireTLS:      cfg.RequireTLS.Get(),
	}
	if global == nil {
		return key
	}
	key.Strategy = global.Strategy.Get()
	key.Audience = global.Audience.Get()
	key.SubjectTokenType = global.SubjectTokenType.Get()
	key.RequestedTokenType = global.RequestedTokenType.Get()
	key.Scopes = strings.Join(global.Scopes.Get(), "\x00")
	if auth := global.GetClientAuth(); auth != nil {
		key.ClientID = auth.ClientID.Get()
		key.ClientSecret = auth.ClientSecret.Get()
		key.AuthStyle = auth.Method.Get()
		key.ClientCertFile = auth.CertificateFile.Get()
		key.ClientKeyFile = auth.PrivateKeyFile.Get()
		key.FederatedTokenFile = auth.TokenFile.Get()
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
