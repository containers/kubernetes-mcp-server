package kubernetes

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// KubeConfigTargetParameterName is the parameter name used to specify
// the kubeconfig context when using the kubeconfig cluster provider strategy.
const KubeConfigTargetParameterName = "context"

// kubeConfigClusterProvider implements ManagerProvider for managing multiple
// Kubernetes clusters using different contexts from a kubeconfig file.
// It lazily initializes managers for each context as they are requested.
type kubeConfigClusterProvider struct {
	defaultContext string
	managers       map[string]*Manager
}

var _ ManagerProvider = &kubeConfigClusterProvider{}

func init() {
	RegisterProvider(config.ClusterProviderKubeConfig, newKubeConfigClusterProvider)
}

// newKubeConfigClusterProvider creates a provider that manages multiple clusters
// via kubeconfig contexts. Returns an error if the manager is in-cluster mode.
func newKubeConfigClusterProvider(m *Manager, cfg *config.StaticConfig) (ManagerProvider, error) {
	// Handle in-cluster mode
	if m.IsInCluster() {
		return nil, fmt.Errorf("kubeconfig ClusterProviderStrategy is invalid for in-cluster deployments")
	}

	rawConfig, err := m.clientCmdConfig.RawConfig()
	if err != nil {
		return nil, err
	}

	allClusterManagers := map[string]*Manager{
		rawConfig.CurrentContext: m, // we already initialized a manager for the default context, let's use it
	}

	for name := range rawConfig.Contexts {
		if name == rawConfig.CurrentContext {
			continue // already initialized this, don't want to set it to nil
		}

		allClusterManagers[name] = nil
	}

	return &kubeConfigClusterProvider{
		defaultContext: rawConfig.CurrentContext,
		managers:       allClusterManagers,
	}, nil
}

func (k *kubeConfigClusterProvider) GetTargets(ctx context.Context) ([]string, error) {
	contextNames := make([]string, 0, len(k.managers))
	for cluster := range k.managers {
		contextNames = append(contextNames, cluster)
	}

	return contextNames, nil
}

func (k *kubeConfigClusterProvider) GetTargetParameterName() string {
	return KubeConfigTargetParameterName
}

func (k *kubeConfigClusterProvider) GetManagerFor(ctx context.Context, context string) (*Manager, error) {
	m, ok := k.managers[context]
	if ok && m != nil {
		return m, nil
	}

	baseManager := k.managers[k.defaultContext]

	if baseManager.IsInCluster() {
		// In cluster mode, so context switching is not applicable
		return baseManager, nil
	}

	m, err := baseManager.newForContext(context)
	if err != nil {
		return nil, err
	}

	k.managers[context] = m

	return m, nil
}

func (k *kubeConfigClusterProvider) GetDefaultTarget() string {
	return k.defaultContext
}

func (k *kubeConfigClusterProvider) WatchTargets(onKubeConfigChanged func() error) {
	m := k.managers[k.defaultContext]

	m.WatchKubeConfig(onKubeConfigChanged)
}

func (k *kubeConfigClusterProvider) Close() {
	m := k.managers[k.defaultContext]

	m.Close()
}
