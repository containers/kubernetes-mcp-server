package kubernetes

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/clientcmd/api/latest"
)

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
	overrides := &clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: ""}}
	if kubernetes.staticConfig.KubeContext != "" {
		overrides.CurrentContext = kubernetes.staticConfig.KubeContext
	}
	kubernetes.clientCmdConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		pathOptions.LoadingRules,
		overrides)
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

func (m *Manager) IsInCluster() bool {
	if m.staticConfig.KubeConfig != "" {
		return false
	}
	cfg, err := InClusterConfig()
	return err == nil && cfg != nil
}

func (m *Manager) configuredNamespace() string {
	if ns, _, nsErr := m.clientCmdConfig.Namespace(); nsErr == nil {
		return ns
	}
	return ""
}

func (m *Manager) NamespaceOrDefault(namespace string) string {
	if namespace == "" {
		return m.configuredNamespace()
	}
	return namespace
}

func (k *Kubernetes) NamespaceOrDefault(namespace string) string {
	return k.manager.NamespaceOrDefault(namespace)
}

// ToRESTConfig returns the rest.Config object (genericclioptions.RESTClientGetter)
func (m *Manager) ToRESTConfig() (*rest.Config, error) {
	return m.cfg, nil
}

// ToRawKubeConfigLoader returns the clientcmd.ClientConfig object (genericclioptions.RESTClientGetter)
func (m *Manager) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return m.clientCmdConfig
}

func (m *Manager) ConfigurationView(minify bool) (runtime.Object, error) {
	var cfg clientcmdapi.Config
	var err error
	if m.IsInCluster() {
		cfg = *clientcmdapi.NewConfig()
		cfg.Clusters["cluster"] = &clientcmdapi.Cluster{
			Server:                m.cfg.Host,
			InsecureSkipTLSVerify: m.cfg.Insecure,
		}
		cfg.AuthInfos["user"] = &clientcmdapi.AuthInfo{
			Token: m.cfg.BearerToken,
		}
		cfg.Contexts["context"] = &clientcmdapi.Context{
			Cluster:  "cluster",
			AuthInfo: "user",
		}
		cfg.CurrentContext = "context"
	} else if cfg, err = m.clientCmdConfig.RawConfig(); err != nil {
		return nil, err
	}
	if minify {
		if m.staticConfig.KubeContext != "" {
			cfg.CurrentContext = m.staticConfig.KubeContext
		}
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
