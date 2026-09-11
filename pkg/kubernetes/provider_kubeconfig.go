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
	mu  sync.RWMutex
	cfg *config.Config
	*ProviderGVKFilter
	defaultContext      string
	managers            map[string]*Manager
	kubeconfigWatcher   *watcher.Kubeconfig
	clusterStateWatcher *watcher.ClusterState

	targetConfigsMu sync.Mutex
	// targetConfigs caches per-target token exchange configs, keyed by context
	// name. Entries are rebuilt when the cache key (derived from live config)
	// changes; cleared wholesale on reset().
	targetConfigs map[string]*cachedTargetConfig
}

type cachedTargetConfig struct {
	key    tokenExchangeConfigCacheKey
	config *tokenexchange.TargetTokenExchangeConfig
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
	// Per-target exchange inherits strategy/credentials/endpoint from the global
	// [token_exchange] block. Without it the exchange silently no-ops at runtime.
	if pc := ret.providerConfig(cfg); pc != nil && len(pc.Targets) > 0 && cfg.GetTokenExchangeConfig() == nil {
		names := make([]string, 0, len(pc.Targets))
		for name := range pc.Targets {
			names = append(names, name)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("cluster_provider_configs.kubeconfig configures per-target token exchange for targets [%s] but no global [token_exchange] block is set", strings.Join(names, ", "))
	}
	ret.ProviderGVKFilter = NewProviderGVKFilter(ret)
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
		if cached.config != nil {
			cached.config.CloseIdleConnections()
		}
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

func (p *kubeConfigClusterProvider) IsTargetCompatibilityToolFiltersEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg.EnableTargetCompatibilityToolFilters.Get()
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

func (p *kubeConfigClusterProvider) GetTargetManagers(ctx context.Context) ([]*Manager, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	contextNames, err := p.getTargetsUnsync()
	if err != nil {
		return nil, err
	}
	managers := make([]*Manager, 0, len(contextNames))
	for _, cn := range contextNames {
		mgr, err := p.managerForContext(ctx, cn, true)
		if err != nil {
			return nil, err
		}
		managers = append(managers, mgr)
	}

	return managers, nil
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
		if cached.config != nil {
			cached.config.CloseIdleConnections()
		}
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

func (p *kubeConfigClusterProvider) GetTokenExchangeStrategy(base *config.Config) string {
	if cfg := p.providerConfig(base); cfg != nil {
		return cfg.Strategy
	}
	return ""
}

func (p *kubeConfigClusterProvider) GetTargetTokenExchangeConfig(target string, base *config.Config, tokenURL string) (*tokenexchange.TargetTokenExchangeConfig, error) {
	cfg := p.providerConfig(base)
	if cfg == nil {
		return nil, nil
	}
	targetCfg, ok := cfg.Targets[target]
	if !ok {
		return nil, nil
	}

	global := base.GetTokenExchangeConfig()
	if global == nil {
		// Target is configured but global [token_exchange] block is absent.
		// This can happen via hot-reload: return an error rather than silently
		// falling through to passthrough mode with the wrong-audience token.
		return nil, fmt.Errorf("target %q has per-target token exchange configured but no global [token_exchange] block is set", target)
	}

	key := computeTargetConfigCacheKey(cfg.Strategy, base, global, targetCfg, tokenURL)

	p.targetConfigsMu.Lock()
	defer p.targetConfigsMu.Unlock()

	if cached, ok := p.targetConfigs[target]; ok {
		if cached.key == key {
			return cached.config, nil
		}
		// Config changed (e.g. SIGHUP rotated credentials or OIDC endpoint rotated)
		// — close idle connections so they aren't reused with stale credentials.
		if cached.config != nil {
			cached.config.CloseIdleConnections()
		}
	}

	result := &tokenexchange.TargetTokenExchangeConfig{
		TokenURL:           tokenURL,
		Audience:           global.Audience.Get(),
		SubjectTokenType:   global.SubjectTokenType.Get(),
		RequestedTokenType: global.RequestedTokenType.Get(),
		Scopes:             append([]string(nil), global.Scopes.Get()...),
		CAFile:             base.CertificateAuthority.Get(),
		TLSMinVersion:      base.TLSMinVersion.Get(),
		TLSCipherSuites:    append([]string(nil), base.TLSCipherSuites.Get()...),
	}
	applyClientAuth(result, global.GetClientAuth())

	result.Audience = targetCfg.Audience
	result.ClientID, result.ClientSecret = resolveTargetClientCreds(
		result.AuthStyle, result.ClientID, result.ClientSecret, targetCfg.ClientId, targetCfg.ClientSecret)
	if len(targetCfg.Scopes) > 0 {
		result.Scopes = append([]string(nil), targetCfg.Scopes...)
	}
	if targetCfg.SubjectIssuer != "" {
		result.SubjectIssuer = targetCfg.SubjectIssuer
	}

	if p.targetConfigs == nil {
		p.targetConfigs = make(map[string]*cachedTargetConfig)
	}
	p.targetConfigs[target] = &cachedTargetConfig{key: key, config: result}
	return result, nil
}

// resolveTargetClientCreds overlays per-target client credentials on the global
// ones. Styles that authenticate with a signed assertion or a federated token
// file (assertion, federated) do not use a client_secret, so a per-target
// client_id overrides on its own. Secret-based styles require both client_id and
// client_secret to be set together, to avoid pairing a per-target id with a
// mismatched global secret.
func resolveTargetClientCreds(authStyle, globalID, globalSecret, targetID, targetSecret string) (id, secret string) {
	id, secret = globalID, globalSecret
	switch authStyle {
	case tokenexchange.AuthStyleAssertion, tokenexchange.AuthStyleFederated:
		if targetID != "" {
			id = targetID
		}
	default:
		if targetID != "" && targetSecret != "" {
			id, secret = targetID, targetSecret
		}
	}
	return id, secret
}

func computeTargetConfigCacheKey(
	strategy string,
	base *config.Config,
	global *config.TokenExchangeConfig,
	targetCfg KubeConfigTargetConfig,
	tokenURL string,
) tokenExchangeConfigCacheKey {
	key := tokenExchangeConfigCacheKey{
		TokenURL:           tokenURL,
		Strategy:           strategy,
		Audience:           targetCfg.Audience,
		SubjectTokenType:   global.SubjectTokenType.Get(),
		RequestedTokenType: global.RequestedTokenType.Get(),
		CAFile:             base.CertificateAuthority.Get(),
		TLSMinVersion:      base.TLSMinVersion.Get(),
		TLSCipherSuites:    strings.Join(base.TLSCipherSuites.Get(), "\x00"),
		SubjectIssuer:      targetCfg.SubjectIssuer,
	}

	scopes := global.Scopes.Get()
	if len(targetCfg.Scopes) > 0 {
		scopes = targetCfg.Scopes
	}
	key.Scopes = strings.Join(scopes, "\x00")

	authStyle := ""
	if auth := global.GetClientAuth(); auth != nil {
		key.ClientID = auth.ClientID.Get()
		key.ClientSecret = auth.ClientSecret.Get()
		key.AuthStyle = auth.Method.Get()
		key.ClientCertFile = auth.CertificateFile.Get()
		key.ClientKeyFile = auth.PrivateKeyFile.Get()
		key.FederatedTokenFile = auth.TokenFile.Get()

		switch config.TokenExchangeClientAuthMethod(auth.Method.Get()) {
		case config.TokenExchangeClientAuthMethodPrivateKey:
			authStyle = tokenexchange.AuthStyleAssertion
		case config.TokenExchangeClientAuthMethodJWTFile:
			authStyle = tokenexchange.AuthStyleFederated
		}
	}
	// Apply the same per-target credential override as the built config, so the
	// key changes exactly when the resulting ClientID/ClientSecret would.
	key.ClientID, key.ClientSecret = resolveTargetClientCreds(
		authStyle, key.ClientID, key.ClientSecret, targetCfg.ClientId, targetCfg.ClientSecret)

	return key
}
