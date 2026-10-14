package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

// KubeConfigTargetParameterName is the parameter name used to specify
// the kubeconfig context when using the kubeconfig cluster provider strategy.
const KubeConfigTargetParameterName = "context"

// kubeConfigClusterProvider implements Provider for managing multiple
// Kubernetes clusters using different contexts from a kubeconfig file.
// It lazily initializes managers for each context as they are requested.
type kubeConfigClusterProvider struct {
	mu                  sync.RWMutex
	cfg                 *config.Config
	defaultContext      string
	managers            map[string]*Manager
	kubeconfigWatcher   *watcher.Kubeconfig
	clusterStateWatcher *watcher.ClusterState

	targetConfigsMu sync.Mutex
	// targetConfigs caches resolved per-target token exchanges, keyed by
	// context name, so the memoized HTTP client in each config is reused.
	// Entries are rebuilt when the resolved values change; cleared wholesale
	// on reset() and Close().
	targetConfigs map[string]*cachedTargetConfig
}

type cachedTargetConfig struct {
	key      tokenExchangeConfigCacheKey
	exchange *TargetTokenExchange
}

var (
	_ Provider              = &kubeConfigClusterProvider{}
	_ TokenExchangeProvider = &kubeConfigClusterProvider{}
)

func init() {
	RegisterProvider(config.ClusterProviderKubeConfig, newKubeConfigClusterProvider)
}

// newKubeConfigClusterProvider creates a provider that manages multiple clusters
// via kubeconfig contexts.
// Internally, it leverages a KubeconfigManager for each context, initializing them
// lazily when requested.
func newKubeConfigClusterProvider(ctx context.Context, cfg *config.Config) (Provider, error) {
	ret := &kubeConfigClusterProvider{cfg: cfg}
	if err := ret.reset(ctx); err != nil {
		return nil, err
	}
	if err := validateTargetTokenExchanges(cfg, ret.providerConfig(cfg)); err != nil {
		ret.Close()
		return nil, err
	}
	return ret, nil
}

func (p *kubeConfigClusterProvider) reset(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.resetLocked(ctx)
}

func (p *kubeConfigClusterProvider) resetLocked(ctx context.Context) error {
	m, err := NewKubeconfigManager(ctx, p.cfg, "")
	if err != nil {
		if errors.Is(err, ErrorKubeconfigInClusterNotAllowed) {
			return fmt.Errorf( //nolint:ST1005 // user-facing error with actionable multi-line guidance
				"kubeconfig ClusterProviderStrategy is invalid for in-cluster deployments: %w\n\n"+
					"If you intend to connect to a different cluster from within a pod, set in your TOML config:\n"+
					"  kubeconfig = \"/path/to/kubeconfig\"\n\n"+
					"An explicit kubeconfig path overrides in-cluster detection; cluster_provider_strategy is optional.\n"+
					"See https://github.com/containers/kubernetes-mcp-server/blob/main/docs/configuration.md#cross-cluster-access-from-a-pod",
				err,
			)
		}
		return err
	}

	rawConfig, err := m.kubernetes.clientCmdConfig.RawConfig()
	if err != nil {
		m.Close()
		return err
	}

	// Determine the effective default context.
	// RawConfig() returns the file's current-context which may be empty when
	// NewKubeconfigManager auto-selected the only available context.
	defaultContext := rawConfig.CurrentContext
	if defaultContext == "" && len(rawConfig.Contexts) == 1 {
		for name := range rawConfig.Contexts {
			defaultContext = name
		}
	}

	p.targetConfigsMu.Lock()
	for _, cached := range p.targetConfigs {
		cached.exchange.Config.CloseIdleConnections()
	}
	p.targetConfigs = nil
	p.targetConfigsMu.Unlock()

	for _, old := range p.managers {
		if old != nil {
			old.Close()
		}
	}
	p.managers = map[string]*Manager{
		defaultContext: m,
	}

	for name := range rawConfig.Contexts {
		if name == defaultContext {
			continue
		}
		p.managers[name] = nil
	}

	p.Close()
	p.kubeconfigWatcher = watcher.NewKubeconfig(ctx, m.kubernetes.clientCmdConfig, p.cfg.KubeconfigDebounceWindow.Get())
	p.clusterStateWatcher = watcher.NewClusterState(ctx, m.kubernetes.DiscoveryClient(), p.cfg.ClusterStatePollInterval.Get(), p.cfg.ClusterStateDebounceWindow.Get())
	p.defaultContext = defaultContext

	return nil
}

