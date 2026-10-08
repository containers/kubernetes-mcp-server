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

The standard core apps are self-contained and do not load scripts, styles, or
data from the network.

## Standard apps

Toolsets can reuse these helpers instead of owning an HTML document:

| Helper | Core tools | Structured result |
| --- | --- | --- |
| `mcpapps.Table` | `namespaces_list`, `projects_list`, `pods_list`, `pods_list_in_namespace`, `resources_list`, `events_list` | `{ "columns": ["Name", ...], "items": [...] }` |
| `mcpapps.Metrics` | `pods_top`, `nodes_top` | The same row envelope, with CPU and Memory values as Kubernetes quantities |
| `mcpapps.Resource` | `pods_get`, `resources_get` | One resource object |

```go
App: mcpapps.Table(
    "ui://example/workloads",
    "Workloads",
    mcpapps.WithDescription("Interactive workload table"),
)
```

Table rows can contain flat cells or full Kubernetes resource objects, which the
app flattens only for display. `columns` is optional and controls column order;
any remaining columns are inferred from the rows. Empty lists must use
`items: []`. `resources_list` preserves its existing structured output, including
complete objects with YAML output; the tool's text still follows the configured
output format. Metrics aggregate
CPU and memory per pod and sort resource quantities numerically. Resource apps
show an overview, labels, conditions, and Pod container details. Specification,
status, annotations, and the complete raw JSON are available in expandable
sections; tool text and structured data are unchanged.

All standard apps bundle `pkg/mcpapps/styles.css`, `standard.js`, and
`standard.html`. Common styling changes belong in `styles.css` and apply to every
standard app without changes to individual tools. The document is assembled from
embedded assets when the app is constructed; there are no runtime asset requests.
The shared runtime reports content-size changes to the host after initialization,
including expanding and collapsing details, and stops observing on teardown.
It also responds to host pings, displays tool cancellation, and applies the host's
initial theme and subsequent partial context updates. Without a host theme, apps
follow the operating system's color preference.

`make browser-test` checks rendering, sorting, errors, safe text insertion, and
the app message lifecycle, alongside the real MCP Apps basic-host integration.
The test host installs separately from the upstream example workspaces, using
`test/browser/basic-host/package-lock.json`. It serves its own local Vite assets;
the standard apps remain self-contained. After running the tests, audit the host
with `npm audit --prefix _output/browser/basic-host`.

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
App: mcpapps.Custom("ui://example/workloads", "Workloads", func(params api.ResourceHandlerParams) (string, error) {
    return assembleEmbeddedHTML(), nil
})
```

The toolset owns its URI, HTML, CSS, JavaScript, images, and vendored
dependencies. Keep the returned document self-contained unless its resource
metadata explicitly declares the external domains it needs. Downstream toolsets
can provide their own embedded assets or content provider for product branding;
they do not need to modify `pkg/mcp` or `pkg/mcpapps`.
