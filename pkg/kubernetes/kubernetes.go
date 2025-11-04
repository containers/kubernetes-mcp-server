package kubernetes

import (
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/containers/kubernetes-mcp-server/pkg/helm"
	"github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"k8s.io/client-go/kubernetes/scheme"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
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
// as the underlying Kubernetes manager. The token is taken from the manager rest.Config.
func (k *Kubernetes) NewKiali() *kiali.Kiali {
	if k == nil || k.manager == nil || k.manager.staticConfig == nil {
		return nil
	}
	km := kiali.NewManager(k.manager.staticConfig)
	if k.manager.cfg != nil {
		km.BearerToken = k.manager.cfg.BearerToken
	}
	return km.GetKiali()
}
