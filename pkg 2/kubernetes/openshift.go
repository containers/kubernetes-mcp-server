package kubernetes

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (m *Manager) IsOpenShift(_ context.Context) bool {
	// This method should be fast and not block (it's called at startup)
	_, err := m.discoveryClient.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   "project.openshift.io",
		Version: "v1",
	}.String())
	return err == nil
}
