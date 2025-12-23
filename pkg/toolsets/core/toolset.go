package core

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "core"
}

func (t *Toolset) GetDescription() string {
	return "Most common tools for Kubernetes management (Pods, Generic Resources, Events, etc.)"
}

func (t *Toolset) GetTools(o api.Openshift) []api.ServerTool {
	return slices.Concat(
		initEvents(),
		initNamespaces(o),
		initNodes(),
		initPods(),
		initResources(o),
	)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	return slices.Concat(
		initHealthChecks(),
	)
}

func init() {
	toolsets.Register(&Toolset{})
}
