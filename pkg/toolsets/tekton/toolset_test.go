package tekton_test

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/tekton"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type filteringProvider struct {
	available bool
	requested []schema.GroupVersionKind
}

func (p *filteringProvider) IsTargetCompatibilityToolFiltersEnabled() bool {
	return true
}

func (p *filteringProvider) AnyTargetHasGVKs(_ context.Context, requested []schema.GroupVersionKind) bool {
	p.requested = requested
	return p.available
}

func TestToolsRequirePipelineRunGVK(t *testing.T) {
	provider := &filteringProvider{available: true}
	tools := (&tekton.Toolset{}).GetTools(provider)
	require.NotEmpty(t, tools)

	for _, tool := range tools {
		require.NotEmpty(t, tool.TargetCompatibilityFilters, tool.Tool.Name)
		require.True(t, tool.TargetCompatibilityFilters[len(tool.TargetCompatibilityFilters)-1](), tool.Tool.Name)
	}
	require.Equal(t, []schema.GroupVersionKind{{Group: "tekton.dev", Version: "v1", Kind: "PipelineRun"}}, provider.requested)
}
