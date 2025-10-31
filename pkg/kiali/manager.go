package kiali

import (
	"context"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"k8s.io/klog/v2"
)

type Manager struct {
	BearerToken   string
	KialiURL      string
	KialiInsecure bool
}

func NewManager(config *config.StaticConfig) *Manager {
	return &Manager{
		BearerToken:   "",
		KialiURL:      config.KialiURL,
		KialiInsecure: config.KialiInsecure,
	}
}

func (m *Manager) Derived(ctx context.Context) (*Kiali, error) {
	authorization, ok := ctx.Value(internalk8s.OAuthAuthorizationHeader).(string)
	if !ok || !strings.HasPrefix(authorization, "Bearer ") {
		return &Kiali{manager: m}, nil
	}
	// Authorization header is present; nothing special is needed for the Kiali HTTP client
	klog.V(5).Infof("%s header found (Bearer), using provided bearer token", internalk8s.OAuthAuthorizationHeader)

	return &Kiali{manager: &Manager{
		BearerToken:   strings.TrimPrefix(authorization, "Bearer "),
		KialiURL:      m.KialiURL,
		KialiInsecure: m.KialiInsecure,
	}}, nil
}
