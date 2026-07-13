package netobserv

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/internal/defaults"
	netobservTools "github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/tools"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return defaults.ToolsetName()
}

func (t *Toolset) GetDescription() string {
	return defaults.ToolsetDescription()
}

func (t *Toolset) GetTools(_ api.FilteringProvider) []api.ServerTool {
	return slices.Concat(
		netobservTools.InitListFlows(),
		netobservTools.InitGetFlowMetrics(),
		netobservTools.InitExportFlows(),
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
