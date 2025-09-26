package kubernetes

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

type ClusterProvider interface {
	GetClusters(ctx context.Context) ([]string, error)
	GetClusterManager(ctx context.Context, cluster string) (*Manager, error)
	GetDefaultCluster() string
}

type kubeConfigClusterProvider struct {
	defaultCluster string
	managers       map[string]*Manager
}

var _ ClusterProvider = &kubeConfigClusterProvider{}

func NewClusterProvider(cfg *config.StaticConfig) (ClusterProvider, error) {
	switch cfg.ClusterProviderStrategy {
	case config.ClusterProviderKubeConfig:
		return newKubeConfigClusterProvider(cfg)
	default:
		return nil, fmt.Errorf("invalid ClusterProviderStrategy '%s', must be 'kubeconfig'", cfg.ClusterProviderStrategy)
	}
}

func newKubeConfigClusterProvider(config *config.StaticConfig) (*kubeConfigClusterProvider, error) {
	m, err := NewManager(config)
	if err != nil {
		return nil, err
	}

	rawConfig, err := m.clientCmdConfig.RawConfig()
	if err != nil {
		return nil, err
	}

	defaultContext := rawConfig.Contexts[rawConfig.CurrentContext]

	allClusterManagers := make(map[string]*Manager)

	for _, context := range rawConfig.Contexts {
		if _, exists := rawConfig.Clusters[context.Cluster]; exists {
			allClusterManagers[context.Cluster] = nil // these will be lazy initialized as they are accessed later
		}
	}

	// we already initialized a manager for the default context, let's use it
	allClusterManagers[defaultContext.Cluster] = m

	return &kubeConfigClusterProvider{
		defaultCluster: defaultContext.Cluster,
		managers:       allClusterManagers,
	}, nil
}

func (k *kubeConfigClusterProvider) GetClusters(ctx context.Context) ([]string, error) {
	clusterNames := make([]string, 0, len(k.managers))
	for cluster := range k.managers {
		clusterNames = append(clusterNames, cluster)
	}

	return clusterNames, nil
}

func (k *kubeConfigClusterProvider) GetClusterManager(ctx context.Context, cluster string) (*Manager, error) {
	m, ok := k.managers[cluster]
	if ok {
		return m, nil
	}

	baseManager := k.managers[k.defaultCluster]

	if baseManager.IsInCluster() {
		// In cluster mode, so context switching is not applicable
		return baseManager, nil
	}

	m, err := baseManager.newForCluster(cluster)
	if err != nil {
		return nil, err
	}

	k.managers[cluster] = m

	return m, nil
}

func (k *kubeConfigClusterProvider) GetDefaultCluster() string {
	return k.defaultCluster
}

func (m *Manager) newForCluster(cluster string) (*Manager, error) {
	contextName, err := m.getContextNameForCluster(cluster)
	if err != nil {
		return nil, err
	}

	pathOptions := clientcmd.NewDefaultPathOptions()
	if m.staticConfig.KubeConfig != "" {
		pathOptions.LoadingRules.ExplicitPath = m.staticConfig.KubeConfig
	}

	clientCmdConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		pathOptions.LoadingRules,
		&clientcmd.ConfigOverrides{
			CurrentContext: contextName,
		},
	)

	cfg, err := clientCmdConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	if cfg.UserAgent == "" {
		cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	manager := &Manager{
		cfg:             cfg,
		clientCmdConfig: clientCmdConfig,
		staticConfig:    m.staticConfig,
	}

	// Initialize clients for new manager
	manager.accessControlClientSet, err = NewAccessControlClientset(manager.cfg, manager.staticConfig)
	if err != nil {
		return nil, err
	}

	manager.discoveryClient = memory.NewMemCacheClient(manager.accessControlClientSet.DiscoveryClient())

	manager.accessControlRESTMapper = NewAccessControlRESTMapper(
		restmapper.NewDeferredDiscoveryRESTMapper(manager.discoveryClient),
		manager.staticConfig,
	)

	manager.dynamicClient, err = dynamic.NewForConfig(manager.cfg)
	if err != nil {
		return nil, err
	}

	return manager, nil
}

func (m *Manager) getContextNameForCluster(cluster string) (string, error) {
	rawConfig, err := m.clientCmdConfig.RawConfig()
	if err != nil {
		return "", err
	}

	// first, check if we have a defined context for this cluster
	contextName, ok := m.staticConfig.ClusterContexts[cluster]
	if ok {
		_, ok := rawConfig.Contexts[contextName]
		if !ok {
			return "", fmt.Errorf(
				"no context named '%s' found in kubeconfig, failed to get context for cluster '%s'",
				contextName,
				cluster,
			)
		}

		return contextName, nil
	}

	// iterate through all contexts until we find one that matches
	for contextName, context := range rawConfig.Contexts {
		if context.Cluster == cluster {
			return contextName, nil
		}
	}

	return "", fmt.Errorf("no contexts in kubeconfig can access cluster '%s'", cluster)
}
