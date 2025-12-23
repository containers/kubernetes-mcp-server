package kubernetes

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/clientcmd/api/latest"
)

const inClusterKubeConfigDefaultContext = "in-cluster"

// InClusterConfig is a variable that holds the function to get the in-cluster config
// Exposed for testing
var InClusterConfig = func() (*rest.Config, error) {
	// TODO use kubernetes.default.svc instead of resolved server
	// Currently running into: `http: server gave HTTP response to HTTPS client`
	inClusterConfig, err := rest.InClusterConfig()
	if inClusterConfig != nil {
		inClusterConfig.Host = "https://kubernetes.default.svc"
	}
	return inClusterConfig, err
}

func IsInCluster(cfg api.ClusterProvider) bool {
	// Even if running in-cluster, if a kubeconfig is provided, we consider it as out-of-cluster
	if cfg != nil && cfg.GetKubeConfigPath() != "" {
		return false
	}
	restConfig, err := InClusterConfig()
	return err == nil && restConfig != nil
}

// ConfigurationContextsDefault returns the current context name
// TODO: Should be moved to the Provider level ?
func (c *Core) ConfigurationContextsDefault() (string, error) {
	cfg, err := c.ToRawKubeConfigLoader().RawConfig()
	if err != nil {
		return "", err
	}
	return cfg.CurrentContext, nil
}

// ConfigurationContextsList returns the list of available context names
// TODO: Should be moved to the Provider level ?
func (c *Core) ConfigurationContextsList() (map[string]string, error) {
	cfg, err := c.ToRawKubeConfigLoader().RawConfig()
	if err != nil {
		return nil, err
	}
	contexts := make(map[string]string, len(cfg.Contexts))
	for name, context := range cfg.Contexts {
		cluster, ok := cfg.Clusters[context.Cluster]
		if !ok || cluster.Server == "" {
			contexts[name] = "unknown"
		} else {
			contexts[name] = cluster.Server
		}
	}
	return contexts, nil
}

// ConfigurationView returns the current kubeconfig content as a kubeconfig YAML
// If minify is true, keeps only the current-context and the relevant pieces of the configuration for that context.
// If minify is false, all contexts, clusters, auth-infos, and users are returned in the configuration.
// TODO: Should be moved to the Provider level ?
func (c *Core) ConfigurationView(minify bool) (runtime.Object, error) {
	var cfg clientcmdapi.Config
	var err error
	if cfg, err = c.ToRawKubeConfigLoader().RawConfig(); err != nil {
		return nil, err
	}
	if minify {
		if err = clientcmdapi.MinifyConfig(&cfg); err != nil {
			return nil, err
		}
	}
	//nolint:staticcheck
	if err = clientcmdapi.FlattenConfig(&cfg); err != nil {
		// ignore error
		//return "", err
	}
	return latest.Scheme.ConvertToVersion(&cfg, latest.ExternalVersion)
}
