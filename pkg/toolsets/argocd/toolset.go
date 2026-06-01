package argocd

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// Toolset provides ArgoCD read-only tools.
type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "argocd"
}

func (t *Toolset) GetDescription() string {
	return "ArgoCD tools for listing and inspecting Applications, AppProjects, and ArgoCD instances."
}

func (t *Toolset) GetTools(_ api.Openshift) []api.ServerTool {
	return slices.Concat(
		applicationTools(),
		appProjectTools(),
		instanceTools(),
	)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	return nil
}

func (t *Toolset) GetResources() []api.ServerResource {
	return nil
}

func (t *Toolset) GetResourceTemplates() []api.ServerResourceTemplate {
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