// managerForWorkspace returns or creates a Manager for the specified kubeContext.
// callerLock indicates whether the caller (true) or this func (false) is responsible for synchronization.
func (p *kubeConfigClusterProvider) managerForContext(ctx context.Context, kubeContext string, callerLock bool) (*Manager, error) {
	if !callerLock {
		p.mu.RLock()
	}
	m, ok := p.managers[kubeContext]
	if !callerLock {
		p.mu.RUnlock()
	}
	if ok && m != nil {
		return m, nil
	}

	if !callerLock {
		p.mu.Lock()
		defer p.mu.Unlock()
		// Recheck in case it was introduced since RUnlock
		m, ok = p.managers[kubeContext]
		if ok && m != nil {
			return m, nil
		}
	}

	m, err := NewKubeconfigManager(ctx, p.cfg, kubeContext)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnknownTarget, err)
	}

	p.managers[kubeContext] = m

	return m, nil
}

func (p *kubeConfigClusterProvider) IsMultiTarget() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.managers) > 1
}

func (p *kubeConfigClusterProvider) getTargetsUnsync() ([]string, error) {
	contextNames := make([]string, 0, len(p.managers))
	for contextName := range p.managers {
		contextNames = append(contextNames, contextName)
	}

	return contextNames, nil
}

func (p *kubeConfigClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.getTargetsUnsync()
}

func (p *kubeConfigClusterProvider) GetTargetParameterName() string {
	return KubeConfigTargetParameterName
}

func (p *kubeConfigClusterProvider) GetDerivedKubernetes(ctx context.Context, kubeContext string) (*Kubernetes, error) {
	p.mu.RLock()
	m, ok := p.managers[kubeContext]
	if ok && m != nil {
		k8s, err := m.Derived(ctx)
		p.mu.RUnlock()
		return k8s, err
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	m, err := p.managerForContext(ctx, kubeContext, true)
	if err != nil {
		return nil, err
	}
	return m.Derived(ctx)
}

func (p *kubeConfigClusterProvider) GetDefaultTarget() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.defaultContext
}

func (p *kubeConfigClusterProvider) ReloadConfig(_ context.Context, cfg *config.Config) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg
	return nil
}

func (p *kubeConfigClusterProvider) PublishKubernetesConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, m := range p.managers {
		if m != nil {
			m.SetConfig(cfg)
		}
	}
}

func (p *kubeConfigClusterProvider) WatchTargets(ctx context.Context, reload McpReloader) {
	reloadWithReset := func() error {
		return reload.Run(func() error {
			if err := p.reset(ctx); err != nil {
				return err
			}
			p.WatchTargets(ctx, reload)
			return reload.ApplyToolsets()
		})
	}
	p.kubeconfigWatcher.Watch(ctx, reloadWithReset)
	p.clusterStateWatcher.Watch(ctx, reload.ClusterStateCallback())
}

func (p *kubeConfigClusterProvider) Close() {
	for _, w := range []watcher.Watcher{p.kubeconfigWatcher, p.clusterStateWatcher} {
		if !reflect.ValueOf(w).IsNil() {
			w.Close()
		}
	}

	p.targetConfigsMu.Lock()
	for _, cached := range p.targetConfigs {
		cached.exchange.Config.CloseIdleConnections()
	}
	p.targetConfigs = nil
	p.targetConfigsMu.Unlock()
}

func (p *kubeConfigClusterProvider) providerConfig(base *config.Config) *KubeConfigProviderConfig {
	extCfg, ok := base.GetProviderConfig(config.ClusterProviderKubeConfig)
	if !ok {
		return nil
	}
	kcCfg, ok := extCfg.(*KubeConfigProviderConfig)
	if !ok {
		return nil
	}
	return kcCfg
}

func (p *kubeConfigClusterProvider) GetTargetTokenExchange(target string, base *config.Config, discoveredTokenURL string) (*TargetTokenExchange, error) {
	pc := p.providerConfig(base)
	if pc == nil {
		return nil, nil
	}
	targetCfg, ok := pc.Targets[target]
	if !ok {
		return nil, nil
	}

	resolved, err := resolveTargetTokenExchange(base, target, targetCfg, discoveredTokenURL)
	if err != nil {
		return nil, err
	}
	key := targetTokenExchangeCacheKey(resolved)

	p.targetConfigsMu.Lock()
	defer p.targetConfigsMu.Unlock()

	if cached, ok := p.targetConfigs[target]; ok {
		if cached.key == key {
			return cached.exchange, nil
		}
		// Resolved values changed (e.g. SIGHUP rotated credentials or the OIDC
		// endpoint rotated): drop idle connections so they are not reused with
		// stale settings.
		cached.exchange.Config.CloseIdleConnections()
	}
	if p.targetConfigs == nil {
		p.targetConfigs = make(map[string]*cachedTargetConfig)
	}
	p.targetConfigs[target] = &cachedTargetConfig{key: key, exchange: resolved}
	return resolved, nil
}

