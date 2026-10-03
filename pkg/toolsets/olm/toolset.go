package olm

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Toolset provides Kubernetes OLM read-only tools for inspecting operator lifecycle.
type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string { return "olm" }

func (t *Toolset) GetDescription() string {
	return "Read-only Kubernetes Operator Lifecycle Manager tools for OLMv0 and OLMv1 resources, catalogs, status, and diagnostics"
}

func (t *Toolset) GetTools(p api.FilteringProvider) []api.ServerTool {
	return GetTools(p)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt                     { return nil }
func (t *Toolset) GetResources() []api.ServerResource                 { return nil }
func (t *Toolset) GetResourceTemplates() []api.ServerResourceTemplate { return nil }

func init() { toolsets.Register(&Toolset{}) }

// hasAnyOLMAPI checks if any of the given GVKs are available
func hasAnyOLMAPI(p api.FilteringProvider, gvks ...schema.GroupVersionKind) func() bool {
	return func() bool {
		if p == nil || !p.IsTargetCompatibilityToolFiltersEnabled() {
			return true
		}
		for _, gvk := range gvks {
			if p.AnyTargetHasGVKs(context.TODO(), []schema.GroupVersionKind{gvk}) {
				return true
			}
		}
		return false
	}
}
