package kiali

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var openshiftProjectGVKs = []schema.GroupVersionKind{{
	Group:   "project.openshift.io",
	Version: "v1",
	Kind:    "Project",
}}

// IsOpenShiftFromProvider reports whether the connected cluster is OpenShift using the
// framework-provided FilteringProvider (same GVK signal as the cluster-state watcher).
func IsOpenShiftFromProvider(ctx context.Context, provider api.FilteringProvider) bool {
	if provider == nil {
		return false
	}
	return provider.AnyTargetHasGVKs(ctx, openshiftProjectGVKs)
}
