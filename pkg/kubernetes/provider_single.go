package kubernetes

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// singleClusterProvider implements ManagerProvider for managing a single
// Kubernetes cluster. Used for in-cluster deployments or when multi-cluster
// support is disabled.
type singleClusterProvider struct {
	strategy string
	manager  *Manager
}

var _ ManagerProvider = &singleClusterProvider{}

func init() {
	RegisterProvider(config.ClusterProviderInCluster, newSingleClusterProvider(config.ClusterProviderInCluster))
	RegisterProvider(config.ClusterProviderDisabled, newSingleClusterProvider(config.ClusterProviderDisabled))
}

// newSingleClusterProvider creates a provider that manages a single cluster.
// Validates that the manager is in-cluster when the in-cluster strategy is used.
func newSingleClusterProvider(strategy string) ProviderFactory {
	return func(m *Manager, cfg *config.StaticConfig) (ManagerProvider, error) {
		if strategy == config.ClusterProviderInCluster && !m.IsInCluster() {
			return nil, fmt.Errorf("server must be deployed in cluster for the in-cluster ClusterProviderStrategy")
		}

		return &singleClusterProvider{
			manager:  m,
			strategy: strategy,
		}, nil
	}
}

func (s *singleClusterProvider) GetTargets(ctx context.Context) ([]string, error) {
	return []string{""}, nil
}

func (s *singleClusterProvider) GetManagerFor(ctx context.Context, target string) (*Manager, error) {
	if target != "" {
		return nil, fmt.Errorf("unable to get manager for other context/cluster with %s strategy", s.strategy)
	}

	return s.manager, nil
}

func (s *singleClusterProvider) GetDefaultTarget() string {
	return ""
}

func (s *singleClusterProvider) GetTargetParameterName() string {
	return ""
}

func (s *singleClusterProvider) WatchTargets(watch func() error) {
	s.manager.WatchKubeConfig(watch)
}

func (s *singleClusterProvider) Close() {
	s.manager.Close()
}
