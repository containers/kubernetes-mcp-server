package mcpapps

import (
	"context"
	"maps"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// ContentProvider supplies the complete, self-contained HTML document for an
// MCP App resource. Toolsets may use it to assemble HTML from embedded assets
// when the resource is read.
type ContentProvider func(context.Context) (string, error)

// StaticHTML adapts a static, already bundled HTML document into a
// ContentProvider. It is intended for content embedded by a toolset with
// //go:embed.
func StaticHTML(html string) ContentProvider {
	return func(context.Context) (string, error) {
		return html, nil
	}
}

// CustomOption configures a custom MCP App resource.
type CustomOption func(*api.ToolApp)

// WithDescription sets the human-readable description of a custom app.
func WithDescription(description string) CustomOption {
	return func(app *api.ToolApp) {
		app.Description = description
	}
}

// WithMetadata sets resource metadata for a custom app. The metadata may
// include MCP Apps rendering preferences such as ui.prefersBorder.
func WithMetadata(meta map[string]any) CustomOption {
	return func(app *api.ToolApp) {
		app.Meta = maps.Clone(meta)
	}
}

// Custom creates an MCP App resource owned by a toolset. The MCP layer
// registers the returned app, links it to the tool, and handles protocol
// wiring. The toolset owns the URI, HTML assets, and decision to attach the
// app to a tool.
//
// Use StaticHTML for a self-contained HTML document, or pass a provider that
// assembles embedded assets when the resource is read.
func Custom(uri, name string, content ContentProvider, options ...CustomOption) *api.ToolApp {
	app := &api.ToolApp{
		URI:     uri,
		Name:    name,
		Handler: content,
	}
	for _, option := range options {
		if option != nil {
			option(app)
		}
	}
	return app
}
