package mcp

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config/configtest"
	"github.com/stretchr/testify/suite"
)

// McpToolProcessingSuite tests MCP tool processing (isToolApplicable)
type McpToolProcessingSuite struct {
	BaseMcpSuite
}

func (s *McpToolProcessingSuite) TestUnrestricted() {
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("Destructive tools ARE NOT read only", func() {
		for _, tool := range tools.Tools {
			readOnly := tool.Annotations.ReadOnlyHint
			destructive := tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint
			s.Falsef(readOnly && destructive, "Tool %s is read-only and destructive, which is not allowed", tool.Name)
		}
	})
}

func (s *McpToolProcessingSuite) TestReadOnly() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		read_only = true
	`)
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("ListTools returns only read-only tools", func() {
		for _, tool := range tools.Tools {
			s.Truef(tool.Annotations.ReadOnlyHint,
				"Tool %s is not read-only but should be", tool.Name)
			s.Falsef(tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint,
				"Tool %s is destructive but should not be in read-only mode", tool.Name)
		}
	})
}

func (s *McpToolProcessingSuite) TestDisableDestructive() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		disable_destructive = true
	`)
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("ListTools does not return destructive tools", func() {
		for _, tool := range tools.Tools {
			s.Falsef(tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint,
				"Tool %s is destructive but should not be in disable_destructive mode", tool.Name)
		}
	})
}

func (s *McpToolProcessingSuite) TestEnabledTools() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		enabled_tools = [ "namespaces_list", "events_list" ]
	`)
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("ListTools returns only explicitly enabled tools", func() {
		s.Len(tools.Tools, 2, "ListTools should return exactly 2 tools")
		for _, tool := range tools.Tools {
			s.Falsef(tool.Name != "namespaces_list" && tool.Name != "events_list",
				"Tool %s is not enabled but should be", tool.Name)
		}
	})
}

func (s *McpToolProcessingSuite) TestDisabledTools() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		disabled_tools = [ "namespaces_list", "events_list" ]
	`)
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("ListTools does not return disabled tools", func() {
		for _, tool := range tools.Tools {
			s.Falsef(tool.Name == "namespaces_list" || tool.Name == "events_list",
				"Tool %s is not disabled but should be", tool.Name)
		}
	})
}

func TestMcpToolProcessing(t *testing.T) {
	suite.Run(t, new(McpToolProcessingSuite))
}
