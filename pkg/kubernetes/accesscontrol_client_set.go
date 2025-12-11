package kubernetes

import (
	"fmt"
	"net/http"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

// AccessControlClientset is a limited clientset delegating interface to the standard kubernetes.Clientset
// Only a limited set of functions are implemented with a single point of access to the kubernetes API where
// apiVersion and kinds are checked for allowed access
type AccessControlClientset struct {
	kubernetes.Interface
	config          api.BaseConfig
	clientCmdConfig clientcmd.ClientConfig
	restConfig      *rest.Config
	restMapper      meta.ResettableRESTMapper
	discoveryClient discovery.CachedDiscoveryInterface
	dynamicClient   dynamic.Interface
	metricsV1beta1  *metricsv1beta1.MetricsV1beta1Client
}

var _ api.KubernetesClientSet = (*AccessControlClientset)(nil)

func NewAccessControlClientset(config api.BaseConfig, clientCmdConfig clientcmd.ClientConfig, restConfig *rest.Config) (*AccessControlClientset, error) {
	acc := &AccessControlClientset{
		config:          config,
		clientCmdConfig: clientCmdConfig,
		restConfig:      rest.CopyConfig(restConfig),
	}
	if acc.restConfig.UserAgent == "" {
		acc.restConfig.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	acc.restConfig.Wrap(func(original http.RoundTripper) http.RoundTripper {
		return &AccessControlRoundTripper{
			delegate:                original,
			deniedResourcesProvider: config,
			restMapper:              acc.restMapper,
		}
	})
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(acc.restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %v", err)
	}
	acc.discoveryClient = memory.NewMemCacheClient(discoveryClient)
	acc.restMapper = restmapper.NewDeferredDiscoveryRESTMapper(acc.discoveryClient)
	acc.Interface, err = kubernetes.NewForConfig(acc.restConfig)
	if err != nil {
		return nil, err
	}
	acc.dynamicClient, err = dynamic.NewForConfig(acc.restConfig)
	if err != nil {
		return nil, err
	}
	acc.metricsV1beta1, err = metricsv1beta1.NewForConfig(acc.restConfig)
	if err != nil {
		return nil, err
	}
	return acc, nil
}

func (a *AccessControlClientset) RESTConfig() *rest.Config {
	return a.restConfig
}

func (a *AccessControlClientset) RESTMapper() meta.ResettableRESTMapper {
	return a.restMapper
}

func (a *AccessControlClientset) DiscoveryClient() discovery.CachedDiscoveryInterface {
	return a.discoveryClient
}

func (a *AccessControlClientset) DynamicClient() dynamic.Interface {
	return a.dynamicClient
}

func (a *AccessControlClientset) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return a.metricsV1beta1
}

func (a *AccessControlClientset) configuredNamespace() string {
	if ns, _, nsErr := a.ToRawKubeConfigLoader().Namespace(); nsErr == nil {
		return ns
	}
	return ""
}

func (a *AccessControlClientset) NamespaceOrDefault(namespace string) string {
	if namespace == "" {
		return a.configuredNamespace()
	}
	return namespace
}

func (a *AccessControlClientset) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return a.DiscoveryClient(), nil
}

func (a *AccessControlClientset) ToRESTMapper() (meta.RESTMapper, error) {
	return a.RESTMapper(), nil
}

// ToRESTConfig returns the rest.Config object (genericclioptions.RESTClientGetter)
func (a *AccessControlClientset) ToRESTConfig() (*rest.Config, error) {
	return a.RESTConfig(), nil
}

// ToRawKubeConfigLoader returns the clientcmd.ClientConfig object (genericclioptions.RESTClientGetter)
func (a *AccessControlClientset) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return a.clientCmdConfig
}
