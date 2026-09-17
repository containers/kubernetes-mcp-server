package kubernetes

import (
	"context"
	"errors"
	"sync"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

// ErrUnknownTarget is returned by GetDerivedKubernetes when the requested
// target (context, workspace, etc.) does not exist or cannot be used.
var ErrUnknownTarget = errors.New("unknown target")

// McpReloader serializes kubeconfig/cluster-state reloads with SIGHUP.
//
// Reapply recomputes the MCP tool surface and acquires the server reload lock.
// Do runs fn while holding that same lock; fn must not call Reapply (deadlock).
// Apply recomputes toolsets; the caller must already be inside Do.
type McpReloader struct {
	Reapply func() error
	Do      func(fn func() error) error
	Apply   func() error
}

// McpReloaderFromCallback builds a reloader for tests that only need a
// change notification. Do runs fn without extra locking.
func McpReloaderFromCallback(cb func() error) McpReloader {
	return McpReloader{
		Reapply: cb,
		Apply:   cb,
		Do:      func(fn func() error) error { return fn() },
	}
}

func (r McpReloader) Run(fn func() error) error {
	if r.Do != nil {
		return r.Do(fn)
	}
	return fn()
}

func (r McpReloader) ApplyToolsets() error {
	if r.Apply != nil {
		return r.Apply()
	}
	if r.Reapply != nil {
		return r.Reapply()
	}
	return nil
}

func (r McpReloader) ClusterStateCallback() func() error {
	if r.Reapply != nil {
		return r.Reapply
	}
	return func() error { return nil }
}

func (r McpReloader) Armed() bool {
	return r.Reapply != nil || r.Do != nil || r.Apply != nil
}

// ManagerProvider provides access to the underlying Manager instances for each target.
type ManagerProvider interface {
	// GetTargetManagers returns managers for all targets.
	// Returns an error if managers for any target cannot be retrieved.
	GetTargetManagers(ctx context.Context) ([]*Manager, error)
}

type Provider interface {
	// Embed the base TargetProvider and FilteringProvider interfaces
	api.TargetProvider
	api.FilteringProvider
	// GetDerivedKubernetes returns a Kubernetes client for the specified target
	GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error)
	// WatchTargets sets up a watcher for changes in the cluster targets and
	// invokes reload when changes are detected.
	WatchTargets(ctx context.Context, reload McpReloader)
	// ReloadConfig replaces the provider's Config and rebuilds managers so
	// values copied into clients at construction (denied_resources,
	// validation, confirmation) pick up a SIGHUP reload. reset() replaces
	// watchers; implementations re-arm WatchTargets afterward.
	ReloadConfig(ctx context.Context, cfg *config.Config) error
	Close()
}

// WatchTargetsRegistration remembers the WatchTargets callback so ReloadConfig
// can re-arm watchers after reset() replaces them.
type WatchTargetsRegistration struct {
	mu     sync.Mutex
	ctx    context.Context
	reload McpReloader
}

func (w *WatchTargetsRegistration) Store(ctx context.Context, reload McpReloader) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ctx = ctx
	w.reload = reload
}

func (w *WatchTargetsRegistration) Rearm(watch func(context.Context, McpReloader)) {
	w.mu.Lock()
	ctx, reload := w.ctx, w.reload
	w.mu.Unlock()
	if reload.Armed() {
		watch(ctx, reload)
	}
}

// TokenExchangeProvider is an optional interface that providers can implement to suport per-target token exchange.
//
// When a provider implements this interface and GetTokenExchangeConfig returns a non-nil config for a target, token
// exchange will be performed before creating the derived Kubernetes client. The exchanged token replaces the original
// in the Authorization header used by the derived client.
//
// If GetTokenExchangeConfig returns nil for a target, or the interface is not implemented for a provider, no per-target
// token exchange is performed and the original token is used as-is.
type TokenExchangeProvider interface {
	// GetTokenExchangeConfig returns the token exchange configuration for the specified target.
	// Returns nil if no per-target exchange is configured
	GetTokenExchangeConfig(target string) *tokenexchange.TargetTokenExchangeConfig

	// GetTokenExchangeStrategy returns the token exchange strategy to use (e.g. "keycloak-v1" or "rfc8693").
	GetTokenExchangeStrategy() string
}

type ProviderOption func(*providerOptions)

type providerOptions struct {
	oauthState     *oauth.State
	configProvider func() *config.Config
}

func WithTokenExchange(oauthState *oauth.State) ProviderOption {
	return func(opts *providerOptions) {
		opts.oauthState = oauthState
	}
}

func WithConfigProvider(configProvider func() *config.Config) ProviderOption {
	return func(opts *providerOptions) {
		opts.configProvider = configProvider
	}
}

func NewProvider(ctx context.Context, cfg *config.Config, opts ...ProviderOption) (Provider, error) {
	var providerOpts providerOptions
	for _, opt := range opts {
		opt(&providerOpts)
	}

	strategy := resolveStrategy(cfg)

	factory, err := getProviderFactory(strategy)
	if err != nil {
		return nil, err
	}

	provider, err := factory(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if providerOpts.oauthState != nil {
		configProvider := providerOpts.configProvider
		if configProvider == nil {
			configProvider = func() *config.Config {
				return cfg
			}
		}
		provider = newTokenExchangingProvider(
			provider,
			configProvider,
			providerOpts.oauthState,
		)
	}

	return provider, nil
}

func resolveStrategy(cfg *config.Config) string {
	if cfg.ClusterProviderStrategy.Get() != "" {
		return cfg.ClusterProviderStrategy.Get()
	}

	if cfg.KubeConfig.Get() != "" {
		return config.ClusterProviderKubeConfig
	}

	if _, inClusterConfigErr := InClusterConfig(); inClusterConfigErr == nil {
		return config.ClusterProviderInCluster
	}

	return config.ClusterProviderKubeConfig
}
