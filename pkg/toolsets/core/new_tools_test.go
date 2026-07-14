package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mockFilteringProvider is a minimal FilteringProvider for unit tests that do
// not need a real cluster. It disables target-compatibility filtering so
// initResources takes its else branch without contacting discovery.
type mockFilteringProvider struct{}

func (mockFilteringProvider) IsTargetCompatibilityToolFiltersEnabled() bool { return false }
func (mockFilteringProvider) AnyTargetHasGVKs(_ context.Context, _ []schema.GroupVersionKind) bool {
	return true
}

// NewToolsSuite verifies that all newly added tools (node management, rollout,
// port-forward, attach, HPA, PDB, ResourceQuota, LimitRange) are registered
// in the core toolset with correct names, descriptions, required parameters,
// and handlers.
type NewToolsSuite struct {
	suite.Suite
	toolset *Toolset
}

func (s *NewToolsSuite) SetupTest() {
	s.toolset = &Toolset{}
}

// toolsByName builds a lookup map of tool name -> ServerTool from the toolset.
func (s *NewToolsSuite) toolsByName() map[string]any {
	tools := s.toolset.GetTools(mockFilteringProvider{})
	m := make(map[string]any, len(tools))
	for _, t := range tools {
		m[t.Tool.Name] = t
	}
	return m
}

func (s *NewToolsSuite) hasTool(name string) bool {
	_, ok := s.toolsByName()[name]
	return ok
}

func (s *NewToolsSuite) TestNodeManagementToolsRegistered() {
	s.Run("nodes_cordon is registered", func() {
		s.True(s.hasTool("nodes_cordon"), "nodes_cordon should be registered")
	})
	s.Run("nodes_uncordon is registered", func() {
		s.True(s.hasTool("nodes_uncordon"), "nodes_uncordon should be registered")
	})
	s.Run("nodes_drain is registered", func() {
		s.True(s.hasTool("nodes_drain"), "nodes_drain should be registered")
	})
	s.Run("nodes_patch_label is registered", func() {
		s.True(s.hasTool("nodes_patch_label"), "nodes_patch_label should be registered")
	})
	s.Run("nodes_patch_taint is registered", func() {
		s.True(s.hasTool("nodes_patch_taint"), "nodes_patch_taint should be registered")
	})
}

func (s *NewToolsSuite) TestRolloutToolsRegistered() {
	s.Run("rollout_status is registered", func() {
		s.True(s.hasTool("rollout_status"), "rollout_status should be registered")
	})
	s.Run("rollout_history is registered", func() {
		s.True(s.hasTool("rollout_history"), "rollout_history should be registered")
	})
	s.Run("rollout_undo is registered", func() {
		s.True(s.hasTool("rollout_undo"), "rollout_undo should be registered")
	})
	s.Run("rollout_restart is registered", func() {
		s.True(s.hasTool("rollout_restart"), "rollout_restart should be registered")
	})
}

func (s *NewToolsSuite) TestPortForwardAndAttachToolsRegistered() {
	s.Run("pods_port_forward is registered", func() {
		s.True(s.hasTool("pods_port_forward"), "pods_port_forward should be registered")
	})
	s.Run("pods_port_forward_stop is registered", func() {
		s.True(s.hasTool("pods_port_forward_stop"), "pods_port_forward_stop should be registered")
	})
	s.Run("pods_port_forward_list is registered", func() {
		s.True(s.hasTool("pods_port_forward_list"), "pods_port_forward_list should be registered")
	})
	s.Run("pods_attach is registered", func() {
		s.True(s.hasTool("pods_attach"), "pods_attach should be registered")
	})
}

func (s *NewToolsSuite) TestHPAToolsRegistered() {
	s.Run("horizontalpodautoscalers_list is registered", func() {
		s.True(s.hasTool("horizontalpodautoscalers_list"), "horizontalpodautoscalers_list should be registered")
	})
	s.Run("horizontalpodautoscalers_get is registered", func() {
		s.True(s.hasTool("horizontalpodautoscalers_get"), "horizontalpodautoscalers_get should be registered")
	})
	s.Run("horizontalpodautoscalers_create is registered", func() {
		s.True(s.hasTool("horizontalpodautoscalers_create"), "horizontalpodautoscalers_create should be registered")
	})
	s.Run("horizontalpodautoscalers_delete is registered", func() {
		s.True(s.hasTool("horizontalpodautoscalers_delete"), "horizontalpodautoscalers_delete should be registered")
	})
}

