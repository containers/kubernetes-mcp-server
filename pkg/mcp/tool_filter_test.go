package mcp

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

type ToolFilterSuite struct {
	suite.Suite
}

func (s *ToolFilterSuite) TestToolFilterType() {
	s.Run("ToolFilter type can be used as function", func() {
		var mutator ToolFilter = func(tool api.ServerTool) bool {
			return tool.Tool.Name == "included"
		}
		s.Run("returns true for included tool", func() {
			tool := api.ServerTool{Tool: api.Tool{Name: "included"}}
			s.True(mutator(tool))
		})
		s.Run("returns false for excluded tool", func() {
			tool := api.ServerTool{Tool: api.Tool{Name: "excluded"}}
			s.False(mutator(tool))
		})
	})
}

func (s *ToolFilterSuite) TestCompositeFilter() {
	s.Run("returns true if all filters return true", func() {
		filter := CompositeFilter(
			func(tool api.ServerTool) bool { return true },
			func(tool api.ServerTool) bool { return true },
		)
		tool := api.ServerTool{Tool: api.Tool{Name: "test"}}
		s.True(filter(tool))
	})
	s.Run("returns false if any filter returns false", func() {
		filter := CompositeFilter(
			func(tool api.ServerTool) bool { return true },
			func(tool api.ServerTool) bool { return false },
		)
		tool := api.ServerTool{Tool: api.Tool{Name: "test"}}
		s.False(filter(tool))
	})
}

func (s *ToolFilterSuite) TestShouldIncludeTargetListTool() {
	s.Run("non-target-list-provider tools: returns true ", func() {
		filter := ShouldIncludeTargetListTool("any", []string{"a", "b", "c", "d", "e", "f"})
		tool := api.ServerTool{Tool: api.Tool{Name: "test"}, TargetListProvider: ptr.To(false)}
		s.True(filter(tool))
	})
	s.Run("target-list-provider tools", func() {
		s.Run("with targets == 1: returns false", func() {
			filter := ShouldIncludeTargetListTool("any", []string{"1"})
			tool := api.ServerTool{Tool: api.Tool{Name: "test"}, TargetListProvider: ptr.To(true)}
			s.False(filter(tool))
		})
		s.Run("with targets == 1", func() {
			s.Run("and tool is configuration_contexts_list and targetName is not context: returns false", func() {
				filter := ShouldIncludeTargetListTool("not_context", []string{"1"})
				tool := api.ServerTool{Tool: api.Tool{Name: "configuration_contexts_list"}, TargetListProvider: ptr.To(true)}
				s.False(filter(tool))
			})
			s.Run("and tool is configuration_contexts_list and targetName is context: returns false", func() {
				filter := ShouldIncludeTargetListTool("context", []string{"1"})
				tool := api.ServerTool{Tool: api.Tool{Name: "configuration_contexts_list"}, TargetListProvider: ptr.To(true)}
				s.False(filter(tool))
			})
			s.Run("and tool is not configuration_contexts_list: returns false", func() {
				filter := ShouldIncludeTargetListTool("any", []string{"1"})
				tool := api.ServerTool{Tool: api.Tool{Name: "other_tool"}, TargetListProvider: ptr.To(true)}
				s.False(filter(tool))
			})
		})
	})
}

func TestToolFilter(t *testing.T) {
	suite.Run(t, new(ToolFilterSuite))
}
