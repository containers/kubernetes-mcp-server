package metrics

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// Toolset implements the observability toolset for advanced Prometheus monitoring.
type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

// GetName returns the name of the toolset.
func (t *Toolset) GetName() string {
	return ToolsetName
}

// GetDescription returns a human-readable description of the toolset.
func (t *Toolset) GetDescription() string {
	return "Toolset for querying Prometheus and Alertmanager endpoints in efficient ways."
}

// GetTools returns all tools provided by this toolset.
func (t *Toolset) GetTools(_ context.Context, _ api.ToolsetContext) []api.ServerTool {
	return []api.ServerTool{
		initListMetrics(),
		initExecuteInstantQuery(),
		initExecuteRangeQuery(),
		initShowTimeseries(),
		initGetLabelNames(),
		initGetLabelValues(),
		initGetSeries(),
		initGetAlerts(),
		initGetSilences(),
	}
}

// GetPrompts returns prompts provided by this toolset.
func (t *Toolset) GetPrompts(_ context.Context, _ api.ToolsetContext) []api.ServerPrompt {
	// Currently, prompts are not supported through this toolset
	// The workflow instructions are embedded in the tool descriptions
	return nil
}

// GetResources returns resources provided by this toolset.
func (t *Toolset) GetResources(_ context.Context, _ api.ToolsetContext) []api.ServerResource {
	return nil
}

// GetResourceTemplates returns resource templates provided by this toolset.
func (t *Toolset) GetResourceTemplates(_ context.Context, _ api.ToolsetContext) []api.ServerResourceTemplate {
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
