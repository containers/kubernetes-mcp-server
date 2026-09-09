package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
)

// singleClusterProvider implements Provider for managing a single
// Kubernetes cluster. Used for in-cluster deployments or when multi-cluster
// support is disabled.
type singleClusterProvider struct {
	cfg *config.Config
	*ProviderGVKFilter
	strategy            string
	manager             *Manager
	kubeconfigWatcher   *watcher.Kubeconfig
	clusterStateWatcher *watcher.ClusterState
}

var _ Provider = &singleClusterProvider{}

func init() {
	RegisterProvider(api.ClusterProviderInCluster, newSingleClusterProvider(api.ClusterProviderInCluster))
	RegisterProvider(api.ClusterProviderDisabled, newSingleClusterProvider(api.ClusterProviderDisabled))
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
	if p.cfg != nil && p.cfg.KubeConfig.Get() != "" && p.strategy == api.ClusterProviderInCluster {
		return fmt.Errorf("kubeconfig file %s cannot be used with the in-cluster ClusterProviderStrategy",
			p.cfg.KubeConfig.Get())
	}

	if p.manager != nil {
		p.manager.Close()
	}
	var err error
	if p.strategy == api.ClusterProviderInCluster || IsInCluster(p.cfg.KubeConfig.Get()) {
		p.manager, err = NewInClusterManager(ctx, p.cfg)
	} else {
		p.manager, err = NewKubeconfigManager(ctx, p.cfg, "")
	}
	if err != nil {
		if errors.Is(err, ErrorInClusterNotInCluster) {
			return fmt.Errorf("server must be deployed in cluster for the %s ClusterProviderStrategy: %w",
				p.strategy, err)
		}
		return err
	}

	p.Close()
	p.kubeconfigWatcher = watcher.NewKubeconfig(ctx, p.manager.kubernetes.clientCmdConfig, p.cfg.KubeconfigDebounceWindow.Get())
	p.clusterStateWatcher = watcher.NewClusterState(ctx, p.manager.kubernetes.DiscoveryClient(), p.cfg.ClusterStatePollInterval.Get(), p.cfg.ClusterStateDebounceWindow.Get())
	return nil
}

func (p *singleClusterProvider) IsTargetCompatibilityToolFiltersEnabled() bool {
	return p.cfg.EnableTargetCompatibilityToolFilters.Get()
}

func (p *singleClusterProvider) IsMultiTarget() bool {
	return false
}

func (p *singleClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	return []string{""}, nil
}

func (p *singleClusterProvider) GetTargetManagers(_ context.Context) ([]*Manager, error) {
	return []*Manager{p.manager}, nil
}

func (p *singleClusterProvider) GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error) {
	if target != "" {
		return nil, fmt.Errorf("unable to get manager for other context/cluster with %s strategy: %w", p.strategy, ErrUnknownTarget)
	}

	return p.manager.Derived(ctx)
}

func (p *singleClusterProvider) GetDefaultTarget() string {
	return ""
}

func (p *singleClusterProvider) GetTargetParameterName() string {
	return ""
}

func (p *singleClusterProvider) WatchTargets(ctx context.Context, reload McpReload) {
	reloadWithReset := func() error {
		if err := p.reset(ctx); err != nil {
			return err
		}
		p.WatchTargets(ctx, reload)
		return reload()
	}
	p.kubeconfigWatcher.Watch(ctx, reloadWithReset)
	p.clusterStateWatcher.Watch(ctx, reload)
}

func (p *singleClusterProvider) Close() {
	for _, w := range []watcher.Watcher{p.kubeconfigWatcher, p.clusterStateWatcher} {
		if !reflect.ValueOf(w).IsNil() {
			w.Close()
		}
	}
}
