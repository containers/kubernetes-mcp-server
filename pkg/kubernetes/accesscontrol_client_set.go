package kubernetes

import (
	"fmt"
	"net/http"

	configapi "github.com/containers/kubernetes-mcp-server/pkg/api/config"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	authenticationv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	authorizationv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
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
	config          configapi.BaseConfig
	clientCmdConfig clientcmd.ClientConfig
	cfg             *rest.Config
	restMapper      meta.ResettableRESTMapper
	discoveryClient discovery.CachedDiscoveryInterface
	dynamicClient   dynamic.Interface
	metricsV1beta1  *metricsv1beta1.MetricsV1beta1Client
}

func NewAccessControlClientset(config configapi.BaseConfig, clientCmdConfig clientcmd.ClientConfig, restConfig *rest.Config) (*AccessControlClientset, error) {
	acc := &AccessControlClientset{
		config:          config,
		clientCmdConfig: clientCmdConfig,
		cfg:             rest.CopyConfig(restConfig),
	}
	if acc.cfg.UserAgent == "" {
		acc.cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	acc.cfg.Wrap(func(original http.RoundTripper) http.RoundTripper {
		return &AccessControlRoundTripper{
			delegate:                original,
			deniedResourcesProvider: config,
			restMapper:              acc.restMapper,
		}
	})
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(acc.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %v", err)
	}
	acc.discoveryClient = memory.NewMemCacheClient(discoveryClient)
	acc.restMapper = restmapper.NewDeferredDiscoveryRESTMapper(acc.discoveryClient)
	acc.Interface, err = kubernetes.NewForConfig(acc.cfg)
	if err != nil {
		return nil, err
	}
	acc.dynamicClient, err = dynamic.NewForConfig(acc.cfg)
	if err != nil {
		return nil, err
	}
	acc.metricsV1beta1, err = metricsv1beta1.NewForConfig(acc.cfg)
	if err != nil {
		return nil, err
	}
	return acc, nil
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

// Nodes returns NodeInterface
// Deprecated: use CoreV1().Nodes() directly
func (a *AccessControlClientset) Nodes() (corev1.NodeInterface, error) {
	return a.CoreV1().Nodes(), nil
}

// Pods returns PodInterface
// Deprecated: use CoreV1().Pods(namespace) directly
func (a *AccessControlClientset) Pods(namespace string) (corev1.PodInterface, error) {
	return a.CoreV1().Pods(namespace), nil
}

// Services returns ServiceInterface
// Deprecated: use CoreV1().Services(namespace) directly
func (a *AccessControlClientset) Services(namespace string) (corev1.ServiceInterface, error) {
	return a.CoreV1().Services(namespace), nil
}

// SelfSubjectAccessReviews returns SelfSubjectAccessReviewInterface
// Deprecated: use AuthorizationV1().SelfSubjectAccessReviews() directly
func (a *AccessControlClientset) SelfSubjectAccessReviews() (authorizationv1.SelfSubjectAccessReviewInterface, error) {
	return a.AuthorizationV1().SelfSubjectAccessReviews(), nil
}

// TokenReview returns TokenReviewInterface
// Deprecated: use AuthenticationV1().TokenReviews() directly
func (a *AccessControlClientset) TokenReview() (authenticationv1.TokenReviewInterface, error) {
	return a.AuthenticationV1().TokenReviews(), nil
}

// ToRawKubeConfigLoader returns the clientcmd.ClientConfig object (genericclioptions.RESTClientGetter)
func (a *AccessControlClientset) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return a.clientCmdConfig
}
