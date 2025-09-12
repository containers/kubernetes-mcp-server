package core

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
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

func (t *Toolset) GetTools(o internalk8s.Openshift) []api.ServerTool {
	return slices.Concat(
		initEvents(),
		initNamespaces(o),
		initPods(),
		initResources(o),
	)
}

func init() {
	toolsets.Register(&Toolset{})
}