// resolveTargetTokenExchange overlays the per-target configuration on the
// global [token_exchange] block. Precedence per field is target, then global,
// and for the token URL finally the endpoint discovered from authorization_url.
// Client authentication is taken as a whole from whichever level sets it, so a
// per-target client_id is never paired with a global client_secret.
func resolveTargetTokenExchange(base *config.Config, target string, t KubeConfigTargetConfig, discoveredTokenURL string) (*TargetTokenExchange, error) {
	global := base.GetTokenExchangeConfig()
	pick := func(targetValue string, globalValue func() string) string {
		if targetValue != "" || global == nil {
			return targetValue
		}
		return globalValue()
	}

	strategy := pick(t.Strategy, func() string { return global.Strategy.Get() })
	if strategy == "" {
		return nil, fmt.Errorf("target %q: no token exchange strategy configured: set cluster_provider_configs.kubeconfig.targets.%q.strategy or token_exchange.strategy", target, target)
	}
	tokenURL := pick(t.TokenURL, func() string { return global.TokenURL.Get() })
	if tokenURL == "" {
		tokenURL = discoveredTokenURL
	}

	caFile := t.CertificateAuthority
	if caFile == "" {
		caFile = base.CertificateAuthority.Get()
	}

	cfg := &tokenexchange.TargetTokenExchangeConfig{
		TokenURL:           tokenURL,
		Audience:           pick(t.Audience, func() string { return global.Audience.Get() }),
		SubjectTokenType:   pick(t.SubjectTokenType, func() string { return global.SubjectTokenType.Get() }),
		RequestedTokenType: pick(t.RequestedTokenType, func() string { return global.RequestedTokenType.Get() }),
		SubjectIssuer:      t.SubjectIssuer,
		CAFile:             caFile,
		TLSMinVersion:      base.TLSMinVersion.Get(),
		TLSCipherSuites:    append([]string(nil), base.TLSCipherSuites.Get()...),
	}
	switch {
	case len(t.Scopes) > 0:
		cfg.Scopes = append([]string(nil), t.Scopes...)
	case global != nil:
		cfg.Scopes = append([]string(nil), global.Scopes.Get()...)
	}
	switch {
	case t.ClientAuth.isSet():
		a := t.ClientAuth
		setClientAuth(cfg, a.Method, a.ClientID, a.ClientSecret, a.CertificateFile, a.PrivateKeyFile, a.TokenFile)
	case global != nil:
		applyClientAuth(cfg, global.GetClientAuth())
	}
	return &TargetTokenExchange{Strategy: strategy, Config: cfg}, nil
}

// validateTargetTokenExchanges fails provider construction when a configured
// target cannot resolve to a usable exchange. The token URL may still come from
// OIDC discovery at runtime, so it is only required when authorization_url is
// unset.
func validateTargetTokenExchanges(base *config.Config, pc *KubeConfigProviderConfig) error {
	if pc == nil {
		return nil
	}
	names := make([]string, 0, len(pc.Targets))
	for name := range pc.Targets {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		resolved, err := resolveTargetTokenExchange(base, name, pc.Targets[name], "")
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if resolved.Config.TokenURL == "" && base.AuthorizationURL.Get() == "" {
			errs = append(errs, fmt.Errorf("target %q: no token URL configured: set cluster_provider_configs.kubeconfig.targets.%q.token_url, token_exchange.token_url, or authorization_url to discover the token endpoint", name, name))
		}
		if err := resolved.Config.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("target %q: %w", name, err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("cluster_provider_configs.kubeconfig: %w", errors.Join(errs...))
}

// targetTokenExchangeCacheKey derives the cache key from the resolved values,
// so the key changes exactly when the exchange that would be performed does.
func targetTokenExchangeCacheKey(te *TargetTokenExchange) tokenExchangeConfigCacheKey {
	c := te.Config
	return tokenExchangeConfigCacheKey{
		TokenURL:           c.TokenURL,
		Strategy:           te.Strategy,
		ClientID:           c.ClientID,
		ClientSecret:       c.ClientSecret,
		Audience:           c.Audience,
		SubjectTokenType:   c.SubjectTokenType,
		RequestedTokenType: c.RequestedTokenType,
		Scopes:             strings.Join(c.Scopes, "\x00"),
		AuthStyle:          c.AuthStyle,
		ClientCertFile:     c.ClientCertFile,
		ClientKeyFile:      c.ClientKeyFile,
		FederatedTokenFile: c.FederatedTokenFile,
		CAFile:             c.CAFile,
		TLSMinVersion:      c.TLSMinVersion,
		TLSCipherSuites:    strings.Join(c.TLSCipherSuites, "\x00"),
		SubjectIssuer:      c.SubjectIssuer,
	}
}
