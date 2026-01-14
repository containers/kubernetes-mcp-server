package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
)

// singleClusterProvider implements Provider for managing a single
// Kubernetes cluster. Used for in-cluster deployments or when multi-cluster
// support is disabled.
type singleClusterProvider struct {
	config              api.BaseConfig
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
	return func(cfg api.BaseConfig) (Provider, error) {
		ret := &singleClusterProvider{
			config:   cfg,
			strategy: strategy,
		}
		if err := ret.reset(); err != nil {
			return nil, err
		}
		return ret, nil
	}
}

func (p *singleClusterProvider) reset() error {
	if p.config != nil && p.config.GetKubeConfigPath() != "" && p.strategy == api.ClusterProviderInCluster {
		return fmt.Errorf("kubeconfig file %s cannot be used with the in-cluster ClusterProviderStrategy",
			p.config.GetKubeConfigPath())
	}

	var err error
	if p.strategy == api.ClusterProviderInCluster || IsInCluster(p.config) {
		p.manager, err = NewInClusterManager(p.config)
	} else {
		p.manager, err = NewKubeconfigManager(p.config, "")
	}
	if err != nil {
		if errors.Is(err, ErrorInClusterNotInCluster) {
			return fmt.Errorf("server must be deployed in cluster for the %s ClusterProviderStrategy: %w",
				p.strategy, err)
		}
		return err
	}

	p.Close()
	p.kubeconfigWatcher = watcher.NewKubeconfig(p.manager.kubernetes.clientCmdConfig)
	p.clusterStateWatcher = watcher.NewClusterState(p.manager.kubernetes.DiscoveryClient())
	return nil
}

func (p *singleClusterProvider) IsOpenShift(ctx context.Context) bool {
	return p.manager.IsOpenShift(ctx)
}

func (p *singleClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	return []string{""}, nil
}

func (p *singleClusterProvider) GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error) {
	if target != "" {
		return nil, fmt.Errorf("unable to get manager for other context/cluster with %s strategy", p.strategy)
	}

	return p.manager.Derived(ctx)
}

func (p *singleClusterProvider) GetDefaultTarget() string {
	return ""
}

func (p *singleClusterProvider) GetTargetParameterName() string {
	return ""
}

func (p *singleClusterProvider) WatchTargets(reload McpReload) {
	reloadWithReset := func() error {
		if err := p.reset(); err != nil {
			return err
		}
		p.WatchTargets(reload)
		return reload()
	}
	p.kubeconfigWatcher.Watch(reloadWithReset)
	p.clusterStateWatcher.Watch(reload)
}

func (p *singleClusterProvider) Close() {
	for _, w := range []watcher.Watcher{p.kubeconfigWatcher, p.clusterStateWatcher} {
		if !reflect.ValueOf(w).IsNil() {
			w.Close()
		}
	}
}
