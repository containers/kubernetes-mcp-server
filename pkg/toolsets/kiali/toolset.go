package kiali

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "kiali"
}

func (t *Toolset) GetDescription() string {
	return "Most common tools for managing Kiali, check the [Kiali documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/KIALI.md) for more details."
}

func (t *Toolset) GetTools(_ internalk8s.Openshift) []api.ServerTool {
	return slices.Concat(
		initGetMeshGraph(),
		initManageIstioConfig(),
		initGetResourceDetails(),
		initGetMetrics(),
		initLogs(),
		initGetTraces(),
	)
}

func (t *Toolset) GetPrompts(_ internalk8s.Openshift) []api.ServerPrompt {
	// Kiali toolset does not provide prompts
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
