package logs

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

const ToolsetName = "observability/logs"

// Toolset implements the observability toolset for Loki.
type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return ToolsetName
}

func (t *Toolset) GetDescription() string {
	return "Toolset for querying Loki logs"
}

func (t *Toolset) GetTools(_ api.Openshift) []api.ServerTool {
	return []api.ServerTool{
		initListInstances(),
		initLabelNames(),
		initLabelValues(),
		initQueryRange(),
	}
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
