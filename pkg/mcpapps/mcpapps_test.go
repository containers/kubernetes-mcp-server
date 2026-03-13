package mcpapps

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type McpAppsSuite struct {
	suite.Suite
}

func (s *McpAppsSuite) TestViewerHTMLForTool() {
	s.Run("returns non-empty string", func() {
		html := ViewerHTMLForTool("pods_list")
		s.NotEmpty(html)
	})
	s.Run("does not contain any INJECT_ placeholders", func() {
		html := ViewerHTMLForTool("pods_list")
		s.NotContains(html, "INJECT_CSS")
		s.NotContains(html, "INJECT_VENDOR_HTM_PREACT")
		s.NotContains(html, "INJECT_VENDOR_CHART_JS")
		s.NotContains(html, "INJECT_VENDOR_PRISM_CORE")
		s.NotContains(html, "INJECT_VENDOR_PRISM_YAML")
		s.NotContains(html, "INJECT_PROTOCOL_JS")
		s.NotContains(html, "INJECT_COMPONENTS_JS")
		s.NotContains(html, "INJECT_APP_JS")
		s.NotContains(html, "INJECT_TOOL_NAME")
	})
	s.Run("contains Preact UMD bundle content", func() {
		html := ViewerHTMLForTool("pods_list")
		s.Contains(html, "htmPreact")
	})
	s.Run("contains Chart.js content", func() {
		html := ViewerHTMLForTool("pods_list")
		s.Contains(html, "Chart")
	})
	s.Run("contains Prism.js content with manual mode", func() {
		html := ViewerHTMLForTool("pods_list")
		s.Contains(html, "window.Prism")
		s.Contains(html, "Prism.languages")
	})
	s.Run("contains YamlView component", func() {
		html := ViewerHTMLForTool("pods_list")
		s.Contains(html, "YamlView")
	})
	s.Run("contains mcpProtocol namespace", func() {
		html := ViewerHTMLForTool("pods_list")
		s.Contains(html, "mcpProtocol")
	})
	s.Run("contains mcpComponents namespace", func() {
		html := ViewerHTMLForTool("pods_list")
		s.Contains(html, "mcpComponents")
	})
	s.Run("is valid HTML document", func() {
		html := ViewerHTMLForTool("pods_list")
		s.True(strings.HasPrefix(html, "<!DOCTYPE html>"))
		s.Contains(html, "</html>")
	})
	s.Run("injects tool name into HTML", func() {
		html := ViewerHTMLForTool("namespaces_list")
		s.Contains(html, "window.__mcpToolName = 'namespaces_list'")
	})
	s.Run("different tools get different HTML", func() {
		html1 := ViewerHTMLForTool("pods_list")
		html2 := ViewerHTMLForTool("namespaces_list")
		s.NotEqual(html1, html2)
		s.Contains(html1, "'pods_list'")
		s.Contains(html2, "'namespaces_list'")
	})
}

func (s *McpAppsSuite) TestEmbeddedFS() {
	s.Run("viewer directory contains expected files", func() {
		entries, err := fs.ReadDir(embeddedFS, "viewer")
		s.Require().NoError(err)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		s.Contains(names, "viewer.html")
		s.Contains(names, "style.css")
		s.Contains(names, "protocol.js")
		s.Contains(names, "components.js")
		s.Contains(names, "app.js")
	})
	s.Run("vendor directory contains expected files", func() {
		entries, err := fs.ReadDir(embeddedFS, "vendor")
		s.Require().NoError(err)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		s.Contains(names, "htm-preact-standalone.umd.js")
		s.Contains(names, "chart.umd.min.js")
		s.Contains(names, "prism-core.min.js")
		s.Contains(names, "prism-yaml.min.js")
	})
	s.Run("all embedded files are non-empty", func() {
		files := []string{
			"viewer/viewer.html",
			"viewer/style.css",
			"viewer/protocol.js",
			"viewer/components.js",
			"viewer/app.js",
			"vendor/htm-preact-standalone.umd.js",
			"vendor/chart.umd.min.js",
			"vendor/prism-core.min.js",
			"vendor/prism-yaml.min.js",
		}
		for _, f := range files {
			data, err := fs.ReadFile(embeddedFS, f)
			s.Require().NoError(err, "file %s should be readable", f)
			s.NotEmpty(data, "file %s should not be empty", f)
		}
	})
}

func (s *McpAppsSuite) TestToolMetaForTool() {
	s.Run("returns map with ui key", func() {
		meta := ToolMetaForTool("pods_list")
		s.Contains(meta, "ui")
	})
	s.Run("ui contains per-tool resourceUri", func() {
		meta := ToolMetaForTool("pods_list")
		ui, ok := meta["ui"].(map[string]any)
		s.Require().True(ok)
		s.Equal("ui://kubernetes-mcp-server/tool/pods_list", ui["resourceUri"])
	})
	s.Run("contains legacy flat ui/resourceUri key", func() {
		meta := ToolMetaForTool("pods_list")
		s.Equal("ui://kubernetes-mcp-server/tool/pods_list", meta["ui/resourceUri"])
	})
	s.Run("nested and legacy keys have same URI", func() {
		meta := ToolMetaForTool("pods_list")
		ui := meta["ui"].(map[string]any)
		s.Equal(ui["resourceUri"], meta["ui/resourceUri"])
	})
	s.Run("different tools get different URIs", func() {
		meta1 := ToolMetaForTool("pods_list")
		meta2 := ToolMetaForTool("namespaces_list")
		uri1 := meta1["ui"].(map[string]any)["resourceUri"]
		uri2 := meta2["ui"].(map[string]any)["resourceUri"]
		s.NotEqual(uri1, uri2)
	})
}

func (s *McpAppsSuite) TestToolResourceURI() {
	s.Run("has correct prefix", func() {
		uri := ToolResourceURI("pods_list")
		s.True(strings.HasPrefix(uri, ViewerResourceURIPrefix))
	})
	s.Run("includes tool name", func() {
		uri := ToolResourceURI("namespaces_list")
		s.Equal("ui://kubernetes-mcp-server/tool/namespaces_list", uri)
	})
}

func (s *McpAppsSuite) TestConstants() {
	s.Run("ViewerResourceURIPrefix has correct value", func() {
		s.Equal("ui://kubernetes-mcp-server/tool/", ViewerResourceURIPrefix)
	})
	s.Run("ResourceMIMEType has correct value", func() {
		s.Equal("text/html;profile=mcp-app", ResourceMIMEType)
	})
}

func TestMcpApps(t *testing.T) {
	suite.Run(t, new(McpAppsSuite))
}
