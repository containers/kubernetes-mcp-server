package netobserv

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

var openshiftProjectGVK = schema.GroupVersionKind{
	Group:   "project.openshift.io",
	Version: "v1",
	Kind:    "Project",
}

// clusterIsOpenShift reports whether the connected cluster is OpenShift using the
// framework-provided cached discovery client (same GVK signal as FilteringProvider).
func clusterIsOpenShift(k8s api.KubernetesClient) bool {
	if k8s == nil {
		return false
	}
	return clusterIsOpenShiftFromDiscovery(k8s.DiscoveryClient())
}

func clusterIsOpenShiftFromDiscovery(dc discovery.DiscoveryInterface) bool {
	if dc == nil {
		return false
	}
	has, err := api.HasGVKs(dc, []schema.GroupVersionKind{openshiftProjectGVK})
	return err == nil && has
}
