package full

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

type Full struct{}

var _ api.Toolset = (*Full)(nil)

func (p *Full) GetName() string {
	return "full"
}

func (p *Full) GetDescription() string {
	return "Complete toolset with all tools and extended outputs"
}

func (p *Full) GetTools(k *internalk8s.Manager) []api.ServerTool {
	return slices.Concat(
		initConfiguration(),
		initEvents(),
		initNamespaces(k),
		initPods(),
		initResources(k),
		initHelm(),
	)
}

func init() {
	toolsets.Register(&Full{})
}
