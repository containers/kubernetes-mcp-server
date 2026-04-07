package tekton

import (
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
	return "Tekton pipeline management tools for Pipelines, PipelineRuns, Tasks, and TaskRuns."
}

func (t *Toolset) GetTools(_ api.Openshift) []api.ServerTool {
	return slices.Concat(
		pipelineTools(),
		pipelineRunTools(),
		taskTools(),
		taskRunTools(),
	)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
