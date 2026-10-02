package tekton_test

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/tekton"
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

func (s *TektonSuite) TestOnlyDiagnosisRequiresPipelineRunGVK() {
	provider := &filteringProvider{available: false}
	tools := (&tekton.Toolset{}).GetTools(provider)
	s.Require().NotEmpty(tools)

	foundDiagnosis := false
	for _, tool := range tools {
		if tool.Tool.Name != "tekton_pipelinerun_diagnose" {
			s.Empty(tool.TargetCompatibilityFilters, tool.Tool.Name)
			continue
		}
		foundDiagnosis = true
		s.Require().Len(tool.TargetCompatibilityFilters, 1)
		s.False(tool.TargetCompatibilityFilters[0]())
	}
	s.True(foundDiagnosis)
	s.Equal([]schema.GroupVersionKind{{Group: "tekton.dev", Version: "v1", Kind: "PipelineRun"}}, provider.requested)
}
