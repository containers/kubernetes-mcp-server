package toolsets

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ToolsetsSuite struct {
	suite.Suite
	originalToolsets []api.Toolset
}

func (s *ToolsetsSuite) SetupTest() {
	s.originalToolsets = Toolsets()
	Clear()
}

func (s *ToolsetsSuite) TearDownTest() {
	Clear()
	for _, toolset := range s.originalToolsets {
		Register(toolset)
	}
}

type TestToolset struct {
	name        string
	description string
}

func (t *TestToolset) GetName() string { return t.name }

func (t *TestToolset) GetDescription() string { return t.description }

func (t *TestToolset) GetTools(context.Context, api.ToolsetContext) []api.ServerTool { return nil }

func (t *TestToolset) GetPrompts(context.Context, api.ToolsetContext) []api.ServerPrompt {
	return nil
}

func (t *TestToolset) GetResources(context.Context, api.ToolsetContext) []api.ServerResource {
	return nil
}

func (t *TestToolset) GetResourceTemplates(context.Context, api.ToolsetContext) []api.ServerResourceTemplate {
	return nil
}

var _ api.Toolset = (*TestToolset)(nil)

type fakeProvider struct{}

func (f *fakeProvider) IsMultiTarget() bool { return false }

func (f *fakeProvider) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }

func (f *fakeProvider) GetDefaultTarget() string { return "" }

func (f *fakeProvider) GetTargetParameterName() string { return "" }

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

func (s *ToolsetsSuite) TestRegisterPanicsOnDuplicate() {
	Register(&TestToolset{name: "duplicate"})
	s.Panics(func() {
		Register(&TestToolset{name: "duplicate"})
	}, "Expected panic on duplicate toolset registration")
}

func (s *ToolsetsSuite) TestUniqueToolNames() {
	toolNames := make(map[string]bool)
	for _, toolset := range s.originalToolsets {
		for _, tool := range toolset.GetTools(s.T().Context(), api.ToolsetContext{Inspector: &fakeProvider{}}) {
			s.Falsef(toolNames[tool.Tool.Name], "duplicate tool name: %s", tool.Tool.Name)
			toolNames[tool.Tool.Name] = true
		}
	}
}

func (s *ToolsetsSuite) TestUniquePromptNames() {
	promptNames := make(map[string]bool)
	for _, toolset := range s.originalToolsets {
		for _, prompt := range toolset.GetPrompts(s.T().Context(), api.ToolsetContext{Inspector: &fakeProvider{}}) {
			s.Falsef(promptNames[prompt.Prompt.Name], "duplicate prompt name: %s", prompt.Prompt.Name)
			promptNames[prompt.Prompt.Name] = true
		}
	}
}

func (s *ToolsetsSuite) TestUniqueResourceURIs() {
	resourceURIs := make(map[string]bool)
	for _, toolset := range s.originalToolsets {
		for _, resource := range toolset.GetResources(s.T().Context(), api.ToolsetContext{Inspector: &fakeProvider{}}) {
			s.Falsef(resourceURIs[resource.Resource.URI], "duplicate resource URI: %s", resource.Resource.URI)
			resourceURIs[resource.Resource.URI] = true
		}
	}
}

func (s *ToolsetsSuite) TestUniqueResourceTemplateURITemplates() {
	uriTemplates := make(map[string]bool)
	for _, toolset := range s.originalToolsets {
		for _, template := range toolset.GetResourceTemplates(s.T().Context(), api.ToolsetContext{Inspector: &fakeProvider{}}) {
			s.Falsef(uriTemplates[template.ResourceTemplate.URITemplate], "duplicate resource template URI template: %s", template.ResourceTemplate.URITemplate)
			uriTemplates[template.ResourceTemplate.URITemplate] = true
		}
	}
}

func (s *ToolsetsSuite) TestToolsetNames() {
	s.Run("Returns empty list if no toolsets registered", func() {
		s.Empty(ToolsetNames(), "Expected empty list of toolset names")
	})

	Register(&TestToolset{name: "z"})
	Register(&TestToolset{name: "b"})
	Register(&TestToolset{name: "1"})
	s.Run("Returns sorted list of registered toolset names", func() {
		names := ToolsetNames()
		s.Equal([]string{"1", "b", "z"}, names, "Expected sorted list of toolset names")
	})
}

func (s *ToolsetsSuite) TestToolsetFromString() {
	s.Run("Returns nil if toolset not found", func() {
		s.Nil(ToolsetFromString("non-existent"), "Expected nil for non-existent toolset")
	})
	s.Run("Returns the correct toolset if found", func() {
		Register(&TestToolset{name: "existent"})
		res := ToolsetFromString("existent")
		s.NotNil(res, "Expected to find the registered toolset")
		s.Equal("existent", res.GetName(), "Expected to find the registered toolset by name")
	})
	s.Run("Returns the correct toolset if found after trimming spaces", func() {
		Register(&TestToolset{name: "no-spaces"})
		res := ToolsetFromString("  no-spaces  ")
		s.NotNil(res, "Expected to find the registered toolset")
		s.Equal("no-spaces", res.GetName(), "Expected to find the registered toolset by name")
	})
}

func (s *ToolsetsSuite) TestValidate() {
	s.Run("Returns nil for empty toolset list", func() {
		s.Nil(Validate([]string{}), "Expected nil for empty toolset list")
	})
	s.Run("Returns error for invalid toolset name", func() {
		err := Validate([]string{"invalid"})
		s.NotNil(err, "Expected error for invalid toolset name")
		s.Contains(err.Error(), "invalid toolset name: invalid", "Expected error message to contain invalid toolset name")
	})
	s.Run("Returns nil for valid toolset names", func() {
		Register(&TestToolset{name: "valid-1"})
		Register(&TestToolset{name: "valid-2"})
		err := Validate([]string{"valid-1", "valid-2"})
		s.Nil(err, "Expected nil for valid toolset names")
	})
	s.Run("Returns error if any toolset name is invalid", func() {
		Register(&TestToolset{name: "valid"})
		err := Validate([]string{"valid", "invalid"})
		s.NotNil(err, "Expected error if any toolset name is invalid")
		s.Contains(err.Error(), "invalid toolset name: invalid", "Expected error message to contain invalid toolset name")
	})
	s.Run("Reports every invalid toolset name", func() {
		err := Validate([]string{"bogus-a", "valid", "bogus-b"})
		s.Require().Error(err)
		s.Contains(err.Error(), "bogus-a")
		s.Contains(err.Error(), "bogus-b")
		s.NotContains(err.Error(), "bogus-a, valid")
	})
}

func TestToolsets(t *testing.T) {
	suite.Run(t, new(ToolsetsSuite))
}
