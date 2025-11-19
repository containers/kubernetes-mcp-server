package kubernetes

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	"github.com/containers/kubernetes-mcp-server/pkg/helm"
	"github.com/containers/kubernetes-mcp-server/pkg/kiali"
)

type HeaderKey string

const (
	CustomClusterURLHeader    = HeaderKey("kubernetes-cluster-url")
	CustomAuthorizationHeader = HeaderKey("kubernetes-authorization")
	// CustomCertificateAuthorityData is the base64-encoded CA certificate.
	CustomCertificateAuthorityDataHeader = HeaderKey("kubernetes-certificate-authority-data")
	// CustomClientCertificateData is the base64-encoded client certificate.
	CustomClientCertificateDataHeader = HeaderKey("kubernetes-client-certificate-data")
	// CustomClientKeyData is the base64-encoded client key.
	CustomClientKeyDataHeader = HeaderKey("kubernetes-client-key-data")

	OAuthAuthorizationHeader = HeaderKey("Authorization")

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
// as the underlying derived Kubernetes manager.
func (k *Kubernetes) NewKiali() *kiali.Kiali {
	return kiali.NewKiali(k.manager.staticConfig, k.manager.cfg)
}
