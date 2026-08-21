package tools

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

type MeshStatusToolSuite struct {
	suite.Suite
}

func (s *MeshStatusToolSuite) TestToolRegistration() {
	s.Run("tool is registered with expected metadata", func() {
		tools := InitGetMeshStatus()
		s.Require().Len(tools, 1, "Expected 1 mesh status tool")
		s.Equal(defaults.ToolsetName()+"_get_mesh_status", tools[0].Tool.Name)
		s.Equal("Get Mesh Status", tools[0].Tool.Annotations.Title)
		s.NotNil(tools[0].Tool.InputSchema)
		s.NotNil(tools[0].Handler)
		s.Empty(tools[0].TargetCompatibilityFilters, "Filters are applied by the toolset, not Init*")
	})
}

func TestMeshStatusToolSuite(t *testing.T) {
	suite.Run(t, new(MeshStatusToolSuite))
}
