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

var _ api.Toolset = (*TestToolset)(nil)

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
