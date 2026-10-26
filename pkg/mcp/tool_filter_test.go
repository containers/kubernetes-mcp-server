package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
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
	s.Run("non-target-list-provider tools: returns true", func() {
		filter := ShouldIncludeTargetListTool("any", true)
		tool := api.ServerTool{Tool: api.Tool{Name: "test"}, TargetListProvider: ptr.To(false)}
		s.True(filter(tool))
	})
	s.Run("target-list-provider tools", func() {
		s.Run("with single cluster: returns false", func() {
			filter := ShouldIncludeTargetListTool("any", false)
			tool := api.ServerTool{Tool: api.Tool{Name: "test"}, TargetListProvider: ptr.To(true)}
			s.False(filter(tool))
		})
		s.Run("with multiple clusters", func() {
			s.Run("and tool is configuration_contexts_list and targetName is not context: returns false", func() {
				filter := ShouldIncludeTargetListTool("not_context", true)
				tool := api.ServerTool{Tool: api.Tool{Name: "configuration_contexts_list"}, TargetListProvider: ptr.To(true)}
				s.False(filter(tool))
			})
			s.Run("and tool is configuration_contexts_list and targetName is context: returns true", func() {
				filter := ShouldIncludeTargetListTool("context", true)
				tool := api.ServerTool{Tool: api.Tool{Name: "configuration_contexts_list"}, TargetListProvider: ptr.To(true)}
				s.True(filter(tool))
			})
			s.Run("and tool is mutated targets_list and targetName is context: returns false", func() {
				filter := ShouldIncludeTargetListTool("context", true)
				tool := api.ServerTool{Tool: api.Tool{Name: "context_list"}, TargetListProvider: ptr.To(true)}
				s.False(filter(tool))
			})
			s.Run("and tool is mutated targets_list and targetName is not context: returns true", func() {
				filter := ShouldIncludeTargetListTool("cluster", true)
				tool := api.ServerTool{Tool: api.Tool{Name: "cluster_list"}, TargetListProvider: ptr.To(true)}
				s.True(filter(tool))
			})
		})
	})
}

func (s *ToolFilterSuite) TestShouldIncludeConfigurationViewTool() {
	tool := api.ServerTool{Tool: api.Tool{Name: "configuration_view"}}

	s.Run("hides configuration view over HTTP by default", func() {
		s.False(ShouldIncludeConfigurationViewTool(true, nil)(tool))
	})
	s.Run("includes explicitly enabled configuration view over HTTP", func() {
		s.True(ShouldIncludeConfigurationViewTool(true, []string{"configuration_view"})(tool))
	})
	s.Run("includes configuration view over stdio", func() {
		s.True(ShouldIncludeConfigurationViewTool(false, nil)(tool))
	})
	s.Run("includes other tools over HTTP", func() {
		otherTool := api.ServerTool{Tool: api.Tool{Name: "configuration_contexts_list"}}
		s.True(ShouldIncludeConfigurationViewTool(true, nil)(otherTool))
	})
}

func (s *ToolFilterSuite) TestTargetCompatible() {
	cfg := &Configuration{Config: config.New()}
	cfg.EnableTargetCompatibilityToolFilters.SetForTest(true)
	applicable := cfg.isToolApplicable(s.T().Context())

	s.Run("accepts when every filter accepts", func() {
		tool := api.ServerTool{TargetCompatibilityFilters: []api.TargetCompatibilityFilter{
			func(context.Context) bool { return true },
			func(context.Context) bool { return true },
		}}
		s.True(applicable(tool))
	})

	s.Run("rejects when any filter rejects", func() {
		tool := api.ServerTool{TargetCompatibilityFilters: []api.TargetCompatibilityFilter{
			func(context.Context) bool { return true },
			func(context.Context) bool { return false },
		}}
		s.False(applicable(tool))
	})

	s.Run("cancels the other filters after a rejection", func() {
		canceled := make(chan struct{})
		tool := api.ServerTool{TargetCompatibilityFilters: []api.TargetCompatibilityFilter{
			func(ctx context.Context) bool {
				<-ctx.Done()
				close(canceled)
				return true
			},
			func(context.Context) bool { return false },
		}}
		s.False(applicable(tool))
		select {
		case <-canceled:
		case <-time.After(2 * time.Second):
			s.Fail("sibling filter was not canceled")
		}
	})
}

func TestToolFilter(t *testing.T) {
	suite.Run(t, new(ToolFilterSuite))
}
