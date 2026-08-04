package kiali

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// KialiGVK is the GroupVersionKind for the Kiali custom resource installed by
// the Kiali Operator (kialis.kiali.io CRD).
//
// Target-compatibility filtering treats either of these as "Kiali available":
//   - the Operator CRD is registered on a target cluster (OSSM / operator installs)
//   - [toolset_configs.kiali].url is set (Helm / standalone installs that talk to
//     the Kiali HTTP API without that CRD)
//
// The URL check uses an optional api.ExtendedConfigProvider on the same value
// passed as FilteringProvider (the MCP server wraps the provider with live
// config before calling Toolset.GetTools).
var KialiGVK = schema.GroupVersionKind{
	Group:   "kiali.io",
	Version: "v1alpha1",
	Kind:    "Kiali",
}

// HasKiali returns a TargetCompatibilityFilter that is true when any target
// cluster has the Kiali Operator GVK registered, or when a Kiali URL is
// configured via toolset_configs.
func HasKiali(p api.FilteringProvider) func() bool {
	return func() bool {
		if cfg, ok := p.(api.ExtendedConfigProvider); ok && hasConfiguredURL(cfg) {
			return true
		}
		if p == nil {
			return false
		}
		return p.AnyTargetHasGVKs(context.TODO(), []schema.GroupVersionKind{KialiGVK})
	}
}

func hasConfiguredURL(cfg api.ExtendedConfigProvider) bool {
	if cfg == nil {
		return false
	}
	ext, ok := cfg.GetToolsetConfig("kiali")
	if !ok {
		return false
	}
	kc, ok := ext.(*Config)
	return ok && kc != nil && strings.TrimSpace(kc.Url) != ""
}
