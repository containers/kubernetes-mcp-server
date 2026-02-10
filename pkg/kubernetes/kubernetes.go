package kubernetes

import (
	"fmt"
	"net/http"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

type HeaderKey string

const (
	CustomAuthorizationHeader = HeaderKey("kubernetes-authorization")
	OAuthAuthorizationHeader  = HeaderKey("Authorization")
	UserAgentHeader           = HeaderKey("User-Agent")

	CustomUserAgent = "kubernetes-mcp-server/bearer-token-auth"
)

type CloseWatchKubeConfig func() error

var Scheme = scheme.Scheme
var ParameterCodec = runtime.NewParameterCodec(Scheme)

// Kubernetes is a limited Kubernetes Client delegating interface to the standard kubernetes.Clientset
// Only a limited set of functions are implemented with a single point of access to the kubernetes API where
// apiVersion and kinds are checked for allowed access
type Kubernetes struct {
	kubernetes.Interface
	config          api.BaseConfig
	clientCmdConfig clientcmd.ClientConfig
	restConfig      *rest.Config
	restMapper      meta.ResettableRESTMapper
	discoveryClient discovery.CachedDiscoveryInterface
	dynamicClient   dynamic.Interface
	metricsV1beta1  *metricsv1beta1.MetricsV1beta1Client
}

var _ api.KubernetesClient = (*Kubernetes)(nil)

func NewKubernetes(config api.BaseConfig, clientCmdConfig clientcmd.ClientConfig, restConfig *rest.Config) (*Kubernetes, error) {
	k := &Kubernetes{
		config:          config,
		clientCmdConfig: clientCmdConfig,
		restConfig:      rest.CopyConfig(restConfig),
	}
	if k.restConfig.UserAgent == "" {
		k.restConfig.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	k.restConfig.Wrap(func(original http.RoundTripper) http.RoundTripper {
		return &AccessControlRoundTripper{
			delegate:                original,
			deniedResourcesProvider: config,
			restMapperProvider:      func() meta.RESTMapper { return k.restMapper },
		}
	})
	k.restConfig.Wrap(func(original http.RoundTripper) http.RoundTripper {
		return &UserAgentRoundTripper{delegate: original}
	})
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(k.restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}
	k.discoveryClient = memory.NewMemCacheClient(discoveryClient)
	k.restMapper = restmapper.NewDeferredDiscoveryRESTMapper(k.discoveryClient)
	k.Interface, err = kubernetes.NewForConfig(k.restConfig)
	if err != nil {
		return nil, err
	}
	k.dynamicClient, err = dynamic.NewForConfig(k.restConfig)
	if err != nil {
		return nil, err
	}
	k.metricsV1beta1, err = metricsv1beta1.NewForConfig(k.restConfig)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (k *Kubernetes) RESTConfig() *rest.Config {
	return k.restConfig
}

func (k *Kubernetes) RESTMapper() meta.ResettableRESTMapper {
	return k.restMapper
}

func (k *Kubernetes) DiscoveryClient() discovery.CachedDiscoveryInterface {
	return k.discoveryClient
}

func (k *Kubernetes) DynamicClient() dynamic.Interface {
	return k.dynamicClient
}

func (k *Kubernetes) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return k.metricsV1beta1
}

func (k *Kubernetes) configuredNamespace() string {
	if ns, _, nsErr := k.ToRawKubeConfigLoader().Namespace(); nsErr == nil {
		return ns
	}
	return ""
}

func (k *Kubernetes) NamespaceOrDefault(namespace string) string {
	if namespace == "" {
		return k.configuredNamespace()
	}
	return namespace
}

func (k *Kubernetes) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return k.DiscoveryClient(), nil
}

func (k *Kubernetes) ToRESTMapper() (meta.RESTMapper, error) {
	return k.RESTMapper(), nil
}

// ToRESTConfig returns the rest.Config object (genericclioptions.RESTClientGetter)
func (k *Kubernetes) ToRESTConfig() (*rest.Config, error) {
	return k.RESTConfig(), nil
}

// ToRawKubeConfigLoader returns the clientcmd.ClientConfig object (genericclioptions.RESTClientGetter)
func (k *Kubernetes) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return k.clientCmdConfig
}
