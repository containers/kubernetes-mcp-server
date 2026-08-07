package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type fakeFilteringProvider struct {
	hasGVKs bool
}

func (f *fakeFilteringProvider) AnyTargetHasGVKs(_ context.Context, _ []schema.GroupVersionKind) bool {
	return f.hasGVKs
}

func (f *fakeFilteringProvider) IsTargetCompatibilityToolFiltersEnabled() bool { return true }

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
		tool := s.findTool(initNodes(&fakeFilteringProvider{hasGVKs: true}), "nodes_top")
		s.Require().NotNil(tool, "expected nodes_top tool")
		s.Require().Len(tool.TargetCompatibilityFilters, 1, "Expected 1 TargetCompatibilityFilter")
		s.True(tool.TargetCompatibilityFilters[0](), "Filter should return true when metrics GVK is available")
	})

	s.Run("nodes_top filter returns false without metrics GVK", func() {
		tool := s.findTool(initNodes(&fakeFilteringProvider{hasGVKs: false}), "nodes_top")
		s.Require().NotNil(tool, "expected nodes_top tool")
		s.Require().Len(tool.TargetCompatibilityFilters, 1)
		s.False(tool.TargetCompatibilityFilters[0](), "Filter should return false when metrics GVK is unavailable")
	})
}

func (s *MetricsToolsSuite) TestPodsTopRegistration() {
	s.Run("pods_top has TargetCompatibilityFilter", func() {
		tool := s.findTool(initPods(&fakeFilteringProvider{hasGVKs: true}), "pods_top")
		s.Require().NotNil(tool, "expected pods_top tool")
		s.Require().Len(tool.TargetCompatibilityFilters, 1, "Expected 1 TargetCompatibilityFilter")
		s.True(tool.TargetCompatibilityFilters[0](), "Filter should return true when metrics GVK is available")
	})

	s.Run("pods_top filter returns false without metrics GVK", func() {
		tool := s.findTool(initPods(&fakeFilteringProvider{hasGVKs: false}), "pods_top")
		s.Require().NotNil(tool, "expected pods_top tool")
		s.Require().Len(tool.TargetCompatibilityFilters, 1)
		s.False(tool.TargetCompatibilityFilters[0](), "Filter should return false when metrics GVK is unavailable")
	})
}

func TestMetricsTools(t *testing.T) {
	suite.Run(t, new(MetricsToolsSuite))
}
