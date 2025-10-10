package kubernetes

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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

// resolveKubernetesConfigurations resolves the required kubernetes configurations and sets them in the Kubernetes struct
func resolveKubernetesConfigurations(kubernetes *Manager) error {
	// Always set clientCmdConfig
	pathOptions := clientcmd.NewDefaultPathOptions()
	if kubernetes.staticConfig.KubeConfig != "" {
		pathOptions.LoadingRules.ExplicitPath = kubernetes.staticConfig.KubeConfig
	}
	kubernetes.clientCmdConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		pathOptions.LoadingRules,
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: ""}})
	var err error
	if kubernetes.IsInCluster() {
		kubernetes.cfg, err = InClusterConfig()
		if err == nil && kubernetes.cfg != nil {
			return nil
		}
	}
	// Out of cluster
	kubernetes.cfg, err = kubernetes.clientCmdConfig.ClientConfig()
	if kubernetes.cfg != nil && kubernetes.cfg.UserAgent == "" {
		kubernetes.cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	return err
}

func (k *Kubernetes) NamespaceOrDefault(namespace string) string {
	return k.manager.NamespaceOrDefault(namespace)
}

// ConfigurationContextsDefault returns the current context name
// TODO: Should be moved to the Provider level ?
func (k *Kubernetes) ConfigurationContextsDefault() (string, error) {
	if k.manager.IsInCluster() {
		return inClusterKubeConfigDefaultContext, nil
	}
	cfg, err := k.manager.clientCmdConfig.RawConfig()
	if err != nil {
		return "", err
	}
	return cfg.CurrentContext, nil
}

// ConfigurationContextsList returns the list of available context names
// TODO: Should be moved to the Provider level ?
func (k *Kubernetes) ConfigurationContextsList() (map[string]string, error) {
	if k.manager.IsInCluster() {
		return map[string]string{inClusterKubeConfigDefaultContext: ""}, nil
	}
	cfg, err := k.manager.clientCmdConfig.RawConfig()
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
func (k *Kubernetes) ConfigurationView(minify bool) (runtime.Object, error) {
	var cfg clientcmdapi.Config
	var err error
	if k.manager.IsInCluster() {
		cfg = *clientcmdapi.NewConfig()
		cfg.Clusters["cluster"] = &clientcmdapi.Cluster{
			Server:                k.manager.cfg.Host,
			InsecureSkipTLSVerify: k.manager.cfg.Insecure,
		}
		cfg.AuthInfos["user"] = &clientcmdapi.AuthInfo{
			Token: k.manager.cfg.BearerToken,
		}
		cfg.Contexts[inClusterKubeConfigDefaultContext] = &clientcmdapi.Context{
			Cluster:  "cluster",
			AuthInfo: "user",
		}
		cfg.CurrentContext = inClusterKubeConfigDefaultContext
	} else if cfg, err = k.manager.clientCmdConfig.RawConfig(); err != nil {
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
