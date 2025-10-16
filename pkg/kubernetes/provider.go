package kubernetes

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
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
	strategy := resolveStrategy(cfg)

	factory, err := getProviderFactory(strategy)
	if err != nil {
		return nil, err
	}

	m, err := NewManager(cfg, "")
	if err != nil {
		return nil, err
	}

	return factory(m, cfg)
}

func resolveStrategy(cfg *config.StaticConfig) string {
	if cfg.ClusterProviderStrategy != "" {
		return cfg.ClusterProviderStrategy
	}

	if cfg.KubeConfig != "" {
		return config.ClusterProviderKubeConfig
	}

	if _, inClusterConfigErr := InClusterConfig(); inClusterConfigErr == nil {
		return config.ClusterProviderInCluster
	}

	return config.ClusterProviderKubeConfig
}
