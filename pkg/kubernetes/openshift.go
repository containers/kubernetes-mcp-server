package kubernetes

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/openshift"
)

func (m *Manager) IsOpenShift(ctx context.Context) bool {
	// This method should be fast and not block (it's called at startup)
	k, err := m.Derived(ctx)
	if err != nil {
		return false
	}
	return openshift.IsOpenshift(k.DiscoveryClient())
}
