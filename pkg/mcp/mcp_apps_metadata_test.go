package mcp

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

type McpAppsMetadataSuite struct{ suite.Suite }

func (s *McpAppsMetadataSuite) TestWithAppResourceURI() {
	originalUI := map[string]any{"prefersBorder": true}
	original := map[string]any{
		"custom": "value",
		"ui":     originalUI,
	}

	result := withAppResourceURI(original, "ui://example/app")

	s.Run("preserves existing metadata", func() {
		s.Equal("value", result["custom"])
		ui, ok := result["ui"].(map[string]any)
		s.Require().True(ok)
		s.True(ui["prefersBorder"].(bool))
	})
	s.Run("adds the app resource URI", func() {
		ui := result["ui"].(map[string]any)
		s.Equal("ui://example/app", ui["resourceUri"])
	})
	s.Run("does not mutate the original metadata", func() {
		s.Equal(map[string]any{"prefersBorder": true}, originalUI)
		s.Equal(originalUI, original["ui"])
	})
}

func (s *McpAppsMetadataSuite) TestAppResourcesValidation() {
	s.Run("rejects an app without a handler", func() {
		_, err := appResources([]api.ServerTool{{
			Tool: api.Tool{Name: "missing-handler"},
			App:  &api.ToolApp{URI: "ui://example/app"},
		}})
		s.ErrorContains(err, "handler")
	})
	s.Run("rejects an app without an absolute URI", func() {
		_, err := appResources([]api.ServerTool{{
			Tool: api.Tool{Name: "missing-uri"},
			App: &api.ToolApp{
				Handler: func(_ context.Context) (string, error) { return "", nil },
			},
		}})
		s.ErrorContains(err, "URI")
	})
	s.Run("rejects duplicate app URIs", func() {
		app := func(context.Context) (string, error) { return "", nil }
		_, err := appResources([]api.ServerTool{
			{Tool: api.Tool{Name: "first"}, App: &api.ToolApp{URI: "ui://example/app", Handler: app}},
			{Tool: api.Tool{Name: "second"}, App: &api.ToolApp{URI: "ui://example/app", Handler: app}},
		})
		s.ErrorContains(err, "same MCP App URI")
	})
}

func TestMcpAppsMetadata(t *testing.T) {
	suite.Run(t, new(McpAppsMetadataSuite))
}
