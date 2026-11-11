package core

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type ToolsetSuite struct {
	suite.Suite
}

func (s *ToolsetSuite) TestResourceDescriptionsUseCompatibilityConfig() {
	for _, tc := range []struct {
		name          string
		config        *config.Config
		includesRoute bool
	}{
		{name: "nil config preserves unfiltered examples", includesRoute: true},
		{name: "disabled filtering preserves unfiltered examples", config: config.BaseDefault(), includesRoute: true},
		{name: "enabled filtering omits unavailable Route", config: config.BaseDefault()},
	} {
		s.Run(tc.name, func() {
			ctx := api.ToolsetContext{Config: tc.config, Inspector: &fakeInspector{}}
			if tc.config != nil {
				ctx.TargetCompatibilityFiltersEnabled = !tc.includesRoute
			}
			tools := (&Toolset{}).GetTools(s.T().Context(), ctx)
			for _, tool := range tools {
				if tool.Tool.Name != "resources_list" {
					continue
				}
				if tc.includesRoute {
					s.Contains(tool.Tool.Description, "route.openshift.io/v1 Route")
				} else {
					s.NotContains(tool.Tool.Description, "route.openshift.io/v1 Route")
				}
				return
			}
			s.Fail("resources_list tool was not registered")
		})
	}
}

func TestToolset(t *testing.T) {
	suite.Run(t, new(ToolsetSuite))
}
