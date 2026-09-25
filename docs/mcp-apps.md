# MCP Apps

MCP Apps let a tool associate an interactive user interface with its result. Enable
them in the server configuration before starting the server:

```toml
apps_enabled = true
```

`apps_enabled` is a startup setting. Restart the server after changing it.

When enabled, tools that provide an app expose a `ui://` resource and include its
URI in the tool metadata. MCP clients that support the `io.modelcontextprotocol/ui`
extension can render that resource. Clients without UI support continue to receive
the tool's normal text and structured results.

The namespace list tool is the initial example. Its app is self-contained and does
not load scripts, styles, or data from the network.

## Authoring an app in a toolset

Toolsets can attach a custom app without changing `pkg/mcp`. Embed a complete
HTML document and use `mcpapps.Custom` with `mcpapps.StaticHTML`:

```go
//go:embed ui/pods-list.html
var podsListHTML string

tool := api.ServerTool{
    Tool:    podsListTool,
    Handler: podsList,
    App: mcpapps.Custom(
        "ui://example/pods-list",
        "Pods list",
        mcpapps.StaticHTML(podsListHTML),
        mcpapps.WithDescription("Interactive pod list"),
        mcpapps.WithMetadata(map[string]any{
            "ui": map[string]any{"prefersBorder": true},
        }),
    ),
}
```

For an app assembled from several embedded assets, pass a `ContentProvider`
instead. It is called when the host reads the resource:

```go
App: mcpapps.Custom("ui://example/workloads", "Workloads", func(ctx context.Context) (string, error) {
    return assembleEmbeddedHTML(), nil
})
```

The toolset owns its URI, HTML, CSS, JavaScript, images, and vendored
dependencies. Keep the returned document self-contained unless its resource
metadata explicitly declares the external domains it needs. Downstream toolsets
can provide their own embedded assets or content provider for product branding;
they do not need to modify `pkg/mcp` or `pkg/mcpapps`.
