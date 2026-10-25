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

// WithStyles appends CSS after an app's existing styles, allowing toolsets to
// customize the standard apps without replacing their shared runtime. CSS must
// be trusted, bundled application code, not tool results or user input. Keep it
// self-contained for disconnected use. Custom documents must include </head>.
func WithStyles(css string) CustomOption {
	return func(app *api.ToolApp) {
		if css == "" || app.Handler == nil {
			return
		}
		handler := app.Handler
		app.Handler = func(params api.ResourceHandlerParams) (*api.ResourceContent, error) {
			content, err := handler(params)
			if err != nil || content == nil {
				return content, err
			}
			styled := *content
			styled.Text = strings.Replace(content.Text, "</head>", "<style>"+css+"</style>\n</head>", 1)
			return &styled, nil
		}
	}
}

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
