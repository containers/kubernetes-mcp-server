package kubernetes

import (
	"context"
	"errors"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

// ErrUnknownTarget is returned by GetDerivedKubernetes when the requested
// target (context, workspace, etc.) does not exist or cannot be used.
var ErrUnknownTarget = errors.New("unknown target")

// McpReload is a function type that defines a callback for reloading MCP toolsets (including tools, prompts, or other configurations)
type McpReload func() error

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
	// WatchTargets sets up a watcher for changes in the cluster targets and calls the provided McpReload function when changes are detected
	WatchTargets(ctx context.Context, reload McpReload)
	Close()
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
		return api.ClusterProviderKubeConfig
	}

	if _, inClusterConfigErr := InClusterConfig(); inClusterConfigErr == nil {
		return api.ClusterProviderInCluster
	}

	return api.ClusterProviderKubeConfig
}
