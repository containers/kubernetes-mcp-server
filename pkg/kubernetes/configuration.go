package kubernetes

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
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
		cfg.Contexts["context"] = &clientcmdapi.Context{
			Cluster:  "cluster",
			AuthInfo: "user",
		}
		cfg.CurrentContext = "context"
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

// ContextsList returns the available contexts from kubeconfig
// Returns a map of context names to cluster servers and the current active context
func (k *Kubernetes) ContextsList() (map[string]string, string, error) {
	if k.manager.IsInCluster() {
		// In cluster mode, there's only one context
		return map[string]string{"cluster": k.manager.cfg.Host}, "cluster", nil
	}

	cfg, err := k.manager.clientCmdConfig.RawConfig()
	if err != nil {
		return nil, "", err
	}

	contexts := make(map[string]string)
	for contextName, context := range cfg.Contexts {
		cluster, exists := cfg.Clusters[context.Cluster]
		if exists {
			contexts[contextName] = cluster.Server
		} else {
			contexts[contextName] = "unknown"
		}
	}

	return contexts, cfg.CurrentContext, nil
}

// newManagerForContext creates a new Manager instance configured for the specified context
func (m *Manager) newManagerForContext(contextName string) (*Manager, error) {
	if m.IsInCluster() {
		// In cluster mode, context switching is not applicable
		return m, nil
	}

	if contextName == "" {
		// Empty context means use default, return current manager
		return m, nil
	}

	// Create new client config with context override
	pathOptions := clientcmd.NewDefaultPathOptions()
	if m.staticConfig.KubeConfig != "" {
		pathOptions.LoadingRules.ExplicitPath = m.staticConfig.KubeConfig
	}

	clientCmdConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		pathOptions.LoadingRules,
		&clientcmd.ConfigOverrides{
			CurrentContext: contextName,
		})

	// Create new rest config for the context
	cfg, err := clientCmdConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	// Create new manager with context-specific config
	newManager := &Manager{
		cfg:             cfg,
		clientCmdConfig: clientCmdConfig,
		staticConfig:    m.staticConfig,
	}

	// Initialize clients for new manager
	newManager.accessControlClientSet, err = NewAccessControlClientset(newManager.cfg, newManager.staticConfig)
	if err != nil {
		return nil, err
	}

	// Initialize discovery client and REST mapper
	newManager.discoveryClient = memory.NewMemCacheClient(newManager.accessControlClientSet.DiscoveryClient())
	newManager.accessControlRESTMapper = NewAccessControlRESTMapper(
		restmapper.NewDeferredDiscoveryRESTMapper(newManager.discoveryClient),
		newManager.staticConfig,
	)

	// Initialize dynamic client for Kubernetes API operations
	newManager.dynamicClient, err = dynamic.NewForConfig(newManager.cfg)
	if err != nil {
		return nil, err
	}

	return newManager, nil
}
