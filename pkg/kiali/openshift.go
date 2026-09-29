package kiali

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	available, err := inspector.Discovery().ServerResourcesForGroupVersion(ctx, openshiftProjectGVK.GroupVersion().String()).Any(func(list *metav1.APIResourceList) bool {
		for _, resource := range list.APIResources {
			if resource.Kind == openshiftProjectGVK.Kind {
				return true
			}
		}
		return false
	})
	return available || (err != nil && !api.IsNotFound(err))
}
