package mcp

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// lintInspector answers discovery with every optional kind served, so the
// toolsets expose every tool they can.
type lintInspector struct{}

func (f *lintInspector) Discovery() api.AggregateDiscovery       { return f }
func (f *lintInspector) Unstructured() api.AggregateUnstructured { return nil }
func (f *lintInspector) ServerResourcesForGroupVersion(ctx context.Context, _ string) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, f, func(context.Context, string) (*metav1.APIResourceList, error) {
		return &metav1.APIResourceList{APIResources: []metav1.APIResource{
			{Kind: "Project"}, {Kind: "Route"}, {Kind: "NodeMetrics"}, {Kind: "PodMetrics"},
		}}, nil
	})
}

func (f *lintInspector) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }
func (f *lintInspector) GetDefaultTarget() string                     { return "" }
func (f *lintInspector) GetTargetParameterName() string               { return "" }
func (f *lintInspector) IsMultiTarget() bool                          { return false }

// TestBuiltInToolsFollowTheHouseStyle holds every registered built-in tool to
// docs/specs/tools-for-models.md, as the chart tests hold the dynamic ones.
func TestBuiltInToolsFollowTheHouseStyle(t *testing.T) {
	count := 0
	for _, toolset := range toolsets.Toolsets() {
		if toolset.GetName() == "dynamic" {
			continue
		}
		for _, tool := range toolset.GetTools(t.Context(), api.ToolsetContext{Inspector: &lintInspector{}}) {
			count++
			if lints := api.LintTool(tool.Tool); len(lints) > 0 {
				t.Errorf("tool %s (toolset %s) does not follow the house style: %v", tool.Tool.Name, toolset.GetName(), lints)
			}
		}
	}
	if count == 0 {
		t.Fatal("no built-in tools registered")
	}
}
