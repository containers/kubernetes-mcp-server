package kubernetes

import (
	"context"
	"errors"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	authenticationv1api "k8s.io/api/authentication/v1"
)

// singleClusterProvider implements Provider for managing a single
// Kubernetes cluster. Used for in-cluster deployments or when multi-cluster
// support is disabled.
type singleClusterProvider struct {
	strategy string
	manager  *Manager
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
	return func(cfg *config.StaticConfig) (Provider, error) {
		if cfg != nil && cfg.KubeConfig != "" && strategy == config.ClusterProviderInCluster {
			return nil, fmt.Errorf("kubeconfig file %s cannot be used with the in-cluster ClusterProviderStrategy", cfg.KubeConfig)
		}

		var m *Manager
		var err error
		if strategy == config.ClusterProviderInCluster || IsInCluster(cfg) {
			m, err = NewInClusterManager(cfg)
		} else {
			m, err = NewKubeconfigManager(cfg, "")
		}
		if err != nil {
			if errors.Is(err, ErrorInClusterNotInCluster) {
				return nil, fmt.Errorf("server must be deployed in cluster for the %s ClusterProviderStrategy: %v", strategy, err)
			}
			return nil, err
		}

		return &singleClusterProvider{
			manager:  m,
			strategy: strategy,
		}, nil
	}
}

func (p *singleClusterProvider) IsOpenShift(ctx context.Context) bool {
	return p.manager.IsOpenShift(ctx)
}

func (p *singleClusterProvider) VerifyToken(ctx context.Context, target, token, audience string) (*authenticationv1api.UserInfo, []string, error) {
	if target != "" {
		return nil, nil, fmt.Errorf("unable to get manager for other context/cluster with %s strategy", p.strategy)
	}
	return p.manager.VerifyToken(ctx, token, audience)
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

func (p *singleClusterProvider) WatchTargets(watch func() error) {
	p.manager.WatchKubeConfig(watch)
}

func (p *singleClusterProvider) Close() {
	p.manager.Close()
}
