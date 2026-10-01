package kubernetes

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metricsv1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

var (
	// NodeMetricsGVK is the GroupVersionKind for Metrics Server NodeMetrics resources.
	NodeMetricsGVK = schema.GroupVersionKind{
		Group:   metricsv1beta1api.GroupName,
		Version: metricsv1beta1api.SchemeGroupVersion.Version,
		Kind:    "NodeMetrics",
	}

	// PodMetricsGVK is the GroupVersionKind for Metrics Server PodMetrics resources.
	PodMetricsGVK = schema.GroupVersionKind{
		Group:   metricsv1beta1api.GroupName,
		Version: metricsv1beta1api.SchemeGroupVersion.Version,
		Kind:    "PodMetrics",
	}
)

// HasNodeMetrics returns a TargetCompatibilityFilter that checks whether any
// target cluster has the NodeMetrics GVK registered.
func HasNodeMetrics(ctx context.Context, inspector api.ClusterInspector) func() bool {
	return func() bool {
		available, err := inspector.Discovery().ServerResourcesForGroupVersion(ctx, NodeMetricsGVK.GroupVersion().String()).Any(func(list *metav1.APIResourceList) bool {
			for _, resource := range list.APIResources {
				if resource.Kind == NodeMetricsGVK.Kind {
					return true
				}
			}
			return false
		})
		// Discovery errors fail open unless they confirm the resource is absent.
		return available || (err != nil && !api.IsNotFound(err))
	}
}

// HasPodMetrics returns a TargetCompatibilityFilter that checks whether any
// target cluster has the PodMetrics GVK registered.
func HasPodMetrics(ctx context.Context, inspector api.ClusterInspector) func() bool {
	return func() bool {
		available, err := inspector.Discovery().ServerResourcesForGroupVersion(ctx, PodMetricsGVK.GroupVersion().String()).Any(func(list *metav1.APIResourceList) bool {
			for _, resource := range list.APIResources {
				if resource.Kind == PodMetricsGVK.Kind {
					return true
				}
			}
			return false
		})
		// Discovery errors fail open unless they confirm the resource is absent.
		return available || (err != nil && !api.IsNotFound(err))
	}
}
