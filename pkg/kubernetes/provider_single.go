package kubernetes

import (
	"context"
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
// Validates that the manager is in-cluster when the in-cluster strategy is used.
func newSingleClusterProvider(strategy string) ProviderFactory {
	return func(m *Manager, cfg *config.StaticConfig) (Provider, error) {
		if strategy == config.ClusterProviderInCluster && !m.IsInCluster() {
			return nil, fmt.Errorf("server must be deployed in cluster for the in-cluster ClusterProviderStrategy")
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

func (p *singleClusterProvider) GetTargets(ctx context.Context) ([]string, error) {
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
