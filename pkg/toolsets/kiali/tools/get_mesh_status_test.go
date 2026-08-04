package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

type fakeProvider struct {
	hasGVKs bool
}

func (f *fakeProvider) AnyTargetHasGVKs(_ context.Context, _ []schema.GroupVersionKind) bool {
	return f.hasGVKs
}

func (f *fakeProvider) IsTargetCompatibilityToolFiltersEnabled() bool { return true }

type fakeProviderWithConfig struct {
	fakeProvider
	url string
}

func (f *fakeProviderWithConfig) GetProviderConfig(string) (api.ExtendedConfig, bool) {
	return nil, false
}

func (f *fakeProviderWithConfig) GetToolsetConfig(name string) (api.ExtendedConfig, bool) {
	if name != "kiali" || f.url == "" {
		return nil, false
	}
	return &kiali.Config{Url: f.url}, true
}

type MeshStatusToolSuite struct {
	suite.Suite
}

func (s *MeshStatusToolSuite) TestToolRegistration() {
	s.Run("tool is registered with TargetCompatibilityFilters", func() {
		tools := InitGetMeshStatus(&fakeProvider{hasGVKs: true})
		s.Require().Len(tools, 1, "Expected 1 mesh status tool")
		s.Equal(defaults.ToolsetName()+"_get_mesh_status", tools[0].Tool.Name)
		s.Equal("Get Mesh Status", tools[0].Tool.Annotations.Title)
		s.NotNil(tools[0].Tool.InputSchema)
		s.NotNil(tools[0].Handler)
		s.Require().Len(tools[0].TargetCompatibilityFilters, 1, "Expected 1 TargetCompatibilityFilter")
		s.True(tools[0].TargetCompatibilityFilters[0](), "Filter should return true when Kiali GVK is present")
	})

	s.Run("filter returns false when Kiali GVK is absent and URL is unset", func() {
		tools := InitGetMeshStatus(&fakeProvider{hasGVKs: false})
		s.Require().Len(tools, 1)
		s.Require().Len(tools[0].TargetCompatibilityFilters, 1)
		s.False(tools[0].TargetCompatibilityFilters[0](), "Filter should return false when Kiali GVK is absent")
	})

	s.Run("filter returns true when URL is configured without Kiali GVK", func() {
		tools := InitGetMeshStatus(&fakeProviderWithConfig{
			fakeProvider: fakeProvider{hasGVKs: false},
			url:          "https://kiali.example",
		})
		s.Require().Len(tools, 1)
		s.Require().Len(tools[0].TargetCompatibilityFilters, 1)
		s.True(tools[0].TargetCompatibilityFilters[0](), "Filter should return true when URL is configured")
	})
}

func TestMeshStatusToolSuite(t *testing.T) {
	suite.Run(t, new(MeshStatusToolSuite))
}
