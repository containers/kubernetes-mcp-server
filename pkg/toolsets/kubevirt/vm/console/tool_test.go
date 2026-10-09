package console

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

type fakeProvider struct{}

func (f *fakeProvider) Discovery() api.AggregateDiscovery         { return f }
func (f *fakeProvider) Unstructured() api.AggregateUnstructured   { return nil }
func (f *fakeProvider) DefaultBearerToken(context.Context) string { return "" }
func (f *fakeProvider) ServerResourcesForGroupVersion(ctx context.Context, _ string) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, f, func(context.Context, string) (*metav1.APIResourceList, error) {
		return &metav1.APIResourceList{APIResources: []metav1.APIResource{{Kind: "VirtualMachine"}}}, nil
	})
}
func (f *fakeProvider) IsMultiTarget() bool                          { return false }
func (f *fakeProvider) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }
func (f *fakeProvider) GetDefaultTarget() string                     { return "" }
func (f *fakeProvider) GetTargetParameterName() string               { return "" }

type ConsoleToolSuite struct {
	suite.Suite
}

func (s *ConsoleToolSuite) TestToolRegistration() {
	s.Run("tool is registered", func() {
		tools := Tools(s.T().Context(), &fakeProvider{})
		s.Require().Len(tools, 1, "Expected 1 console tool")
		s.Equal("vm_console_screenshot", tools[0].Tool.Name)
		s.Equal("Virtual Machine: Console Screenshot", tools[0].Tool.Annotations.Title)
		s.NotEmpty(tools[0].Tool.Description)
		s.NotNil(tools[0].Tool.InputSchema)
		s.NotNil(tools[0].Handler)
		s.Require().Len(tools[0].TargetCompatibilityFilters, 1, "Expected 1 TargetCompatibilityFilter")
		s.True(tools[0].TargetCompatibilityFilters[0](), "Filter should return true with fakeProvider")
	})

	s.Run("tool has correct properties", func() {
		tool := Tools(s.T().Context(), &fakeProvider{})[0].Tool

		// Check annotations
		s.True(ptr.Deref(tool.Annotations.ReadOnlyHint, false), "screenshot should be read-only")
		s.False(ptr.Deref(tool.Annotations.DestructiveHint, true), "screenshot should not be destructive")
		s.True(ptr.Deref(tool.Annotations.IdempotentHint, false), "screenshot should be idempotent")

		// Check schema
		schema := tool.InputSchema
		s.Require().NotNil(schema.Properties)
		s.Contains(schema.Properties, "namespace")
		s.Contains(schema.Properties, "name")
		s.Len(schema.Properties, 2, "the tool takes no parameters beyond namespace and name")

		// Check required fields
		s.ElementsMatch([]string{"namespace", "name"}, schema.Required)
	})
}

func TestConsoleToolSuite(t *testing.T) {
	suite.Run(t, new(ConsoleToolSuite))
}
