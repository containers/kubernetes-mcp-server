package kubernetes

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/runtime"
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

var Scheme = scheme.Scheme
var ParameterCodec = runtime.NewParameterCodec(Scheme)

type Kubernetes struct {
	accessControlClientSet *AccessControlClientset
}

var _ api.KubernetesClient = (*Kubernetes)(nil)

// AccessControlClientset returns the access-controlled clientset
// This ensures that any denied resources configured in the system are properly enforced
func (k *Kubernetes) AccessControlClientset() api.KubernetesClientSet {
	return k.accessControlClientSet
}
