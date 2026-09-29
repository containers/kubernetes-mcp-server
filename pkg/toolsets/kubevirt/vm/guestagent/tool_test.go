package guestagent

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/suite"
)

type fakeProvider struct{}

func (f *fakeProvider) Discovery() api.AggregateDiscovery       { return f }
func (f *fakeProvider) Unstructured() api.AggregateUnstructured { return nil }
func (f *fakeProvider) HasGVKs(ctx context.Context, _ []schema.GroupVersionKind) api.BoolResults {
	return api.BoolResults(api.NewResults(ctx, f, func(context.Context, string) (bool, error) {
		return true, nil
	}))
}
func (f *fakeProvider) ServerResourcesForGroupVersion(ctx context.Context, _ string) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, f, func(context.Context, string) (*metav1.APIResourceList, error) {
		return &metav1.APIResourceList{}, nil
	})
}
func (f *fakeProvider) IsMultiTarget() bool                          { return false }
func (f *fakeProvider) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }
func (f *fakeProvider) GetDefaultTarget() string                     { return "" }
func (f *fakeProvider) GetTargetParameterName() string               { return "" }

type GuestAgentToolSuite struct {
	suite.Suite
}

func (s *GuestAgentToolSuite) TestToolRegistration() {
	s.Run("tool is registered", func() {
		tools := Tools(s.T().Context(), &fakeProvider{})
		s.Require().Len(tools, 1, "Expected 1 guest agent tool")
		s.Equal("vm_guest_info", tools[0].Tool.Name)
		s.Equal("Virtual Machine: Guest Agent Info", tools[0].Tool.Annotations.Title)
		s.NotNil(tools[0].Tool.InputSchema)
		s.NotNil(tools[0].Handler)
		s.Require().Len(tools[0].TargetCompatibilityFilters, 1, "Expected 1 TargetCompatibilityFilter")
		s.True(tools[0].TargetCompatibilityFilters[0](), "Filter should return true with fakeProvider")
	})

	s.Run("tool has correct properties", func() {
		tools := Tools(s.T().Context(), &fakeProvider{})
		tool := tools[0].Tool

		// Check annotations
		s.True(*tool.Annotations.ReadOnlyHint, "guest info should be read-only")
		s.False(*tool.Annotations.DestructiveHint, "guest info should not be destructive")
		s.True(*tool.Annotations.IdempotentHint, "guest info should be idempotent")

		// Check schema
		schema := tool.InputSchema
		s.Require().NotNil(schema.Properties)
		s.Contains(schema.Properties, "namespace")
		s.Contains(schema.Properties, "name")
		s.Contains(schema.Properties, "info_type")

		// Check required fields
		s.ElementsMatch([]string{"namespace", "name"}, schema.Required)

		// Check info_type enum values
		infoTypeSchema := schema.Properties["info_type"]
		s.Require().NotNil(infoTypeSchema)
		s.ElementsMatch([]any{"all", "os", "filesystem", "users", "network"}, infoTypeSchema.Enum)
	})
}

func (s *GuestAgentToolSuite) TestInfoTypeConstants() {
	s.Run("info type constants are defined", func() {
		s.Equal(GuestAgentInfoType("all"), InfoTypeAll)
		s.Equal(GuestAgentInfoType("os"), InfoTypeOS)
		s.Equal(GuestAgentInfoType("filesystem"), InfoTypeFilesystem)
		s.Equal(GuestAgentInfoType("users"), InfoTypeUsers)
		s.Equal(GuestAgentInfoType("network"), InfoTypeNetwork)
	})
}

func TestGuestAgentToolSuite(t *testing.T) {
	suite.Run(t, new(GuestAgentToolSuite))
}
