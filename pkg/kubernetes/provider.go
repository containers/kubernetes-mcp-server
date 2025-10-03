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

const (
	KubeConfigTargetParameterName = "context"
)

type ManagerProvider interface {
	GetTargets(ctx context.Context) ([]string, error)
	GetManagerFor(ctx context.Context, target string) (*Manager, error)
	GetDefaultTarget() string
	GetTargetParameterName() string
	WatchTargets(func() error)
	Close()
}

type kubeConfigClusterProvider struct {
	defaultContext string
	managers       map[string]*Manager
}

var _ ManagerProvider = &kubeConfigClusterProvider{}

type inClusterProvider struct {
	manager *Manager
}

var _ ManagerProvider = &inClusterProvider{}

func NewManagerProvider(cfg *config.StaticConfig) (ManagerProvider, error) {
	m, err := NewManager(cfg)
	if err != nil {
		return nil, err
	}

	switch resolveStrategy(cfg, m) {
	case config.ClusterProviderKubeConfig:
		return newKubeConfigClusterProvider(m)
	case config.ClusterProviderInCluster:
		return newInClusterProvider(m)
	default:
		return nil, fmt.Errorf("invalid ClusterProviderStrategy '%s', must be 'kubeconfig' or 'in-cluster'", cfg.ClusterProviderStrategy)
	}
}

func newKubeConfigClusterProvider(m *Manager) (*kubeConfigClusterProvider, error) {
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

func newInClusterProvider(m *Manager) (*inClusterProvider, error) {
	return &inClusterProvider{
		manager: m,
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

func (i *inClusterProvider) GetTargets(ctx context.Context) ([]string, error) {
	return []string{""}, nil
}

func (i *inClusterProvider) GetManagerFor(ctx context.Context, target string) (*Manager, error) {
	if target != "" {
		return nil, fmt.Errorf("unable to get manager for other context/cluster with in-cluster strategy")
	}

	return i.manager, nil
}

func (i *inClusterProvider) GetDefaultTarget() string {
	return ""
}

func (i *inClusterProvider) GetTargetParameterName() string {
	return ""
}

func (i *inClusterProvider) WatchTargets(watch func() error) {
	i.manager.WatchKubeConfig(watch)
}

func (i *inClusterProvider) Close() {
	i.manager.Close()
}

func (m *Manager) newForContext(context string) (*Manager, error) {
	pathOptions := clientcmd.NewDefaultPathOptions()
	if m.staticConfig.KubeConfig != "" {
		pathOptions.LoadingRules.ExplicitPath = m.staticConfig.KubeConfig
	}

	clientCmdConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		pathOptions.LoadingRules,
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
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

func resolveStrategy(cfg *config.StaticConfig, m *Manager) string {
	if cfg.ClusterProviderStrategy != "" {
		return cfg.ClusterProviderStrategy
	}

	if m.IsInCluster() {
		return config.ClusterProviderInCluster
	}

	return config.ClusterProviderKubeConfig
}
