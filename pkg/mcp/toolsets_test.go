package mcp

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	configuration "github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
)

type ToolsetsSuite struct {
	suite.Suite
	originalToolsets []api.Toolset
	*test.MockServer
	*test.McpClient
	Cfg       *configuration.StaticConfig
	mcpServer *Server
}

func (s *ToolsetsSuite) SetupTest() {
	s.originalToolsets = toolsets.Toolsets()
	s.MockServer = test.NewMockServer()
	s.Cfg = configuration.Default()
	s.Cfg.KubeConfig = s.MockServer.KubeconfigFile(s.T())
}

func (s *ToolsetsSuite) TearDownTest() {
	toolsets.Clear()
	for _, toolset := range s.originalToolsets {
		toolsets.Register(toolset)
	}
	s.MockServer.Close()
}

func (s *ToolsetsSuite) TearDownSubTest() {
	if s.McpClient != nil {
		s.McpClient.Close()
	}
	if s.mcpServer != nil {
		s.mcpServer.Close()
	}
}

func (s *ToolsetsSuite) TestNoToolsets() {
	s.Run("No toolsets registered", func() {
		toolsets.Clear()
		s.Cfg.Toolsets = []string{}
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Run("ListTools returns no tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
			s.Empty(tools.Tools, "Expected no tools from ListTools")
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsTools() {
	s.Run("Default configuration toolsets", func() {
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("ListTools returns correct Tool metadata", func() {
			expectedMetadata := test.ReadFile("testdata", "toolsets-full-tools.json")
			metadata, err := json.MarshalIndent(tools.Tools, "", "  ")
			s.Require().NoErrorf(err, "failed to marshal tools metadata: %v", err)
			s.JSONEq(expectedMetadata, string(metadata), "tools metadata does not match expected")
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsToolsInOpenShift() {
	s.Run("Default configuration toolsets in OpenShift", func() {
		s.Handle(&test.InOpenShiftHandler{})
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("ListTools returns correct Tool metadata", func() {
			expectedMetadata := test.ReadFile("testdata", "toolsets-full-tools-openshift.json")
			metadata, err := json.MarshalIndent(tools.Tools, "", "  ")
			s.Require().NoErrorf(err, "failed to marshal tools metadata: %v", err)
			s.JSONEq(expectedMetadata, string(metadata), "tools metadata does not match expected")
		})
	})
}

func (s *ToolsetsSuite) TestGranularToolsetsTools() {
	testCases := []api.Toolset{
		&core.Toolset{},
		&config.Toolset{},
		&helm.Toolset{},
	}
	for _, testCase := range testCases {
		s.Run("Toolset "+testCase.GetName(), func() {
			toolsets.Clear()
			toolsets.Register(testCase)
			s.Cfg.Toolsets = []string{testCase.GetName()}
			s.InitMcpClient()
			tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
			s.Run("ListTools returns tools", func() {
				s.NotNil(tools, "Expected tools from ListTools")
				s.NoError(err, "Expected no error from ListTools")
			})
			s.Run("ListTools returns correct Tool metadata", func() {
				expectedMetadata := test.ReadFile("testdata", "toolsets-"+testCase.GetName()+"-tools.json")
				metadata, err := json.MarshalIndent(tools.Tools, "", "  ")
				s.Require().NoErrorf(err, "failed to marshal tools metadata: %v", err)
				s.JSONEq(expectedMetadata, string(metadata), "tools metadata does not match expected")
			})
		})
	}
}

func (s *ToolsetsSuite) InitMcpClient() {
	var err error
	s.mcpServer, err = NewServer(Configuration{StaticConfig: s.Cfg})
	s.Require().NoError(err, "Expected no error creating MCP server")
	s.McpClient = test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP(nil))
}

func TestToolsets(t *testing.T) {
	suite.Run(t, new(ToolsetsSuite))
}
