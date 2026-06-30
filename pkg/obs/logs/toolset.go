package logs

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"

	"github.com/rhobs/obs-mcp/pkg/logs/loki"
)

const ToolsetName = "observability/logs"

// Toolset implements the observability toolset for Loki.
type Toolset struct {
	NewLokiLoader func(params api.ToolHandlerParams, url, tenant string) (loki.Loader, error)
}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return ToolsetName
}

func (t *Toolset) GetDescription() string {
	return "Toolset for querying Loki logs"
}

func (t *Toolset) GetTools(_ api.Openshift) []api.ServerTool {
	return []api.ServerTool{
		ListInstancesTool.ToServerTool(ToServerHandler(t.NewLokiLoader, ListInstancesHandler)),
		LabelNamesTool.ToServerTool(ToServerHandler(t.NewLokiLoader, LabelNamesHandler)),
		LabelValuesTool.ToServerTool(ToServerHandler(t.NewLokiLoader, LabelValuesHandler)),
		QueryRangeTool.ToServerTool(ToServerHandler(t.NewLokiLoader, QueryRangeHandler)),
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
