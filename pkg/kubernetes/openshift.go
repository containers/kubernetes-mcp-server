package kubernetes

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Openshift interface {
	IsOpenShift(context.Context) bool
}

func (m *Manager) IsOpenShift(ctx context.Context) bool {
	// This method should be fast and not block (it's called at startup)
	k, err := m.Derived(ctx)
	if err != nil {
		return false
	}
	_, err = k.AccessControlClientset().DiscoveryClient().ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   "project.openshift.io",
		Version: "v1",
	}.String())
	return err == nil
}
