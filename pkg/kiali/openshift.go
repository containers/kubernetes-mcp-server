package kiali

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var openshiftProjectGVK = schema.GroupVersionKind{
	Group:   "project.openshift.io",
	Version: "v1",
	Kind:    "Project",
}

// IsOpenShiftFromInspector reports whether any connected cluster is OpenShift.
// On discovery errors, retain OpenShift-specific RBAC requirements as a conservative bound.
func IsOpenShiftFromInspector(ctx context.Context, inspector api.ClusterInspector) bool {
	if inspector == nil {
		return false
	}
	return api.AnyTargetHasGVK(ctx, inspector, openshiftProjectGVK)
}
