package kubernetes

import (
	"k8s.io/apimachinery/pkg/runtime"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/helm"
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

// CurrentBearerToken returns the bearer token that the Kubernetes client is currently
// configured to use, or empty if none is set in the underlying rest.Config.
func (k *Kubernetes) CurrentBearerToken() string {
	if k == nil || k.manager == nil || k.manager.cfg == nil {
		return ""
	}
	return strings.TrimSpace(k.manager.cfg.BearerToken)
}
