package kiali

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// KialiGVK is the GroupVersionKind for the Kiali custom resource installed by
// the Kiali Operator (kialis.kiali.io CRD).
var KialiGVK = schema.GroupVersionKind{
	Group:   "kiali.io",
	Version: "v1alpha1",
	Kind:    "Kiali",
}

// HasKiali returns a TargetCompatibilityFilter that checks whether any
// target cluster has the Kiali GVK registered.
func HasKiali(p api.FilteringProvider) func() bool {
	return func() bool {
		return p.AnyTargetHasGVKs(context.TODO(), []schema.GroupVersionKind{KialiGVK})
	}
}
