package kubernetes

import (
	"fmt"
	"net/http"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
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
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

// AccessControlClientset is a limited clientset delegating interface to the standard kubernetes.Clientset
// Only a limited set of functions are implemented with a single point of access to the kubernetes API where
// apiVersion and kinds are checked for allowed access
type AccessControlClientset struct {
	cfg *rest.Config
	kubernetes.Interface
	restMapper      meta.ResettableRESTMapper
	discoveryClient discovery.CachedDiscoveryInterface
	dynamicClient   dynamic.Interface
	metricsV1beta1  *metricsv1beta1.MetricsV1beta1Client
}

func NewAccessControlClientset(staticConfig *config.StaticConfig, restConfig *rest.Config) (*AccessControlClientset, error) {
	rest.CopyConfig(restConfig)
	acc := &AccessControlClientset{
		cfg: rest.CopyConfig(restConfig),
	}
	if acc.cfg.UserAgent == "" {
		acc.cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	acc.cfg.Wrap(func(original http.RoundTripper) http.RoundTripper {
		return &AccessControlRoundTripper{
			delegate:     original,
			staticConfig: staticConfig,
			restMapper:   acc.restMapper,
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
