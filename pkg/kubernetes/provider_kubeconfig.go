package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	mu             sync.RWMutex
	cfg            *config.Config
	defaultContext string
	managers       map[string]*Manager
	WatcherGroup

	targetConfigsMu sync.Mutex
	// targetConfigs caches resolved per-target token exchanges, keyed by
	// context name, so the memoized HTTP client in each config is reused.
	// Entries are rebuilt when the resolved values change; cleared wholesale
	// on reset() and Close().
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
	ret.SetReset(ret.reset)
	if err := ret.reset(ctx); err != nil {
		return nil, err
	}
	contexts, err := ret.GetTargets(ctx)
	if err != nil {
		ret.Close()
		return nil, err
	}
	if err := validateTargetTokenExchanges(cfg, ret.providerConfig(cfg), contexts); err != nil {
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
	if err := p.ErrClosed(); err != nil {
		return err
	}

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
		cached.config.CloseIdleConnections()
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

	p.defaultContext = defaultContext
	return p.Replace(
		watcher.NewKubeconfig(ctx, m.kubernetes.clientCmdConfig, p.cfg.KubeconfigDebounceWindow.Get()),
		watcher.NewClusterState(ctx, m.kubernetes.DiscoveryClient(), p.cfg.ClusterStatePollInterval.Get(), p.cfg.ClusterStateDebounceWindow.Get()),
	)
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

// Close stops the watchers via the embedded WatcherGroup and releases the
// cached per-target token exchanges.
func (p *kubeConfigClusterProvider) Close() {
	p.WatcherGroup.Close()

	p.targetConfigsMu.Lock()
	for _, cached := range p.targetConfigs {
		cached.config.CloseIdleConnections()
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

// GetTokenExchangeConfig returns the token exchange configuration for the
// specified target. Returns nil if no per-target exchange is configured.
// The empty target resolves to the default context, matching the ACM hub
// provider's contract.
func (p *kubeConfigClusterProvider) GetTokenExchangeConfig(target string) *tokenexchange.TargetTokenExchangeConfig {
	if target == "" {
		target = p.GetDefaultTarget()
	}
	p.mu.RLock()
	base, pc := p.cfg, p.providerConfig(p.cfg)
	p.mu.RUnlock()
	if pc == nil {
		return nil
	}
	t, ok := pc.Targets[target]
	if !ok {
		return nil
	}

	resolved := resolveTargetConfig(base, t)
	key := targetTokenExchangeCacheKey(resolved)

	p.targetConfigsMu.Lock()
	defer p.targetConfigsMu.Unlock()

	if cached, ok := p.targetConfigs[target]; ok {
		if cached.key == key {
			return cached.config
		}
		// Resolved values changed (e.g. SIGHUP rotated the trusted CA): drop
		// idle connections so they are not reused with stale settings.
		cached.config.CloseIdleConnections()
	}
	if p.targetConfigs == nil {
		p.targetConfigs = make(map[string]*cachedTargetConfig)
	}
	p.targetConfigs[target] = &cachedTargetConfig{key: key, config: resolved}
	return resolved
}

// GetTokenExchangeStrategy returns the token exchange strategy to use
// (e.g. "keycloak-v1" or "rfc8693") for configured targets. The strategy is
// shared by all targets and required whenever any target is configured
// (enforced at config load).
func (p *kubeConfigClusterProvider) GetTokenExchangeStrategy() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pc := p.providerConfig(p.cfg)
	if pc == nil {
		return ""
	}
	return pc.TokenExchangeStrategy
}

// resolveTargetConfig maps a target's TOML configuration onto the
// exchanger's configuration. The exchange semantics are fully specified by
// the target itself; transport settings (CA, TLS versions and cipher
// suites) come from the live server config so a SIGHUP applies without a
// restart, and the per-target cache key covers them so a rotation rebuilds
// the memoized HTTP client.
func resolveTargetConfig(base *config.Config, t KubeConfigTargetConfig) *tokenexchange.TargetTokenExchangeConfig {
	caFile := t.CertificateAuthority
	if caFile == "" {
		caFile = base.CertificateAuthority.Get()
	}
	cfg := &tokenexchange.TargetTokenExchangeConfig{
		TokenURL:           t.TokenURL,
		Audience:           t.Audience,
		SubjectTokenType:   t.SubjectTokenType,
		RequestedTokenType: t.RequestedTokenType,
		SubjectIssuer:      t.SubjectIssuer,
		Scopes:             append([]string(nil), t.Scopes...),
		CAFile:             caFile,
		TLSMinVersion:      base.TLSMinVersion.Get(),
		TLSCipherSuites:    append([]string(nil), base.TLSCipherSuites.Get()...),
	}
	if t.ClientAuth.isSet() {
		a := t.ClientAuth
		setClientAuth(cfg, a.Method, a.ClientID, a.ClientSecret, a.CertificateFile, a.PrivateKeyFile, a.TokenFile)
	}
	return cfg
}

// validateTargetTokenExchanges fails provider construction when a configured
// target cannot resolve to a usable exchange. contexts are the context names
// found in the kubeconfig: every configured target must match one, so a
// mistyped context name fails loudly instead of silently skipping the
// exchange it was meant to configure. Field syntax (including the required
// token_url and token_exchange_strategy) is validated at config load, and
// the require_oauth and cluster_auth_mode rules run with config validation
// (Config.ValidateClusterAuthMode, via ClusterAuthValidator), so they are
// enforced at startup and on reload alike.
func validateTargetTokenExchanges(base *config.Config, pc *KubeConfigProviderConfig, contexts []string) error {
	if pc == nil || len(pc.Targets) == 0 {
		return nil
	}

	sortedContexts := slices.Sorted(slices.Values(contexts))
	names := make([]string, 0, len(pc.Targets))
	for name := range pc.Targets {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		if !slices.Contains(sortedContexts, name) {
			errs = append(errs, fmt.Errorf("target %q: no matching context in the kubeconfig (available contexts: %s)", name, strings.Join(sortedContexts, ", ")))
		}
		if err := resolveTargetConfig(base, pc.Targets[name]).Validate(); err != nil {
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
func targetTokenExchangeCacheKey(c *tokenexchange.TargetTokenExchangeConfig) tokenExchangeConfigCacheKey {
	return tokenExchangeConfigCacheKey{
		TokenURL:           c.TokenURL,
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
