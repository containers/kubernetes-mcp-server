package logs

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
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

func (t *Toolset) GetTools(_ context.Context, toolsetContext api.ToolsetContext) []api.ServerTool {
	return []api.ServerTool{
		initListInstances(toolsetContext.Inspector),
		initLabelNames(toolsetContext.Inspector),
		initLabelValues(toolsetContext.Inspector),
		initQueryRange(toolsetContext.Inspector),
	}
}

func (t *Toolset) GetPrompts(_ context.Context, _ api.ToolsetContext) []api.ServerPrompt {
	return nil
}

func (t *Toolset) GetResources(_ context.Context, _ api.ToolsetContext) []api.ServerResource {
	return nil
}

func (t *Toolset) GetResourceTemplates(_ context.Context, _ api.ToolsetContext) []api.ServerResourceTemplate {
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
