package helm

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "helm"
}

func (t *Toolset) GetDescription() string {
	return "Tools for managing Helm charts and releases"
}

func (t *Toolset) GetTools(_ api.Openshift) []api.ServerTool {
	return slices.Concat(
		initHelm(),
	)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	// Helm toolset does not provide prompts
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
