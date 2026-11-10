package tekton

import (
	"context"
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// Toolset provides Tekton pipeline management tools.
type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "tekton"
}

func (t *Toolset) GetDescription() string {
	return "Tekton pipeline management tools for Pipelines, PipelineRuns, Tasks, TaskRuns, and troubleshooting."
}

func (t *Toolset) GetTools(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerTool {
	return slices.Concat(
		pipelineTools(),
		pipelineRunTools(ctx, toolsetContext.Inspector),
		taskTools(),
		taskRunTools(),
	)
}

func (t *Toolset) GetPrompts(_ context.Context, _ api.ToolsetContext) []api.ServerPrompt {
	return pipelineTroubleshootPrompts()
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
