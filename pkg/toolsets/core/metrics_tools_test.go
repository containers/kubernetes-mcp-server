package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type fakeInspector struct {
	hasGVKs bool
}

func (f *fakeInspector) Discovery() api.AggregateDiscovery       { return f }
func (f *fakeInspector) Unstructured() api.AggregateUnstructured { return nil }
func (f *fakeInspector) HasGVKs(ctx context.Context, _ []schema.GroupVersionKind) api.BoolResults {
	return api.BoolResults(api.NewResults(ctx, f, func(context.Context, string) (bool, error) {
		return f.hasGVKs, nil
	}))
}
func (f *fakeInspector) ServerResourcesForGroupVersion(ctx context.Context, _ string) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, f, func(context.Context, string) (*metav1.APIResourceList, error) {
		return &metav1.APIResourceList{}, nil
	})
}
func (f *fakeInspector) IsMultiTarget() bool                          { return false }
func (f *fakeInspector) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }
func (f *fakeInspector) GetDefaultTarget() string                     { return "" }
func (f *fakeInspector) GetTargetParameterName() string               { return "" }

type MetricsToolsSuite struct {
	suite.Suite
}

func (s *MetricsToolsSuite) findTool(tools []api.ServerTool, name string) *api.ServerTool {
	for i := range tools {
		if tools[i].Tool.Name == name {
			return &tools[i]
		}
	}
	return nil
}

func (s *MetricsToolsSuite) TestNodesTopRegistration() {
	s.Run("nodes_top has TargetCompatibilityFilter", func() {
		tool := s.findTool(initNodes(s.T().Context(), &fakeInspector{hasGVKs: true}), "nodes_top")
		s.Require().NotNil(tool, "expected nodes_top tool")
		s.Require().Len(tool.TargetCompatibilityFilters, 1, "Expected 1 TargetCompatibilityFilter")
		s.True(tool.TargetCompatibilityFilters[0](), "Filter should return true when metrics GVK is available")
	})

	s.Run("nodes_top filter returns false without metrics GVK", func() {
		tool := s.findTool(initNodes(s.T().Context(), &fakeInspector{hasGVKs: false}), "nodes_top")
		s.Require().NotNil(tool, "expected nodes_top tool")
		s.Require().Len(tool.TargetCompatibilityFilters, 1)
		s.False(tool.TargetCompatibilityFilters[0](), "Filter should return false when metrics GVK is unavailable")
	})
}

func (s *MetricsToolsSuite) TestPodsTopRegistration() {
	s.Run("pods_top has TargetCompatibilityFilter", func() {
		tool := s.findTool(initPods(s.T().Context(), &fakeInspector{hasGVKs: true}), "pods_top")
		s.Require().NotNil(tool, "expected pods_top tool")
		s.Require().Len(tool.TargetCompatibilityFilters, 1, "Expected 1 TargetCompatibilityFilter")
		s.True(tool.TargetCompatibilityFilters[0](), "Filter should return true when metrics GVK is available")
	})

	s.Run("pods_top filter returns false without metrics GVK", func() {
		tool := s.findTool(initPods(s.T().Context(), &fakeInspector{hasGVKs: false}), "pods_top")
		s.Require().NotNil(tool, "expected pods_top tool")
		s.Require().Len(tool.TargetCompatibilityFilters, 1)
		s.False(tool.TargetCompatibilityFilters[0](), "Filter should return false when metrics GVK is unavailable")
	})
}

func TestMetricsTools(t *testing.T) {
	suite.Run(t, new(MetricsToolsSuite))
}
