package mcpapps

import (
	"embed"
	"io/fs"
	"strings"
	"sync"
)

const ResourceMIMEType = "text/html;profile=mcp-app"

// ViewerResourceURIPrefix is the base URI prefix for per-tool viewer resources.
const ViewerResourceURIPrefix = "ui://kubernetes-mcp-server/tool/"

//go:embed viewer vendor
var embeddedFS embed.FS

var (
	baseHTML      string
	buildOnce     sync.Once
	toolHTMLCache sync.Map // map[string]string — per-tool assembled HTML
)

func mustReadFile(name string) string {
	data, err := fs.ReadFile(embeddedFS, name)
	if err != nil {
		panic("mcpapps: embedded file not found: " + name)
	}
	return string(data)
}

func buildBaseHTML() string {
	buildOnce.Do(func() {
		baseHTML = mustReadFile("viewer/viewer.html")
		baseHTML = strings.Replace(baseHTML, "INJECT_CSS", mustReadFile("viewer/style.css"), 1)
		baseHTML = strings.Replace(baseHTML, "INJECT_VENDOR_HTM_PREACT", mustReadFile("vendor/htm-preact-standalone.umd.js"), 1)
		baseHTML = strings.Replace(baseHTML, "INJECT_VENDOR_CHART_JS", mustReadFile("vendor/chart.umd.min.js"), 1)
		baseHTML = strings.Replace(baseHTML, "INJECT_VENDOR_PRISM_CORE", mustReadFile("vendor/prism-core.min.js"), 1)
		baseHTML = strings.Replace(baseHTML, "INJECT_VENDOR_PRISM_YAML", mustReadFile("vendor/prism-yaml.min.js"), 1)
		baseHTML = strings.Replace(baseHTML, "INJECT_PROTOCOL_JS", mustReadFile("viewer/protocol.js"), 1)
		baseHTML = strings.Replace(baseHTML, "INJECT_COMPONENTS_JS", mustReadFile("viewer/components.js"), 1)
		baseHTML = strings.Replace(baseHTML, "INJECT_APP_JS", mustReadFile("viewer/app.js"), 1)
	})
	return baseHTML
}

// ViewerHTMLForTool returns the assembled viewer HTML with the tool name injected.
// The result is cached per tool name.
func ViewerHTMLForTool(toolName string) string {
	if cached, ok := toolHTMLCache.Load(toolName); ok {
		return cached.(string)
	}
	html := strings.Replace(buildBaseHTML(), "INJECT_TOOL_NAME", toolName, 1)
	toolHTMLCache.Store(toolName, html)
	return html
}

// ToolResourceURI returns the per-tool resource URI for the given tool name.
func ToolResourceURI(toolName string) string {
	return ViewerResourceURIPrefix + toolName
}

// ToolMetaForTool returns the _meta map for a specific tool, pointing to its per-tool resource URI.
// It sets both the nested "ui.resourceUri" key and the legacy flat "ui/resourceUri" key
// for backward compatibility with MCP hosts that read the flat key (e.g. VS Code).
func ToolMetaForTool(toolName string) map[string]any {
	uri := ToolResourceURI(toolName)
	return map[string]any{
		"ui":             map[string]any{"resourceUri": uri},
		"ui/resourceUri": uri,
	}
}
