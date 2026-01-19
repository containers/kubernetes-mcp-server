package kcp

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "kcp"
}

func (t *Toolset) GetDescription() string {
	return "Manage kcp workspaces and multi-tenancy features"
}

func (t *Toolset) GetTools(_ api.Openshift) []api.ServerTool {
	return slices.Concat(
		initWorkspaceTools(),
	)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
