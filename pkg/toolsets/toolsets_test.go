package toolsets

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/stretchr/testify/suite"
)

type ToolsetsSuite struct {
	suite.Suite
}

func (s *ToolsetsSuite) SetupTest() {
	Clear()
}

type TestToolset struct {
	name        string
	description string
}

func (t *TestToolset) GetName() string { return t.name }

func (t *TestToolset) GetDescription() string { return t.description }

func (t *TestToolset) GetTools(k *kubernetes.Manager) []api.ServerTool { return nil }

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
}

func TestToolsets(t *testing.T) {
	suite.Run(t, new(ToolsetsSuite))
}
