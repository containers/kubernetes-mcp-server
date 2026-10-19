package tekton

import (
	"context"
	"slices"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type compatibilityInspector struct {
	resources map[string][]string
}

func (i *compatibilityInspector) Discovery() api.AggregateDiscovery       { return i }
func (i *compatibilityInspector) Unstructured() api.AggregateUnstructured { return nil }
func (i *compatibilityInspector) BearerToken(ctx context.Context) api.Results[string] {
	return api.NewResults(ctx, i, func(context.Context, string) (string, error) { return "", nil })
}
func (i *compatibilityInspector) ServerResourcesForGroupVersion(ctx context.Context, _ string) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, i, func(_ context.Context, target string) (*metav1.APIResourceList, error) {
		kinds := i.resources[target]
		list := &metav1.APIResourceList{GroupVersion: "tekton.dev/v1"}
		for _, kind := range kinds {
			list.APIResources = append(list.APIResources, metav1.APIResource{Kind: kind})
		}
		return list, nil
	})
}
func (i *compatibilityInspector) GetTargets(context.Context) ([]string, error) {
	targets := make([]string, 0, len(i.resources))
	for target := range i.resources {
		targets = append(targets, target)
	}
	slices.Sort(targets)
	return targets, nil
}
func (i *compatibilityInspector) GetDefaultTarget() string       { return "" }
func (i *compatibilityInspector) GetTargetParameterName() string { return "" }
func (i *compatibilityInspector) IsMultiTarget() bool            { return len(i.resources) > 1 }

type CompatibilitySuite struct {
	suite.Suite
}

func (s *CompatibilitySuite) TestPipelineRunLogsRequiresTaskRunOnSameTarget() {
	inspector := &compatibilityInspector{resources: map[string][]string{
		"first":  {pipelineRunKind},
		"second": {taskRunKind},
	}}

	tool := findTektonTool(pipelineRunTools(s.T().Context(), inspector), "tekton_pipelinerun_logs")

	s.Require().NotNil(tool)
	s.Require().Len(tool.TargetCompatibilityFilters, 1)
	s.False(tool.TargetCompatibilityFilters[0]())
}

func (s *CompatibilitySuite) TestPipelineRunToolsWithOnlyPipelineRun() {
	inspector := &compatibilityInspector{resources: map[string][]string{
		"default": {pipelineRunKind},
	}}
	tools := pipelineRunTools(s.T().Context(), inspector)

	lifecycle := findTektonTool(tools, "tekton_pipelinerun_lifecycle")
	logs := findTektonTool(tools, "tekton_pipelinerun_logs")

	s.Require().NotNil(lifecycle)
	s.Require().NotNil(logs)
	s.True(lifecycle.TargetCompatibilityFilters[0]())
	s.False(logs.TargetCompatibilityFilters[0]())
}

func (s *CompatibilitySuite) TestStartToolsRequireDefinitionAndRunKinds() {
	inspector := &compatibilityInspector{resources: map[string][]string{
		"default": {pipelineKind, pipelineRunKind, taskKind, taskRunKind},
	}}

	pipelineStart := findTektonTool(pipelineTools(s.T().Context(), inspector), "tekton_pipeline_start")
	taskStart := findTektonTool(taskTools(s.T().Context(), inspector), "tekton_task_start")

	s.Require().NotNil(pipelineStart)
	s.Require().NotNil(taskStart)
	s.True(pipelineStart.TargetCompatibilityFilters[0]())
	s.True(taskStart.TargetCompatibilityFilters[0]())
}

func findTektonTool(tools []api.ServerTool, name string) *api.ServerTool {
	for i := range tools {
		if tools[i].Tool.Name == name {
			return &tools[i]
		}
	}
	return nil
}

func TestCompatibility(t *testing.T) {
	suite.Run(t, new(CompatibilitySuite))
}
