package config

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "config"
}

func (t *Toolset) GetDescription() string {
	return "View and manage the current local Kubernetes configuration (kubeconfig)"
}

func (t *Toolset) GetTools(_ api.Openshift) []api.ServerTool {
	return slices.Concat(
		initConfiguration(),
	)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	// Config toolset does not provide prompts
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
