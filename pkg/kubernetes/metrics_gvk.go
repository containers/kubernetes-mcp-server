package kubernetes

import (
	"context"

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
func HasNodeMetrics(p api.FilteringProvider) func() bool {
	return func() bool {
		return p.AnyTargetHasGVKs(context.TODO(), []schema.GroupVersionKind{NodeMetricsGVK})
	}
}

// HasPodMetrics returns a TargetCompatibilityFilter that checks whether any
// target cluster has the PodMetrics GVK registered.
func HasPodMetrics(p api.FilteringProvider) func() bool {
	return func() bool {
		return p.AnyTargetHasGVKs(context.TODO(), []schema.GroupVersionKind{PodMetricsGVK})
	}
}
