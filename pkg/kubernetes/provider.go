package kubernetes

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

type Provider interface {
	// Openshift extends the Openshift interface to provide OpenShift specific functionality to toolset providers
	// TODO: with the configurable toolset implementation and especially the multi-cluster approach
	// extending this interface might not be a good idea anymore.
	// For the kubecontext case, a user might be targeting both an OpenShift flavored cluster and a vanilla Kubernetes cluster.
	// See: https://github.com/containers/kubernetes-mcp-server/pull/372#discussion_r2421592315
	Openshift
	TokenVerifier
	GetTargets(ctx context.Context) ([]string, error)
	GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error)
	GetDefaultTarget() string
	GetTargetParameterName() string
	WatchTargets(func() error)
	Close()
}

func NewProvider(cfg *config.StaticConfig) (Provider, error) {
	m, err := NewManager(cfg)
	if err != nil {
		return nil, err
	}

	strategy := resolveStrategy(cfg, m)

	factory, err := getProviderFactory(strategy)
	if err != nil {
		return nil, err
	}

	return factory(m, cfg)
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
