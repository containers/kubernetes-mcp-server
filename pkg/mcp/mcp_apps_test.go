package mcp

import (
	"encoding/json"
	"testing"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type McpAppsSuite struct{ BaseMcpSuite }

func (s *McpAppsSuite) TestNamespacesListApp() {
	s.Cfg.AppsEnabled.SetForTest(true)
	s.InitMcpClient()

	s.Run("advertises the MCP Apps extension", func() {
		s.Require().NotNil(s.InitializeResult.Capabilities.Resources)
		s.Contains(s.InitializeResult.Capabilities.Extensions, "io.modelcontextprotocol/ui")
	})
	s.Run("associates namespaces_list with its UI resource", func() {
		tools, err := s.ListTools()
		s.Require().NoError(err)
		for _, tool := range tools.Tools {
			if tool.Name != "namespaces_list" {
				continue
			}
			ui, ok := tool.Meta["ui"].(map[string]any)
			s.Require().True(ok)
			s.Equal("ui://kubernetes-mcp-server/namespaces-list", ui["resourceUri"])
			return
		}
		s.Fail("namespaces_list was not registered")
	})
	s.Run("serves the application HTML through resources/read", func() {
		resource, err := s.Session.ReadResource(s.T().Context(), &gosdk.ReadResourceParams{
			URI: "ui://kubernetes-mcp-server/namespaces-list",
		})
		s.Require().NoError(err)
		s.Require().Len(resource.Contents, 1)
		s.Equal("text/html;profile=mcp-app", resource.Contents[0].MIMEType)
		s.Contains(resource.Contents[0].Text, "ui/notifications/tool-result")
		ui, ok := resource.Contents[0].Meta["ui"].(map[string]any)
		s.Require().True(ok)
		s.True(ui["prefersBorder"].(bool))
	})
	s.Run("returns flat UI rows independently from YAML output", func() {
		result, err := s.CallTool("namespaces_list", map[string]any{})
		s.Require().NoError(err)
		var payload struct {
			Items []map[string]any `json:"items"`
		}
		encoded, err := json.Marshal(result.StructuredContent)
		s.Require().NoError(err)
		s.Require().NoError(json.Unmarshal(encoded, &payload))
		for _, row := range payload.Items {
			s.NotContains(row, "metadata")
		}
	})
}

func (s *McpAppsSuite) TestAppsDisabled() {
	s.InitMcpClient()

	s.Run("does not associate namespaces_list with an app", func() {
		tools, err := s.ListTools()
		s.Require().NoError(err)
		for _, tool := range tools.Tools {
			if tool.Name == "namespaces_list" {
				s.NotContains(tool.Meta, "ui")
				return
			}
		}
		s.Fail("namespaces_list was not registered")
	})
	s.Run("does not register the app resource", func() {
		_, err := s.Session.ReadResource(s.T().Context(), &gosdk.ReadResourceParams{
			URI: "ui://kubernetes-mcp-server/namespaces-list",
		})
		s.Error(err)
	})
}

func (s *McpAppsSuite) TestAppsAdvertisedWithoutAnEnabledAppTool() {
	s.Cfg.AppsEnabled.SetForTest(true)
	s.Cfg.EnabledTools.SetForTest([]string{"events_list"})
	s.InitMcpClient()

	s.Run("advertises the extension", func() {
		s.Contains(s.InitializeResult.Capabilities.Extensions, "io.modelcontextprotocol/ui")
	})
	s.Run("does not register an app resource", func() {
		_, err := s.Session.ReadResource(s.T().Context(), &gosdk.ReadResourceParams{
			URI: "ui://kubernetes-mcp-server/namespaces-list",
		})
		s.Error(err)
	})
}

func TestMcpApps(t *testing.T) {
	suite.Run(t, new(McpAppsSuite))
}
