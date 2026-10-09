package tekton_test

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/tekton"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type filteringProvider struct {
	available bool
	requested []string
}

func (p *filteringProvider) Discovery() api.AggregateDiscovery       { return p }
func (p *filteringProvider) Unstructured() api.AggregateUnstructured { return nil }
func (p *filteringProvider) ServerResourcesForGroupVersion(ctx context.Context, groupVersion string) api.Results[*metav1.APIResourceList] {
	p.requested = append(p.requested, groupVersion)
	return api.NewResults(ctx, p, func(context.Context, string) (*metav1.APIResourceList, error) {
		list := &metav1.APIResourceList{}
		if p.available {
			list.APIResources = []metav1.APIResource{{Kind: "PipelineRun"}}
		}
		return list, nil
	})
}
func (p *filteringProvider) IsMultiTarget() bool                          { return false }
func (p *filteringProvider) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }
func (p *filteringProvider) GetDefaultTarget() string                     { return "" }
func (p *filteringProvider) GetTargetParameterName() string               { return "" }

func (s *TektonSuite) TestOnlyDiagnosisRequiresPipelineRunGVK() {
	provider := &filteringProvider{available: false}
	tools := (&tekton.Toolset{}).GetTools(s.T().Context(), api.ToolsetContext{Inspector: provider})
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
		s.Require().NotNil(tool.Tool.Annotations.OpenWorldHint)
		s.True(*tool.Tool.Annotations.OpenWorldHint)
	}
	s.True(foundDiagnosis)
	s.Equal([]string{"tekton.dev/v1"}, provider.requested)
}
