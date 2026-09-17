package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
)

// singleClusterProvider implements Provider for managing a single
// Kubernetes cluster. Used for in-cluster deployments or when multi-cluster
// support is disabled.
type singleClusterProvider struct {
	mu  sync.RWMutex
	cfg *config.Config
	*ProviderGVKFilter
	strategy            string
	manager             *Manager
	kubeconfigWatcher   *watcher.Kubeconfig
	clusterStateWatcher *watcher.ClusterState
}

var _ Provider = &singleClusterProvider{}

func init() {
	RegisterProvider(config.ClusterProviderInCluster, newSingleClusterProvider(config.ClusterProviderInCluster))
	RegisterProvider(config.ClusterProviderDisabled, newSingleClusterProvider(config.ClusterProviderDisabled))
}

// newSingleClusterProvider creates a provider that manages a single cluster.
// When used within a cluster or with an 'in-cluster' strategy, it uses an InClusterManager.
// Otherwise, it uses a KubeconfigManager.
func newSingleClusterProvider(strategy string) ProviderFactory {
	return func(ctx context.Context, cfg *config.Config) (Provider, error) {
		ret := &singleClusterProvider{
			cfg:      cfg,
			strategy: strategy,
		}
		if err := ret.reset(ctx); err != nil {
			return nil, err
		}
		ret.ProviderGVKFilter = NewProviderGVKFilter(ret)
		return ret, nil
	}
}

func (p *singleClusterProvider) reset(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.resetLocked(ctx)
}

func (p *singleClusterProvider) resetLocked(ctx context.Context) error {
	if p.cfg != nil && p.cfg.KubeConfig.Get() != "" && p.strategy == config.ClusterProviderInCluster {
		return fmt.Errorf("kubeconfig file %s cannot be used with the in-cluster ClusterProviderStrategy",
			p.cfg.KubeConfig.Get())
	}

	var (
		manager *Manager
		err     error
	)
	if p.strategy == config.ClusterProviderInCluster || IsInCluster(p.cfg.KubeConfig.Get()) {
		manager, err = NewInClusterManager(ctx, p.cfg)
	} else {
		manager, err = NewKubeconfigManager(ctx, p.cfg, "")
	}
	if err != nil {
		if errors.Is(err, ErrorInClusterNotInCluster) {
			return fmt.Errorf("server must be deployed in cluster for the %s ClusterProviderStrategy: %w",
				p.strategy, err)
		}
		return err
	}

	p.Close()
	if p.manager != nil {
		p.manager.Close()
	}
	p.manager = manager
	p.kubeconfigWatcher = watcher.NewKubeconfig(ctx, p.manager.kubernetes.clientCmdConfig, p.cfg.KubeconfigDebounceWindow.Get())
	p.clusterStateWatcher = watcher.NewClusterState(ctx, p.manager.kubernetes.DiscoveryClient(), p.cfg.ClusterStatePollInterval.Get(), p.cfg.ClusterStateDebounceWindow.Get())
	return nil
}

func (p *singleClusterProvider) IsTargetCompatibilityToolFiltersEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg.EnableTargetCompatibilityToolFilters.Get()
}

func (p *singleClusterProvider) IsMultiTarget() bool {
	return false
}

func (p *singleClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	return []string{""}, nil
}

func (p *singleClusterProvider) GetTargetManagers(_ context.Context) ([]*Manager, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return []*Manager{p.manager}, nil
}

func (p *singleClusterProvider) GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error) {
	if target != "" {
		return nil, fmt.Errorf("unable to get manager for other context/cluster with %s strategy: %w", p.strategy, ErrUnknownTarget)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.manager == nil {
		return nil, errors.New("kubernetes manager is not initialized")
	}
	return p.manager.Derived(ctx)
}

func (p *singleClusterProvider) GetDefaultTarget() string {
	return ""
}

func (p *singleClusterProvider) ReloadConfig(_ context.Context, cfg *config.Config) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg
	return nil
}

func (p *singleClusterProvider) PublishKubernetesConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.manager != nil {
		p.manager.SetConfig(cfg)
	}
}

func (p *singleClusterProvider) GetTargetParameterName() string {
	return ""
}

func (p *singleClusterProvider) WatchTargets(ctx context.Context, reload McpReloader) {
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

func (p *singleClusterProvider) Close() {
	for _, w := range []watcher.Watcher{p.kubeconfigWatcher, p.clusterStateWatcher} {
		if !reflect.ValueOf(w).IsNil() {
			w.Close()
		}
	}
}
