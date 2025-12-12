package kiali

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
	kialiTools "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return defaults.ToolsetName()
}

func (t *Toolset) GetDescription() string {
	return defaults.ToolsetDescription()
}

func (t *Toolset) GetTools(_ api.Openshift) []api.ServerTool {
	return slices.Concat(
		kialiTools.InitGetMeshGraph(),
		kialiTools.InitManageIstioConfig(),
		kialiTools.InitGetResourceDetails(),
		kialiTools.InitGetMetrics(),
		kialiTools.InitLogs(),
		kialiTools.InitGetTraces(),
	)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	// Kiali toolset does not provide prompts
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