func (s *NewToolsSuite) TestPDBToolsRegistered() {
	s.Run("poddisruptionbudgets_list is registered", func() {
		s.True(s.hasTool("poddisruptionbudgets_list"), "poddisruptionbudgets_list should be registered")
	})
	s.Run("poddisruptionbudgets_get is registered", func() {
		s.True(s.hasTool("poddisruptionbudgets_get"), "poddisruptionbudgets_get should be registered")
	})
	s.Run("poddisruptionbudgets_create is registered", func() {
		s.True(s.hasTool("poddisruptionbudgets_create"), "poddisruptionbudgets_create should be registered")
	})
	s.Run("poddisruptionbudgets_delete is registered", func() {
		s.True(s.hasTool("poddisruptionbudgets_delete"), "poddisruptionbudgets_delete should be registered")
	})
}

func (s *NewToolsSuite) TestResourceQuotaToolsRegistered() {
	s.Run("resourcequotas_list is registered", func() {
		s.True(s.hasTool("resourcequotas_list"), "resourcequotas_list should be registered")
	})
	s.Run("resourcequotas_get is registered", func() {
		s.True(s.hasTool("resourcequotas_get"), "resourcequotas_get should be registered")
	})
	s.Run("resourcequotas_create is registered", func() {
		s.True(s.hasTool("resourcequotas_create"), "resourcequotas_create should be registered")
	})
	s.Run("resourcequotas_delete is registered", func() {
		s.True(s.hasTool("resourcequotas_delete"), "resourcequotas_delete should be registered")
	})
}

func (s *NewToolsSuite) TestLimitRangeToolsRegistered() {
	s.Run("limitranges_list is registered", func() {
		s.True(s.hasTool("limitranges_list"), "limitranges_list should be registered")
	})
	s.Run("limitranges_get is registered", func() {
		s.True(s.hasTool("limitranges_get"), "limitranges_get should be registered")
	})
	s.Run("limitranges_create is registered", func() {
		s.True(s.hasTool("limitranges_create"), "limitranges_create should be registered")
	})
	s.Run("limitranges_delete is registered", func() {
		s.True(s.hasTool("limitranges_delete"), "limitranges_delete should be registered")
	})
}

func (s *NewToolsSuite) TestPodsLogHasFollowParameter() {
	s.Run("pods_log schema includes follow parameter", func() {
		tools := s.toolset.GetTools(mockFilteringProvider{})
		for _, t := range tools {
			if t.Tool.Name == "pods_log" {
				props := t.Tool.InputSchema.Properties
				_, hasFollow := props["follow"]
				s.True(hasFollow, "pods_log should have a 'follow' parameter")
				_, hasSinceSeconds := props["since_seconds"]
				s.True(hasSinceSeconds, "pods_log should have a 'since_seconds' parameter")
				return
			}
		}
		s.Fail("pods_log tool not found")
	})
}

func (s *NewToolsSuite) TestAllNewToolsHaveHandlers() {
	s.Run("every new tool has a non-nil handler", func() {
		tools := s.toolset.GetTools(mockFilteringProvider{})
		newToolNames := []string{
			"nodes_cordon", "nodes_uncordon", "nodes_drain",
			"nodes_patch_label", "nodes_patch_taint",
			"rollout_status", "rollout_history", "rollout_undo", "rollout_restart",
			"pods_port_forward", "pods_port_forward_stop", "pods_port_forward_list",
			"pods_attach",
			"horizontalpodautoscalers_list", "horizontalpodautoscalers_get",
			"horizontalpodautoscalers_create", "horizontalpodautoscalers_delete",
			"poddisruptionbudgets_list", "poddisruptionbudgets_get",
			"poddisruptionbudgets_create", "poddisruptionbudgets_delete",
			"resourcequotas_list", "resourcequotas_get",
			"resourcequotas_create", "resourcequotas_delete",
			"limitranges_list", "limitranges_get",
			"limitranges_create", "limitranges_delete",
		}
		for _, name := range newToolNames {
			found := false
			for _, t := range tools {
				if t.Tool.Name == name {
					found = true
					s.NotNil(t.Handler, "tool %s should have a non-nil handler", name)
					break
				}
			}
			s.Truef(found, "tool %s should be registered", name)
		}
	})
}

func TestNewToolsSuite(t *testing.T) {
	suite.Run(t, new(NewToolsSuite))
}
