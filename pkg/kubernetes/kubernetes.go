package kubernetes

import (
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

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

type Kubernetes struct {
	manager *Manager
}

// AccessControlRestClient returns the access-controlled rest.Interface
// This ensures that any denied resources configured in the system are properly enforced
func (k *Kubernetes) AccessControlRestClient() (rest.Interface, error) {
	config, err := k.manager.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	config.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return &AccessControlRoundTripper{
			delegate:                rt,
			accessControlRESTMapper: k.manager.accessControlRESTMapper,
		}
	}

	client, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// AccessControlClientset returns the access-controlled clientset
// This ensures that any denied resources configured in the system are properly enforced
func (k *Kubernetes) AccessControlClientset() *AccessControlClientset {
	return k.manager.accessControlClientSet
}

var Scheme = scheme.Scheme
var ParameterCodec = runtime.NewParameterCodec(Scheme)

func (k *Kubernetes) NewHelm() *helm.Helm {
	// This is a derived Kubernetes, so it already has the Helm initialized
	return helm.NewHelm(k.manager)
}

// NewKiali returns a Kiali client initialized with the same StaticConfig and bearer token
// as the underlying derived Kubernetes manager.
func (k *Kubernetes) NewKiali() *kiali.Kiali {
	return kiali.NewKiali(k.manager.staticConfig, k.manager.cfg)
}
