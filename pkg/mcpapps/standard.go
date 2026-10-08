package mcpapps

import (
	_ "embed"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

//go:embed standard.html
var standardHTML string

//go:embed styles.css
var standardStyles string

//go:embed standard.js
var standardScript string

// standardApp bundles the shared presentation assets into an offline document.
// Toolsets provide data; common styling and behavior are owned by this package.
func standardApp(uri, name, kind string, options ...CustomOption) *api.ToolApp {
	html := strings.NewReplacer(
		"{{KIND}}", kind,
		"{{STYLES}}", standardStyles,
		"{{SCRIPT}}", standardScript,
	).Replace(standardHTML)
	return Custom(uri, name, StaticHTML(html), options...)
}
