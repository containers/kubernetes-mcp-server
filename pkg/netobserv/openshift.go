package netobserv

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

var openshiftProjectGVKs = []schema.GroupVersionKind{{
	Group:   "project.openshift.io",
	Version: "v1",
	Kind:    "Project",
}}

// isOpenShiftFromProvider reports whether the connected cluster is OpenShift using the
// framework-provided FilteringProvider (same GVK signal as the cluster-state watcher).
func isOpenShiftFromProvider(ctx context.Context, provider api.FilteringProvider) bool {
	if provider == nil {
		return false
	}
	return provider.AnyTargetHasGVKs(ctx, openshiftProjectGVKs)
}

func clusterIsOpenShiftFromDiscovery(dc discovery.DiscoveryInterface) bool {
	if dc == nil {
		return false
	}
	has, err := api.HasGVKs(dc, openshiftProjectGVKs)
	return err == nil && has
}
