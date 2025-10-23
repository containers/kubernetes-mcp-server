package mcp

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

// McpToolProcessingSuite tests MCP tool processing (isToolApplicable)
type McpToolProcessingSuite struct {
	BaseMcpSuite
}

func (s *McpToolProcessingSuite) TestUnrestricted() {
	s.InitMcpClient()

	tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("Destructive tools ARE NOT read only", func() {
		for _, tool := range tools.Tools {
			readOnly := ptr.Deref(tool.Annotations.ReadOnlyHint, false)
			destructive := ptr.Deref(tool.Annotations.DestructiveHint, false)
			s.Falsef(readOnly && destructive, "Tool %s is read-only and destructive, which is not allowed", tool.Name)
		}
	})
}

func (s *McpToolProcessingSuite) TestReadOnly() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		read_only = true
	`), s.Cfg), "Expected to parse read only server config")
	s.InitMcpClient()

	tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("ListTools returns only read-only tools", func() {
		for _, tool := range tools.Tools {
			s.Falsef(tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint,
				"Tool %s is not read-only but should be", tool.Name)
			s.Falsef(tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint,
				"Tool %s is destructive but should not be in read-only mode", tool.Name)
		}
	})
}

func (s *McpToolProcessingSuite) TestDisableDestructive() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		disable_destructive = true
	`), s.Cfg), "Expected to parse disable destructive server config")
	s.InitMcpClient()

	tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
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
	s.Require().NoError(toml.Unmarshal([]byte(`
		enabled_tools = [ "namespaces_list", "events_list" ]
	`), s.Cfg), "Expected to parse enabled tools server config")
	s.InitMcpClient()

	tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
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
	s.Require().NoError(toml.Unmarshal([]byte(`
		disabled_tools = [ "namespaces_list", "events_list" ]
	`), s.Cfg), "Expected to parse disabled tools server config")
	s.InitMcpClient()

	tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
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
