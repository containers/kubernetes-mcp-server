package kubernetes

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// McpReload is a function type that defines a callback for reloading MCP toolsets (including tools, prompts, or other configurations)
type McpReload func() error

type Provider interface {
	// Openshift extends the Openshift interface to provide OpenShift specific functionality to toolset providers
	// TODO: with the configurable toolset implementation and especially the multi-cluster approach
	// extending this interface might not be a good idea anymore.
	// For the kubecontext case, a user might be targeting both an OpenShift flavored cluster and a vanilla Kubernetes cluster.
	// See: https://github.com/containers/kubernetes-mcp-server/pull/372#discussion_r2421592315
	api.Openshift
	TokenVerifier
	GetTargets(ctx context.Context) ([]string, error)
	GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error)
	GetDefaultTarget() string
	GetTargetParameterName() string
	// WatchTargets sets up a watcher for changes in the cluster targets and calls the provided McpReload function when changes are detected
	WatchTargets(reload McpReload)
	Close()
}

func NewProvider(cfg api.BaseConfig) (Provider, error) {
	strategy := resolveStrategy(cfg)

	factory, err := getProviderFactory(strategy)
	if err != nil {
		return nil, err
	}

	return factory(cfg)
}

func resolveStrategy(cfg api.BaseConfig) string {
	if cfg.GetClusterProviderStrategy() != "" {
		return cfg.GetClusterProviderStrategy()
	}

	if cfg.GetKubeConfigPath() != "" {
		return api.ClusterProviderKubeConfig
	}

	if _, inClusterConfigErr := InClusterConfig(); inClusterConfigErr == nil {
		return api.ClusterProviderInCluster
	}

	return api.ClusterProviderKubeConfig
}
