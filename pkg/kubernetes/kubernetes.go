package kubernetes

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/containers/kubernetes-mcp-server/pkg/helm"
	"github.com/containers/kubernetes-mcp-server/pkg/kiali"
)

type HeaderKey string

const (
	CustomAuthorizationHeader = HeaderKey("kubernetes-authorization")
	OAuthAuthorizationHeader  = HeaderKey("Authorization")

	CustomUserAgent = "kubernetes-mcp-server/bearer-token-auth"
)

type CloseWatchKubeConfig func() error

var Scheme = scheme.Scheme
var ParameterCodec = runtime.NewParameterCodec(Scheme)

type Kubernetes struct {
	accessControlClientSet *AccessControlClientset
}

var _ helm.Kubernetes = (*Kubernetes)(nil)

// AccessControlClientset returns the access-controlled clientset
// This ensures that any denied resources configured in the system are properly enforced
func (k *Kubernetes) AccessControlClientset() *AccessControlClientset {
	return k.accessControlClientSet
}

func (k *Kubernetes) NewHelm() *helm.Helm {
	// This is a derived Kubernetes, so it already has the Helm initialized
	return helm.NewHelm(k)
}

// NewKiali returns a Kiali client initialized with the same StaticConfig and bearer token
// as the underlying derived Kubernetes manager.
func (k *Kubernetes) NewKiali() *kiali.Kiali {
	return kiali.NewKiali(k.AccessControlClientset().staticConfig, k.AccessControlClientset().cfg)
}

func (k *Kubernetes) configuredNamespace() string {
	if ns, _, nsErr := k.AccessControlClientset().ToRawKubeConfigLoader().Namespace(); nsErr == nil {
		return ns
	}
	return ""
}

func (k *Kubernetes) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return k.AccessControlClientset().DiscoveryClient(), nil
}

func (k *Kubernetes) ToRESTMapper() (meta.RESTMapper, error) {
	return k.AccessControlClientset().RESTMapper(), nil
}

// ToRESTConfig returns the rest.Config object (genericclioptions.RESTClientGetter)
func (k *Kubernetes) ToRESTConfig() (*rest.Config, error) {
	return k.AccessControlClientset().cfg, nil
}

func (k *Kubernetes) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return k.AccessControlClientset().ToRawKubeConfigLoader()
}
