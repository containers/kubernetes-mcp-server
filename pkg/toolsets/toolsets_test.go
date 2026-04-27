package toolsets

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
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

func (t *TestToolset) GetTools(_ api.Openshift) []api.ServerTool { return nil }

func (t *TestToolset) GetPrompts() []api.ServerPrompt { return nil }

func (t *TestToolset) GetResources() []api.ServerResource { return nil }

func (t *TestToolset) GetResourceTemplates() []api.ServerResourceTemplate { return nil }

var _ api.Toolset = (*TestToolset)(nil)

func (s *ToolsetsSuite) TestRegisterPanicsOnDuplicate() {
	Register(&TestToolset{name: "duplicate"})
	s.Panics(func() {
		Register(&TestToolset{name: "duplicate"})
	}, "Expected panic on duplicate toolset registration")
}

func (s *ToolsetsSuite) TestUniqueToolNames() {
	toolNames := make(map[string]bool)
	for _, toolset := range Toolsets() {
		for _, tool := range toolset.GetTools(nil) {
			s.Falsef(toolNames[tool.Tool.Name], "duplicate tool name: %s", tool.Tool.Name)
			toolNames[tool.Tool.Name] = true
		}
	}
}

func (s *ToolsetsSuite) TestUniquePromptNames() {
	promptNames := make(map[string]bool)
	for _, toolset := range Toolsets() {
		for _, prompt := range toolset.GetPrompts() {
			s.Falsef(promptNames[prompt.Prompt.Name], "duplicate prompt name: %s", prompt.Prompt.Name)
			promptNames[prompt.Prompt.Name] = true
		}
	}
}

func (s *ToolsetsSuite) TestUniqueResourceURIs() {
	resourceURIs := make(map[string]bool)
	for _, toolset := range Toolsets() {
		for _, resource := range toolset.GetResources() {
			s.Falsef(resourceURIs[resource.Resource.URI], "duplicate resource URI: %s", resource.Resource.URI)
			resourceURIs[resource.Resource.URI] = true
		}
	}
}

func (s *ToolsetsSuite) TestUniqueResourceTemplateURITemplates() {
	uriTemplates := make(map[string]bool)
	for _, toolset := range Toolsets() {
		for _, template := range toolset.GetResourceTemplates() {
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
}

func TestToolsets(t *testing.T) {
	suite.Run(t, new(ToolsetsSuite))
}
