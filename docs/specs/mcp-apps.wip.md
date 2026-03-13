# MCP Apps

MCP Apps enables tools to return interactive HTML-based UIs rendered in sandboxed iframes by MCP hosts. This document describes the architecture, design decisions, and configuration for the MCP Apps integration.

**Issue**: [containers/kubernetes-mcp-server#753](https://github.com/containers/kubernetes-mcp-server/issues/753)

## Table of Contents

- [1. What Are MCP Apps](#1-what-are-mcp-apps)
- [2. Go-SDK Support](#2-go-sdk-support)
- [3. Styling and CSS Isolation](#3-styling-and-css-isolation)
- [4. Multiple Widgets and Tool Call Accumulation](#4-multiple-widgets-and-tool-call-accumulation)
- [5. Frontend Stack Decision](#5-frontend-stack-decision)
- [6. MCP postMessage Protocol](#6-mcp-postmessage-protocol)
- [7. Configuration: Opt-In Feature](#7-configuration-opt-in-feature)
- [8. Architecture](#8-architecture)
- [9. Output Layer](#9-output-layer)
- [10. Per-Tool Resource URIs and Dual-Flow Viewer](#10-per-tool-resource-uris-and-dual-flow-viewer)
- [11. structuredContent Object Wrapping](#11-structuredcontent-object-wrapping)
- [12. Known Compatibility Notes](#12-known-compatibility-notes)
- [13. YAML Syntax Highlighting](#13-yaml-syntax-highlighting)
- [Appendix A: Research and Development Journal](#appendix-a-research-and-development-journal)

---

## 1. What Are MCP Apps

MCP Apps is the first official extension to the Model Context Protocol (spec version 2026-01-26).
It enables MCP tools to return interactive HTML-based UIs rendered in sandboxed iframes by the
MCP host (VS Code, Claude Desktop, ChatGPT, etc.).

The flow:

1. **Tool definition** declares `_meta.ui.resourceUri` pointing to a `ui://` resource
2. **Model calls the tool** — host executes it on the server
3. **Host fetches the UI resource** — gets HTML via MCP `resources/read`
4. **Host renders in sandboxed iframe** — passes tool data via `postMessage`
5. **Bidirectional communication** — app can call server tools via JSON-RPC over `postMessage`

Extension identifier: `io.modelcontextprotocol/ui`

Supported hosts: VS Code (Insiders and Stable), Claude (web/desktop), ChatGPT, Goose, Postman, MCPJam.

Key specification sources:
- [MCP Apps Specification (2026-01-26)](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx)
- [MCP Apps Documentation](https://modelcontextprotocol.io/docs/extensions/apps)
- [ext-apps SDK Repository](https://github.com/modelcontextprotocol/ext-apps)

## 2. Go-SDK Support

The go-sdk (v1.4.0) added an `Extensions` field to both `ServerCapabilities` and
`ClientCapabilities` per SEP-2133.

API surface:

```go
// On ServerCapabilities
Extensions map[string]any `json:"extensions,omitempty"`
func (c *ServerCapabilities) AddExtension(name string, settings map[string]any)

// On ClientCapabilities
Extensions map[string]any `json:"extensions,omitempty"`
func (c *ClientCapabilities) AddExtension(name string, settings map[string]any)
```

Both `AddExtension` methods normalize `nil` settings to `map[string]any{}` (spec requires
an object, not null).

### Usage for MCP Apps capability negotiation

**Server declares support** (in `initialize` response):
```json
{
  "capabilities": {
    "extensions": {
      "io.modelcontextprotocol/ui": {}
    }
  }
}
```

**Client declares support** (in `initialize` request):
```json
{
  "capabilities": {
    "extensions": {
      "io.modelcontextprotocol/ui": {
        "mimeTypes": ["text/html;profile=mcp-app"]
      }
    }
  }
}
```

In Go (`pkg/mcp/mcp.go`):
```go
caps.AddExtension("io.modelcontextprotocol/ui", nil)
```

## 3. Styling and CSS Isolation

### Spec guarantees

**CSS isolation is not a concern.** The MCP Apps specification mandates that all app views
run inside **sandboxed iframes**. This provides complete CSS isolation by design:

- Views have no access to the host's DOM, cookies, or storage
- All communication passes through `postMessage`
- App CSS cannot leak into the host; host CSS cannot reach the app
- Each app runs in its own isolated context

This means Shadow DOM, `@scope`, `all: initial`, and other CSS isolation techniques are
**unnecessary** for MCP Apps.

### Theming

The viewer applies host theme and CSS variables per the MCP Apps spec.

#### How theming works (per spec)

The host does **not** inject CSS variables directly into the iframe's DOM. Instead:

1. Host sends `hostContext.theme` (`"dark"` or `"light"`) and optionally
   `hostContext.styles.variables` (a `Record<string, string>` of CSS custom properties)
   via the `ui/initialize` response.
2. The **view is responsible** for applying these — calling
   `document.documentElement.style.colorScheme = theme` and iterating
   `styles.variables` with `style.setProperty(key, value)`.
3. Theme updates are communicated via `ui/notifications/host-context-changed`.

This is implemented in `viewer/app.js` via `applyTheme()` and `applyStyleVariables()`,
called from both the `initialize()` handler and the `host-context-changed` handler.

#### CSS variable names (spec-defined)

The spec defines ~65 CSS custom properties across these categories:

- **Background**: `--color-background-primary`, `-secondary`, `-tertiary`, `-inverse`,
  `-ghost`, `-info`, `-danger`, `-success`, `-warning`, `-disabled`
- **Text**: `--color-text-primary`, `-secondary`, `-tertiary`, etc.
- **Border**: `--color-border-primary`, `-secondary`, etc.
- **Ring/Focus**: `--color-ring-primary`, `-secondary`, etc.
- **Typography**: `--font-sans`, `--font-mono`, `--font-weight-*`, `--font-text-*-size`,
  `--font-heading-*-size`, line heights
- **Border radius**: `--border-radius-xs` through `-full`
- **Shadows**: `--shadow-hairline`, `-sm`, `-md`, `-lg`

Spacing is intentionally excluded (layouts break when spacing varies).

#### Fallback strategy: `light-dark()` CSS function

Since hosts may provide inconsistent variable subsets (or none at all — MCP Inspector
only provides `theme`, not `styles.variables`), all CSS uses `var()` with `light-dark()`
fallbacks:

```css
body {
  background: var(--color-background-primary, light-dark(#ffffff, #1a1a1a));
  color: var(--color-text-primary, light-dark(#1a1a2e, #e5e7eb));
}
```

The `light-dark()` CSS function selects the correct value based on the `color-scheme`
property set by `applyTheme()`. This means:
- If the host provides CSS variables → those are used
- If not → `light-dark()` picks the right fallback based on the theme

For Chart.js (which uses canvas, not CSS), colors are read from
`document.documentElement.getAttribute('data-theme')` at chart creation time.

#### Real-time theme switching

Real-time theme changes on already-rendered content require the host to send
`host-context-changed` notifications — this is the host's responsibility per the spec.
CSS-only elements (tables, text, borders) update automatically via `light-dark()` when
`color-scheme` changes. Canvas-based elements (Chart.js) are created with the correct
theme at render time but don't live-update without a re-render.

**MCP Inspector limitation**: The Inspector computes `hostContext` once with
`useMemo([], [])` and does not send `host-context-changed` on theme toggle.
This is a known limitation of the Inspector, not something the viewer can work around
from inside a sandboxed iframe.

The `prefersBorder` metadata allows apps to request border/background treatment from the host.

### Known issues

- Different hosts provide inconsistent CSS variable subsets
  ([ext-apps #382](https://github.com/modelcontextprotocol/ext-apps/issues/382))
- Platform-specific styling pressure — Claude, ChatGPT, VS Code each have their own guidelines
  ([ext-apps #467](https://github.com/modelcontextprotocol/ext-apps/issues/467))
- `autoResize: true` causes layout bugs including infinite growth loops
  ([ext-apps #502](https://github.com/modelcontextprotocol/ext-apps/issues/502))
- MCP Inspector does not send `host-context-changed` on theme toggle (computed once)

### Recommendation

- Always provide CSS variable fallback values using `light-dark()` for theme awareness
- Treat all host-provided variables as optional
- Use `autoResize: false` + explicit height + manual `sendSizeChanged()` for predictable sizing
- Set `prefersBorder` explicitly

### Default Content Security Policy

When `ui.csp` is omitted, the default CSP is:
```
default-src 'none';
script-src 'self' 'unsafe-inline';
style-src 'self' 'unsafe-inline';
img-src 'self' data:;
media-src 'self' data:;
connect-src 'none';
```

This means:
- Inline `<script>` and `<style>` tags work (`'unsafe-inline'`)
- No external network requests (`connect-src 'none'`)
- No CDN imports possible without declaring them in `_meta.ui.csp`

## 4. Multiple Widgets and Tool Call Accumulation

### Spec behavior

**Each tool call with `_meta.ui.resourceUri` creates a separate iframe** (1:1 mapping).
The spec does NOT address widget accumulation or consolidation.

Evidence from the specification:
- The lifecycle diagram shows "Host renders View in an iframe" for each tool call
- `McpUiHostContext.toolInfo` is singular (not an array) — each iframe knows its one tool
- `ontoolresult` is called exactly once per iframe lifecycle
- The `AppBridge` (host-side SDK) manages one view per instance

If an agent calls 5 tools with UI in one turn, the host creates 5 separate iframes.

### Consolidation mechanism: `callServerTool()` + `visibility: ["app"]`

The spec provides a pull-based pattern for consolidation:

- **`visibility: ["app"]`** — hides a tool from the model, making it callable only from within
  an app iframe
- **`app.callServerTool()`** — allows an app to call any server tool, including app-only tools

**Pattern from the system-monitor example:**
```
1. Model calls get-system-info (model-visible, has _meta.ui) → creates widget
2. Widget calls poll-system-stats (app-only) via callServerTool() → gets live data
3. Widget renders a consolidated dashboard
```

### Impact assessment for "app per tool"

With every tool having `_meta.ui.resourceUri`, typical agent behavior:
- 1-3 tool calls per turn → 1-3 inline widgets (acceptable UX)
- 5-10 tool calls per turn → 5-10 iframes (potentially noisy but each replaces text output)

**Mitigation strategies:**
- Keep widgets compact (controlled height via `sendSizeChanged()`)
- Use `IntersectionObserver` to pause off-screen widgets (performance)
- A widget can call `callServerTool()` to show related data (reducing need for separate calls)

### Display modes

Apps can request different display modes:
- **`inline`** — embedded in chat flow (hosts MUST support this)
- **`fullscreen`** — takes over window
- **`panel`** — side panel
- **`pip`** — picture-in-picture overlay

## 5. Frontend Stack Decision

### Decision: Preact + HTM + Chart.js (no build step)

| Library | Raw Size | Verdict |
|---------|----------|---------|
| `htm@3.1.1/preact/standalone.umd.js` | ~13 KB | Preact + HTM + Hooks in one UMD bundle |
| `chart.js@4.4.8/dist/chart.umd.min.js` | ~205 KB | Bar charts for metrics visualization |
| `prismjs@1.30.0` (core + YAML) | ~9.4 KB | YAML syntax highlighting (see [Section 13](#13-yaml-syntax-highlighting)) |
| MCP protocol (custom implementation) | ~2 KB | JSON-RPC over postMessage |
| **Total** | **~230 KB** | |

**Why Preact over Alpine.js:**
- Component model matches "data in, UI out" widget pattern
- Hooks (`useState`, `useEffect`, `useRef`, etc.) provide clean state management
- Better composability for complex UIs (tables, dashboards)
- Smaller total footprint
- React ecosystem knowledge transfers directly

**Why not HTMX:**
HTMX expects HTTP requests (`hx-get="/endpoint"`) and server-returned HTML fragments.
MCP Apps communicate via postMessage/JSON-RPC through the host. Fundamental architecture mismatch.

**Why implement MCP protocol directly (not SDK):**
- SDK is 314 KB (74 KB gzipped), mostly Zod schema validation
- The actual protocol is ~80 lines of vanilla JS
- Spec explicitly states the SDK is optional
- Spec includes a working inline implementation example
- No schema validation needed on the app side (host validates)

**Why Chart.js:**
- Metrics data (`pods_top`) benefits from visual bar charts for CPU and memory
- UMD build works in IIFE context without module system
- Responsive and theme-friendly (integrates with CSS variables)

### Single-file constraint

MCP `resources/read` returns HTML as a text string via JSON-RPC. The host renders it
in an iframe (via `srcdoc` or blob URL). Therefore:

- **Import maps with relative paths won't work** (no origin to resolve against)
- **All JS must be inline** in `<script>` tags
- **CDN imports blocked** by default CSP (`connect-src 'none'`)

### Multi-file source, single-file output

While the final HTML delivered to the host must be a single file with all JS/CSS inlined,
the **source code** is organized into separate files under `pkg/mcpapps/viewer/` for
maintainability. At startup, Go's `embed.FS` reads all files and assembles them via
placeholder replacement (`INJECT_*` → file contents), cached with `sync.Once`.

### Vendoring strategy

- **Makefile target** (`make vendor-js`) downloads minified files from jsdelivr CDN
- **Files committed to repo** — always available for airgapped builds
- **Makefile used for version bumps** — re-run when updating library versions

Actual Makefile target:

```makefile
HTM_VERSION ?= 3.1.1
CHART_JS_VERSION ?= 4.4.8
PRISM_VERSION ?= 1.30.0
MCP_APPS_VENDOR_DIR ?= pkg/mcpapps/vendor

.PHONY: vendor-js
vendor-js:
	@mkdir -p $(MCP_APPS_VENDOR_DIR)
	curl -sL "https://cdn.jsdelivr.net/npm/htm@$(HTM_VERSION)/preact/standalone.umd.js" \
		-o $(MCP_APPS_VENDOR_DIR)/htm-preact-standalone.umd.js
	curl -sL "https://cdn.jsdelivr.net/npm/chart.js@$(CHART_JS_VERSION)/dist/chart.umd.min.js" \
		-o $(MCP_APPS_VENDOR_DIR)/chart.umd.min.js
	curl -sL "https://cdn.jsdelivr.net/npm/prismjs@$(PRISM_VERSION)/components/prism-core.min.js" \
		-o $(MCP_APPS_VENDOR_DIR)/prism-core.min.js
	curl -sL "https://cdn.jsdelivr.net/npm/prismjs@$(PRISM_VERSION)/components/prism-yaml.min.js" \
		-o $(MCP_APPS_VENDOR_DIR)/prism-yaml.min.js
```

## 6. MCP postMessage Protocol

### Protocol overview

JSON-RPC 2.0 over `window.parent.postMessage()`. Implemented in `viewer/protocol.js`
(~80 lines) and exposed as `window.mcpProtocol` namespace.

### Essential messages

**Initialization handshake (required):**
1. App → Host: `ui/initialize` request (with appInfo, capabilities, protocolVersion)
2. Host → App: response with hostInfo, hostCapabilities, hostContext (includes toolInfo)
3. App → Host: `ui/notifications/initialized` notification

**Data flow (receive):**
- `ui/notifications/tool-input` — tool call arguments (sent once after init)
- `ui/notifications/tool-result` — tool execution result (sent once)
- `ui/notifications/host-context-changed` — theme changes
- `ui/resource-teardown` — cleanup before iframe destruction

**App-initiated (send):**
- `tools/call` — call a server tool (`callServerTool()`)
- `ui/notifications/size-changed` — resize notification
- `ping` — respond to health checks

### Implementation

The protocol is implemented in `viewer/protocol.js` as an IIFE that exposes
`window.mcpProtocol` with the following API:

```javascript
window.mcpProtocol = {
  initialize: initialize,       // → Promise<{hostContext, hostInfo, ...}>
  onNotification: onNotification, // (method, handler) → void
  onRequest: onRequest,         // (method, handler) → void
  sendRequest: sendRequest,     // (method, params) → Promise
  sendNotification: sendNotification // (method, params) → void
};
```

### Data available after initialization

```javascript
const { hostContext } = await initialize();
// hostContext.toolInfo.tool.name → which tool created this iframe
// hostContext.toolInfo.tool.inputSchema → tool's parameter schema
// hostContext.toolInfo.id → JSON-RPC request ID of the tools/call
// hostContext.theme → "dark" or "light"
// hostContext.styleVariables → CSS custom properties from host
```

## 7. Configuration: Opt-In Feature

MCP Apps is an **opt-in feature** controlled by configuration, disabled by default.

### Configuration flag

In `StaticConfig` (`pkg/config/config.go`):

```go
// AppsEnabled enables MCP Apps interactive UI extensions.
// When true, tools expose a _meta.ui.resourceUri field and the server
// registers the viewer HTML as a ui:// resource.
AppsEnabled bool `toml:"apps_enabled,omitempty"`
```

TOML configuration:
```toml
apps_enabled = true
```

CLI flag (`pkg/kubernetes-mcp-server/cmd/root.go`):
```
--apps    Enable MCP Apps interactive UI extensions
```

### What the flag controls

**When `apps_enabled = false` (default):**
- No `io.modelcontextprotocol/ui` extension in server capabilities
- `Resources` capability remains `nil` (unchanged from current behavior)
- No `ui://` resource registered
- Tools have no `_meta.ui` field (Meta stays nil)
- Behavior is identical to current server — zero overhead, zero side effects

**When `apps_enabled = true`:**
- Server declares `io.modelcontextprotocol/ui` extension via `AddExtension()`
- `Resources` capability enabled (`&mcp.ResourceCapabilities{}`)
- Per-tool `ui://kubernetes-mcp-server/tool/{toolName}` resources registered (one per enabled tool)
- All tools get `_meta.ui.resourceUri` pointing to their per-tool resource (via `WithAppsMeta()` mutator)
- Tools return `structuredContent` alongside text for UI consumption
- Array-typed `structuredContent` is wrapped in `{"items": [...]}` for MCP spec compliance

### Implementation details

The flag is checked at three points:

1. **Server capabilities** (`pkg/mcp/mcp.go`):
   ```go
   if configuration.AppsEnabled {
       caps.AddExtension("io.modelcontextprotocol/ui", nil)
       caps.Resources = &mcp.ResourceCapabilities{}
   }
   ```

2. **Per-tool resource registration** (`pkg/mcp/mcp.go` — `registerMCPAppResources(toolNames)`):
   Called during `reloadToolsets()` to register one `ui://` resource per enabled tool.
   Each resource returns assembled HTML with the tool name injected via
   `mcpapps.ViewerHTMLForTool(toolName)`.

3. **Tool Meta injection** (`pkg/mcp/tool_mutator.go` — `WithAppsMeta()`):
   A centralized `ToolMutator` that injects `_meta.ui` with a per-tool resource URI
   into every tool. Applied during tool registration, so **no changes to individual
   toolset files** are needed.
   ```go
   func WithAppsMeta() ToolMutator {
       return func(tool api.ServerTool) api.ServerTool {
           if tool.Tool.Meta == nil {
               tool.Tool.Meta = mcpapps.ToolMetaForTool(tool.Tool.Name)
           }
           return tool
       }
   }
   ```

### Configuration flow

The existing configuration loading chain supports this naturally:

```
BaseDefault() → config.toml → conf.d/*.toml → CLI flags
                (apps_enabled = true)           (--apps)
```

The flag is also compatible with dynamic reload via `SIGHUP` — when toggled at runtime,
the server can re-register tools with or without `_meta.ui` on the next `reloadToolsets()`.

### Interaction with other config options

- **`read_only = true`**: MCP Apps still works, showing only read-only tool results
- **`disable_destructive = true`**: MCP Apps works for non-destructive tools
- **`enabled_tools` / `disabled_tools`**: Only applicable tools get the UI; filtered
  tools are not affected
- **`stateless = true`**: Compatible — MCP Apps uses Resources (static), not tool
  list change notifications

## 8. Architecture

### High-level architecture

```
┌─────────────────────────────────────────────────────┐
│  MCP Host (Claude, VS Code, ChatGPT, etc.)          │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Sandboxed iframe (one per tool call)          │  │
│  │                                                │  │
│  │  viewer.html (per-tool, assembled at startup)  │  │
│  │  ├── Inline: htm/preact/standalone (~13 KB)    │  │
│  │  ├── Inline: Chart.js UMD         (~205 KB)    │  │
│  │  ├── Inline: Prism.js core+YAML   (~9.4 KB)   │  │
│  │  ├── Inline: protocol.js           (~2 KB)     │  │
│  │  ├── Inline: components.js         (~5 KB)     │  │
│  │  ├── Inline: window.__mcpToolName = 'X'        │  │
│  │  └── Inline: app.js               (~3 KB)      │  │
│  │      Dual flow:                                │  │
│  │        1. tool-result → render (spec-compliant)│  │
│  │        2. tool-input → call tool via           │  │
│  │           serverTools → render (fallback)      │  │
│  └────────────────────────────────────────────────┘  │
│                     ↕ postMessage (JSON-RPC 2.0)     │
└─────────────────────────────────────────────────────┘
                      ↕ MCP protocol
┌─────────────────────────────────────────────────────┐
│  kubernetes-mcp-server (Go binary)                   │
│                                                      │
│  ServerCapabilities:                                 │
│    extensions: {"io.modelcontextprotocol/ui": {}}    │
│    resources: {}                                     │
│                                                      │
│  Per-tool resources (registered in reloadToolsets):  │
│    uri: ui://kubernetes-mcp-server/tool/{toolName}  │
│    mimeType: text/html;profile=mcp-app               │
│    content: embed.FS → assembled per-tool            │
│                                                      │
│  Every tool (via WithAppsMeta mutator):              │
│    _meta.ui.resourceUri → per-tool resource URI      │
│    Handler returns text + structuredContent           │
│    (arrays wrapped in {"items": [...]} for spec)     │
└─────────────────────────────────────────────────────┘
```

### Package structure

```
pkg/mcpapps/
├── mcpapps.go          # embed.FS, ViewerHTMLForTool(), ToolMetaForTool(), ToolResourceURI()
├── mcpapps_test.go     # 19 tests: per-tool HTML, embed.FS, placeholders, constants
├── viewer/
│   ├── viewer.html     # HTML shell with INJECT_* placeholders (~38 lines)
│   ├── style.css       # All CSS + Chart.js container (~60 lines)
│   ├── protocol.js     # MCP postMessage protocol → window.mcpProtocol (~100 lines)
│   ├── components.js   # SortableTable, TableView, MetricsTable, GenericView → window.mcpComponents (~150 lines)
│   └── app.js          # App root + dual-flow + render/mount (~130 lines)
└── vendor/
    ├── htm-preact-standalone.umd.js   # ~13 KB (htm@3.1.1)
    ├── chart.umd.min.js              # ~205 KB (chart.js@4.4.8)
    ├── prism-core.min.js             # ~7.5 KB (prismjs@1.30.0)
    └── prism-yaml.min.js             # ~1.9 KB (prismjs@1.30.0)
```

### HTML assembly (mcpapps.go)

HTML assembly uses a two-stage process:

1. **Base HTML** (`buildBaseHTML`): reads all embedded files and performs 6 placeholder
   replacements (CSS, vendor libs, application scripts), cached via `sync.Once`.
2. **Per-tool HTML** (`ViewerHTMLForTool`): replaces `INJECT_TOOL_NAME` with the
   specific tool name, cached per-tool via `sync.Map`.

```go
//go:embed viewer vendor
var embeddedFS embed.FS

var (
    baseHTML      string
    buildOnce     sync.Once
    toolHTMLCache sync.Map // map[string]string — per-tool assembled HTML
)

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

func ViewerHTMLForTool(toolName string) string {
    if cached, ok := toolHTMLCache.Load(toolName); ok {
        return cached.(string)
    }
    html := strings.Replace(buildBaseHTML(), "INJECT_TOOL_NAME", toolName, 1)
    toolHTMLCache.Store(toolName, html)
    return html
}
```

Script load order in `viewer.html`: vendor libs first (htm-preact, Chart.js,
Prism core + YAML grammar), then protocol → components → tool name injection → app.

### JS namespace communication

The three application scripts communicate via window namespace objects:

- `viewer/protocol.js` → exposes `window.mcpProtocol`
- `viewer/components.js` → exposes `window.mcpComponents` (reads `window.htmPreact`)
- `viewer/app.js` → reads both `window.mcpProtocol` and `window.mcpComponents`

### Dual-flow viewer behavior

Each tool gets its own viewer HTML with `window.__mcpToolName` set to the tool name.
The viewer supports two data flows to handle different MCP host behaviors:

1. **Initializes**: Calls `mcpProtocol.initialize()`, receives `hostContext`.
   Prefers `hostContext.toolInfo.tool.name` if available, falls back to
   `window.__mcpToolName` (always available via per-tool resource URI).
2. **Registers handlers BEFORE init** to avoid timing windows:
   - `ui/notifications/tool-result` — spec-compliant flow
   - `ui/notifications/tool-input` — fallback flow
3. **Spec-compliant flow** (tool-result): Host calls the tool, then sends the result
   directly to the viewer. The viewer receives `structuredContent` and renders it.
4. **Fallback flow** (tool-input → serverTools): Host opens the viewer without calling
   the tool first (e.g., MCP Inspector "Apps" tab). The viewer receives `tool-input`,
   calls the tool itself via `protocol.sendRequest('tools/call', ...)`, and renders
   the result. This requires knowing the tool name (from `window.__mcpToolName`).
5. **Unwraps items envelope**: `structuredContent` is always a JSON object per spec.
   Arrays are wrapped in `{"items": [...]}` by the server. The viewer extracts
   `structured.items` when present.
6. **Routes to component**: Switch on data shape (no tool-name knowledge):
   - Self-describing metrics (`chart` + `columns` + `items[]`) → `MetricsTable` (Chart.js bar chart + sortable table)
   - Any structured array → `TableView` (generic sortable table)
   - Other structured data → `GenericView` (formatted JSON)
   - YAML text content → `YamlView` (Prism.js syntax-highlighted YAML)
   - Text fallback → `GenericView` (raw text)
7. **Supports theme**: CSS uses `var(--color-*, fallback)` for host integration

### Tool rendering matrix

Every registered tool gets a per-tool `ui://` resource URI and viewer HTML.
The viewer renders each tool's output using shape-based routing — no tool-name
knowledge required. The table below maps every tool to its viewer component and
current implementation status.

**Viewer components:**
- **Table** — `TableView` (sortable table from `[]map[string]any`)
- **Chart** — `MetricsTable` (Chart.js bar chart + sortable table from self-describing data)
- **YAML** — `YamlView` (Prism.js syntax-highlighted YAML)
- **Text** — `GenericView` (plain `<pre>` block)

#### Core toolset

| Tool | Description | Viewer | Status |
|------|------------|--------|--------|
| `pods_list` | List pods (all namespaces) | Table | Done |
| `pods_list_in_namespace` | List pods (single namespace) | Table | Done |
| `pods_get` | Get single pod | YAML | Pending |
| `pods_delete` | Delete a pod | Text | — |
| `pods_top` | Pod CPU/memory metrics | Chart | Done |
| `pods_exec` | Execute command in pod | Text | — |
| `pods_log` | Stream pod logs | Text | — |
| `pods_run` | Create and run a pod | YAML | Pending |
| `nodes_top` | Node CPU/memory metrics | Chart | Done |
| `nodes_log` | Stream node journal logs | Text | — |
| `nodes_stats_summary` | Node stats summary (JSON) | Text | — |
| `namespaces_list` | List namespaces | Table | Done |
| `projects_list` | List OpenShift projects | Table | Done |
| `events_list` | List cluster events | YAML | Pending |
| `resources_list` | List any resource kind | Table | Done |
| `resources_get` | Get single resource | YAML | Pending |
| `resources_create_or_update` | Create or update resource | YAML | Pending |
| `resources_delete` | Delete a resource | Text | — |
| `resources_scale` | Scale a resource | YAML | Pending |

#### Config toolset

| Tool | Description | Viewer | Status |
|------|------------|--------|--------|
| `configuration_view` | Show kubeconfig | YAML | Pending |
| `configuration_contexts_list` | List kubeconfig contexts | Text | — |
| `targets_list` | List configured targets | Text | — |

#### Helm toolset

| Tool | Description | Viewer | Status |
|------|------------|--------|--------|
| `helm_install` | Install a Helm chart | Text | — |
| `helm_list` | List Helm releases | Text | — |
| `helm_uninstall` | Uninstall a Helm release | Text | — |

#### KCP toolset

| Tool | Description | Viewer | Status |
|------|------------|--------|--------|
| `kcp_workspaces_list` | List KCP workspaces | Text | — |
| `kcp_workspace_describe` | Describe a KCP workspace | YAML | Pending |

#### Kiali toolset

| Tool | Description | Viewer | Status |
|------|------------|--------|--------|
| `kiali_mesh_graph` | Service mesh graph (JSON) | Text | — |
| `kiali_get_metrics` | Resource metrics (JSON) | Text | — |
| `kiali_get_traces` | Distributed traces (JSON) | Text | — |
| `kiali_get_resource_details` | Resource details (JSON) | Text | — |
| `kiali_workload_logs` | Workload pod logs | Text | — |
| `kiali_manage_istio_config_read` | Read Istio config (JSON) | Text | — |
| `kiali_manage_istio_config` | Manage Istio config (JSON) | Text | — |

#### KubeVirt toolset

| Tool | Description | Viewer | Status |
|------|------------|--------|--------|
| `vm_create` | Create a VirtualMachine | YAML | Pending |
| `vm_clone` | Clone a VirtualMachine | YAML | Pending |
| `vm_lifecycle` | Start/stop/restart a VM | YAML | Pending |

#### Summary

| Viewer | Tools | Done | Pending |
|--------|-------|------|---------|
| Table | 7 | 7 | 0 |
| Chart | 2 | 2 | 0 |
| YAML | 12 | 0 | 12 |
| Text | 12 | — | — |
| **Total** | **33** | **9** | **12** |

Text-output tools (logs, exec, delete confirmations, Helm output, Kiali JSON) don't
benefit from specialized rendering — `GenericView` is the correct component. The 12
pending YAML tools are the primary target for the Prism.js syntax highlighting work
described in [Section 13](#13-yaml-syntax-highlighting).

## 9. Output Layer

The output layer (`pkg/output/`) and tool result constructors (`pkg/api/toolsets.go`)
support generic structured content extraction without per-tool extraction functions.

### Extending the `Output` interface with `PrintObjStructured`

A method that returns both text and structured data from the same object:

```go
// PrintResult holds both the text representation and optional structured data
// extracted from a Kubernetes object.
type PrintResult struct {
    Text       string
    Structured any // nil when structured extraction is not applicable
}

type Output interface {
    GetName() string
    AsTable() bool
    PrintObj(obj runtime.Unstructured) (string, error)                // unchanged
    PrintObjStructured(obj runtime.Unstructured) (*PrintResult, error) // new
}
```

**YAML implementation** — the structured data is the cleaned-up object itself
(list items or single object, with managedFields stripped):

```go
func (p *yaml) PrintObjStructured(obj runtime.Unstructured) (*PrintResult, error) {
    text, err := p.PrintObj(obj)
    if err != nil {
        return nil, err
    }
    // Extract structured: for lists, return items as []map[string]any;
    // for single objects, return the object map
    switch t := obj.(type) {
    case *unstructured.UnstructuredList:
        items := make([]map[string]any, 0, len(t.Items))
        for _, item := range t.Items {
            items = append(items, item.Object)
        }
        return &PrintResult{Text: text, Structured: items}, nil
    case *unstructured.Unstructured:
        return &PrintResult{Text: text, Structured: t.Object}, nil
    }
    return &PrintResult{Text: text}, nil
}
```

**Table implementation** — the structured data is the table rows as maps
(column headers become keys):

```go
func (p *table) PrintObjStructured(obj runtime.Unstructured) (*PrintResult, error) {
    text, err := p.PrintObj(obj)
    if err != nil {
        return nil, err
    }
    // Extract structured data from Table response
    if obj.GetObjectKind().GroupVersionKind() == metav1.SchemeGroupVersion.WithKind("Table") {
        t := &metav1.Table{}
        if convErr := runtime.DefaultUnstructuredConverter.FromUnstructured(
            obj.UnstructuredContent(), t,
        ); convErr == nil {
            return &PrintResult{Text: text, Structured: tableToStructured(t)}, nil
        }
    }
    return &PrintResult{Text: text}, nil
}
```

The `tableToStructured` helper converts `metav1.Table` rows to `[]map[string]any`
using column definitions as keys — this is a **generic extraction** that works for
any Kubernetes resource type without per-resource custom code.

**Key advantage**: The Table format from the Kubernetes API already contains exactly
the fields that `kubectl get` would show (NAME, NAMESPACE, STATUS, AGE, etc.), which
are also the most relevant fields for the MCP Apps viewer. No per-tool extraction needed.

### `NewToolCallResultFull` constructor

A third constructor that accepts pre-formatted text alongside structured data:

```go
// NewToolCallResultFull creates a ToolCallResult with both human-readable text
// and structured content for MCP Apps UI rendering.
// The text is used for LLM consumption and backward-compatible MCP clients.
// The structured content is used by MCP Apps viewers.
func NewToolCallResultFull(text string, structured any, err error) *ToolCallResult {
    return &ToolCallResult{
        Content:           text,
        StructuredContent: structured,
        Error:             err,
    }
}
```

The three constructors express clear intent:

- `NewToolCallResult(text, err)` — text only (no UI data)
- `NewToolCallResultStructured(structured, err)` — structured only (text auto-generated as JSON)
- `NewToolCallResultFull(text, structured, err)` — both (text is human-readable, structured is for UI)

### Simplified tool handler pattern

With `PrintObjStructured` and `NewToolCallResultFull`, a typical list tool handler becomes:

```go
// Before (6 lines of result construction)
text, textErr := params.ListOutput.PrintObj(ret)
if structured := extractPodListStructured(ret); structured != nil {
    return &api.ToolCallResult{Content: text, StructuredContent: structured, Error: textErr}, nil
}
return api.NewToolCallResult(text, textErr), nil

// After (3 lines)
result, err := params.ListOutput.PrintObjStructured(ret)
if err != nil {
    return api.NewToolCallResult("", fmt.Errorf("...")), nil
}
return api.NewToolCallResultFull(result.Text, result.Structured, nil), nil
```

### Scope

The generic output layer handles standard Kubernetes list operations. **Non-Kubernetes-object
tools** (helm, events, pods_top, nodes_top) produce output from non-standard sources (Helm
client, metrics API). They continue using custom text formatting with optional structured
content via `NewToolCallResultFull`.

### Impact on viewer components

With `PrintObjStructured` providing generic structured data from the Kubernetes Table API,
the viewer implements a single **generic table component** that works for any list tool.
The Table API response already includes column headers and cell values, so the viewer
doesn't need to know the resource type — it just renders whatever columns the API provides.

This means:
- `TableView` is a **generic Kubernetes list table** (auto-discovers columns from data)
- `MetricsTable` is data-driven (reads columns/chart/items from self-describing structured content, no tool-specific knowledge)
- `GenericView` remains as fallback for non-table data

## 10. Per-Tool Resource URIs and Dual-Flow Viewer

Each tool gets its own `ui://` resource URI with the tool name embedded in the viewer HTML.
This enables a dual-flow viewer that works with both spec-compliant hosts (that send
`tool-result`) and hosts like MCP Inspector (that only send `tool-input` and expect
the viewer to call the tool).

### Design rationale

Some MCP hosts (e.g., MCP Inspector's "Apps" tab) open the viewer **without calling the
tool first** — they only send `ui/notifications/tool-input` with empty arguments, never
`tool-result`. In this scenario, the viewer needs to know which tool it belongs to so it
can call the tool via `serverTools`. Per-tool resource URIs solve this by embedding the
tool name (`window.__mcpToolName`) into each viewer's HTML.

### Per-tool resource URIs

Instead of a single shared `ui://kubernetes-mcp-server/viewer.html` resource, each tool
gets its own resource URI:

```
ui://kubernetes-mcp-server/tool/pods_list
ui://kubernetes-mcp-server/tool/namespaces_list
ui://kubernetes-mcp-server/tool/pods_top
...
```

Each resource returns the same base HTML but with `window.__mcpToolName` set to the
specific tool name. This gives the viewer the information it needs to call the tool
via `serverTools` when the host doesn't provide the result directly.

### Resource registration

Resources are registered dynamically in `reloadToolsets()` — one per enabled tool.
This ensures resources stay in sync with the current set of enabled tools (respecting
`enabled_tools`, `disabled_tools`, `read_only`, etc.).

### Dual-flow viewer

The viewer registers handlers for both flows **before** calling `initialize()` to
avoid timing windows:

1. **`tool-result` handler** (spec-compliant): Receives the result directly from
   the host and renders it.
2. **`tool-input` handler** (fallback): Receives tool input arguments, calls the
   tool via `protocol.sendRequest('tools/call', {name: toolName, arguments: args})`,
   and renders the result.

## 11. structuredContent Object Wrapping

The MCP specification requires `structuredContent` to marshal to a JSON **object** (record),
but the output layer produces `[]map[string]any` (arrays) for list operations. This requires
wrapping arrays before they reach the MCP protocol layer.

### Solution

`ensureStructuredObject()` in `pkg/mcp/mcp.go` uses `reflect` to detect slice/array values
and wraps them in `{"items": [...]}`:

```go
func ensureStructuredObject(v any) any {
    rv := reflect.ValueOf(v)
    if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
        return map[string]any{"items": v}
    }
    return v
}
```

Called from `NewStructuredResult()` before setting `result.StructuredContent`.

The viewer (`app.js`) unwraps the envelope:

```javascript
var raw = result.structuredContent;
var structured = (raw && raw.items && Array.isArray(raw.items)) ? raw.items : raw;
```

The unwrapping is selective: it only extracts `structured.items` when the wrapper object
has a single key (plain wrapper), preserving self-describing objects (like metrics data
that contain `chart`, `columns`, and `items` together).

## 12. Known Compatibility Notes

### VS Code: Resource Registration Ordering

MCP Apps resources must be registered **before** tools in `reloadToolsets()`. The go-sdk's
`AddTool()` calls `changeAndNotify()` which sends a `notifications/tools/list_changed`
notification to connected clients **immediately** after each tool is added.

VS Code eagerly pre-fetches MCP App resources: when it receives `tools/list_changed`, it calls
`tools/list`, reads `_meta.ui.resourceUri` from each tool, and immediately calls `resources/read`
to pre-load the UI HTML. If resources are registered after tools, VS Code's `resources/read`
request arrives before the resource exists, producing error `-32002` (Resource not found).

MCP Inspector does not pre-fetch resources this way — it only loads them on demand when the user
opens the "Apps" tab, by which time the resources are already registered.

### How VS Code Discovers MCP App Resources

Investigation of the [VS Code source code](https://github.com/microsoft/vscode/blob/5b38f1d5296b42ae51049d452362b673ff714dbe/src/vs/platform/mcp/common/modelContextProtocolApps.ts)
revealed the complete flow:

1. **`mcpServer.ts::_normalizeTool()`** — reads `_meta.ui.resourceUri` (nested key) from the
   tool definition and stores it on the tool object.
2. **`mcpLanguageModelToolContribution.ts::prepareToolInvocation()`** — passes the URI unchanged
   into `IMcpToolCallUIData`.
3. **`mcpToolCallUI.ts::loadResource()`** — calls `resources/read` with the URI **verbatim**
   (no transformation, encoding, or normalization).
4. **`mcpServerRequestHandler.ts`** — sends the `resources/read` MCP protocol request.

Key detail: VS Code reads the **nested** `_meta.ui.resourceUri` key, not the legacy flat
`_meta["ui/resourceUri"]` key.

### Registration order solution

Resources are registered before tools in `reloadToolsets()`:

```go
// Build tool name list from applicable tools
newToolNames := make([]string, 0, len(applicableTools))
for _, t := range applicableTools {
    newToolNames = append(newToolNames, t.Tool.Name)
}

// Register resources BEFORE tools so they're available when clients
// receive tools/list_changed and immediately try resources/read
if s.configuration.AppsEnabled {
    s.registerMCPAppResources(newToolNames)
}

// Now register tools (each AddTool sends tools/list_changed)
newTools, err := reloadItems(previousTools, applicableTools, ...)
```

### Legacy `_meta` key

`ToolMetaForTool()` sets both metadata formats for backward compatibility, matching what
the ext-apps SDK's `registerAppTool()` does:

```go
return map[string]any{
    "ui":             map[string]any{"resourceUri": uri},  // nested (VS Code reads this)
    "ui/resourceUri": uri,                                  // flat legacy key
}
```

### Key takeaway

When an MCP server registers tools with `_meta.ui.resourceUri`, the corresponding resources
**must** be registered before the tools. The go-sdk sends `tools/list_changed` notifications
synchronously within `AddTool()`, and eager clients like VS Code will immediately try to read
the referenced resources.

---

## 13. YAML Syntax Highlighting

### Problem

Tools that return single Kubernetes resources (`pods_get`, `resources_get`,
`resources_create_or_update`, `resources_scale`, etc.) produce YAML text output via
`output.MarshalYaml()`. These tools use `NewToolCallResult(text)` (text-only, no
`structuredContent`), so the viewer's rendering cascade falls through to `GenericView` —
a plain `<pre>` block with no highlighting.

For single-resource operations, YAML *is* the primary output format. Providing syntax
highlighting makes the output significantly more readable, especially for large resources
with deeply nested specs.

### Library choice: Prism.js

| Candidate | Core Size | YAML Grammar | Module Format | API | Verdict |
|-----------|-----------|-------------|---------------|-----|---------|
| **Prism.js 1.30.0** | 7.5 KB (minified) | 1.9 KB (minified) | IIFE (`window.Prism`) | **Sync**: `Prism.highlight()` | **Selected** |
| highlight.js 11.11.1 | 76 KB (not minified) | 5 KB (not minified) | CJS/ESM only | Sync but needs registration | Rejected — no pre-minified builds, 10× larger core |
| speed-highlight 1.2.14 | 23 KB (all 34 langs bundled) | 0.3 KB (minimal) | ESM only | Async (`Promise`) | Rejected — ESM-only, async API, monolithic bundle |

**Why Prism.js:**

1. **Pre-minified components** — `prism-core.min.js` (7.5 KB) + `prism-yaml.min.js` (1.9 KB)
   can be vendored directly, matching the existing no-build-step approach
2. **IIFE format** — loads via `<script>`, sets `window.Prism`, same pattern as HTM/Preact
3. **Synchronous API** — `Prism.highlight(code, Prism.languages.yaml, 'yaml')` returns an
   HTML string with `<span class="token ...">` wrappers, ideal for Preact rendering via
   `dangerouslySetInnerHTML`
4. **Comprehensive YAML grammar** — handles all YAML features: keys, scalars, multi-line
   block scalars, anchors (`*ref`, `&anchor`), tags (`!!str`), datetime literals,
   booleans, null, quoted strings, comments, directives (`%YAML`)
5. **Modest size impact** — +9.4 KB on top of existing ~220 KB (~4% increase)

### Vendoring

Two new files in `pkg/mcpapps/vendor/`:

```
vendor/
├── htm-preact-standalone.umd.js   # existing (~13 KB)
├── chart.umd.min.js               # existing (~205 KB)
├── prism-core.min.js              # new (~7.5 KB, prismjs@1.30.0)
└── prism-yaml.min.js              # new (~1.9 KB, prismjs@1.30.0)
```

Downloaded via `make vendor-js` from jsdelivr CDN (same mechanism as existing vendors).

**Important**: Prism auto-highlights all `<code>` elements on DOMContentLoaded by default.
This must be disabled by setting `window.Prism = { manual: true }` **before** loading the
Prism core script. The viewer controls highlighting explicitly via the `Prism.highlight()` API.

### Theming

Rather than vendoring a Prism theme file, the viewer provides a custom theme in `style.css`
using the existing `light-dark()` CSS function. This ensures YAML highlighting integrates
with the host's theme (dark/light mode) and CSS custom properties.

Prism token classes used by the YAML grammar:

| Token class | YAML element | Light color | Dark color |
|-------------|-------------|-------------|------------|
| `.token.atrule` | Keys (`apiVersion:`, `metadata:`) | `#07a` (blue) | `#cc99cd` (purple) |
| `.token.string` | Quoted strings | `#690` (green) | `#7ec699` (green) |
| `.token.number`, `.token.datetime` | Numbers, dates | `#905` (magenta) | `#f08d49` (orange) |
| `.token.important` | Booleans (`true`/`false`), null | `#e90` (amber) | `#f8c555` (yellow) |
| `.token.comment` | Comments (`# ...`) | `#708090` (gray) | `#999` (gray) |
| `.token.punctuation` | Structural chars (`:`, `-`, `|`, `>`) | `#999` | `#ccc` |
| `.token.tag` | Tags (`!!str`, `!custom`) | `#905` | `#e2777a` (pink) |
| `.token.scalar` | Block scalars (`|`, `>` multi-line) | `#690` (green) | `#7ec699` (green) |

CSS implementation (added to `style.css`):

```css
/* Prism.js YAML syntax highlighting — theme via light-dark() */
.token.atrule { color: light-dark(#07a, #cc99cd); }
.token.string, .token.scalar { color: light-dark(#690, #7ec699); }
.token.number, .token.datetime { color: light-dark(#905, #f08d49); }
.token.important { color: light-dark(#e90, #f8c555); }
.token.comment { color: light-dark(#708090, #999); font-style: italic; }
.token.punctuation { color: light-dark(#999, #ccc); }
.token.tag { color: light-dark(#905, #e2777a); }
```

### YamlView component

A new `YamlView` component in `components.js` renders syntax-highlighted YAML:

```javascript
function YamlView(props) {
  var highlighted = useMemo(function() {
    if (!props.text || typeof Prism === 'undefined') return null;
    return Prism.highlight(props.text, Prism.languages.yaml, 'yaml');
  }, [props.text]);
  if (!highlighted) {
    return html`<${GenericView} text=${props.text} />`;
  }
  return html`<pre class="raw yaml"
    dangerouslySetInnerHTML=${{ __html: highlighted }} />`;
}
```

- Uses `Prism.highlight()` synchronously inside `useMemo` — no re-tokenization on re-render
- Falls back to `GenericView` if Prism is unavailable
- Reuses the existing `pre.raw` CSS class for consistent container styling

### YAML detection in app.js

The viewer needs to detect when text content is YAML and route to `YamlView` instead of
`GenericView`. YAML detection uses a simple heuristic on the text content:

```javascript
function looksLikeYaml(text) {
  if (!text) return false;
  // Some tools prepend "# comment\n" headers for the LLM — skip those before checking.
  var lines = text.trimStart().split('\n');
  for (var i = 0; i < lines.length && i < 5; i++) {
    var line = lines[i].trimStart();
    if (line === '' || line.charAt(0) === '#') continue;
    return /^(apiVersion:|kind:|metadata:|---)/.test(line);
  }
  return false;
}
```

This is deliberately conservative — it targets Kubernetes resource YAML specifically rather
than arbitrary YAML. The patterns `apiVersion:`, `kind:`, `metadata:`, and `---` cover all
standard Kubernetes resource output from `MarshalYaml()`.

Several tools prepend a `# ` YAML comment header before the YAML body (e.g.,
`# The following resources (YAML) have been created or updated successfully`).
The detection skips up to 5 leading blank/comment lines before checking for
Kubernetes YAML markers. Prism.js handles the `#` comment lines natively with
proper syntax highlighting.

Updated routing in `app.js`:

```javascript
// Route to the appropriate view based on data shape
if (structured && structured.chart && structured.columns && Array.isArray(structured.items)) {
  return html`<${components.MetricsTable} data=${structured} />`;
}
if (structured && Array.isArray(structured) && structured.length > 0 && typeof structured[0] === 'object') {
  return html`<${components.TableView} data=${structured} />`;
}
if (structured) {
  return html`<${components.GenericView} text=${JSON.stringify(structured, null, 2)} />`;
}
// Text-only results: detect YAML for syntax highlighting
if (looksLikeYaml(textContent)) {
  return html`<${components.YamlView} text=${textContent} />`;
}
return html`<${components.GenericView} text=${textContent} />`;
```

### HTML assembly changes

Two new `INJECT_*` placeholders in `viewer.html` for Prism scripts, loaded after Chart.js
and before `protocol.js`. A `window.Prism = { manual: true }` assignment must appear
**before** the Prism core script to prevent auto-highlighting:

```html
<!-- Vendor: Prism.js (YAML syntax highlighting) -->
<script>window.Prism = { manual: true };</script>
<script>INJECT_VENDOR_PRISM_CORE</script>
<script>INJECT_VENDOR_PRISM_YAML</script>
```

---

## Appendix A: Research and Development Journal

> **Note**: This appendix contains research context, implementation status tracking, development
> history, and other material from the original research document. It is preserved here for
> reference during PR review and can be removed once the spec and implementation are finalized.

### A.1 PoC Reference (openshift-mcp-server PR #143)

[PR #143](https://github.com/openshift/openshift-mcp-server/pull/143) is a rough PoC
implementing a Pods Top dashboard with MCP Apps.

#### What it does

- Adds a `mcp-app/` directory with a Vite + TypeScript frontend build
- Uses `vite-plugin-singlefile` to produce a single HTML with all JS/CSS inlined
- Bundles the HTML into Go binary via `//go:embed`
- Uses `@modelcontextprotocol/ext-apps` SDK (v1.0.0) for host communication
- Implements column sorting, refresh via `app.callServerTool()`, dark/light theme support

#### Architecture (3 layers)

1. **Go embed package** (`mcp-app/mcpapp.go`):
   - `//go:embed dist/pods-top-app.html`
   - `Resources()` returns a registry of all app resources
   - `ToolMeta(resourceURI)` helper generates `_meta` map
   - MIME type: `text/html;profile=mcp-app`
   - Resource URI: `ui://kubernetes-mcp-server/pods-top.html`

2. **MCP resource registration** (`pkg/mcp/mcp.go`):
   - Iterates `mcpapp.Resources()` and registers each as an MCP resource
   - Enables `Resources` capability (was `nil` before)

3. **Tool linkage** (`pkg/toolsets/core/pods.go`):
   - `pods_top` tool gets `Meta: mcpapp.ToolMeta(mcpapp.PodsTopResourceURI)`
   - Tool handler returns both text and `structuredContent`

#### Concerns noted

- **Build step required**: `npm install && vite build` as dependency of `make build`
- **Committed build artifacts**: `dist/pods-top-app.html` in git (should be .gitignored)
- **innerHTML without sanitization**: XSS risk if pod names contain HTML-special characters
- **SDK size**: `@modelcontextprotocol/ext-apps` is 314 KB (mostly Zod validation)
- **Tight Go/TypeScript coupling**: Matching struct/interface with no shared schema

#### Patterns reused in our implementation

- `//go:embed` for bundling HTML into Go binary
- `ToolMeta()` helper for consistent `_meta` structure
- Dual content return (text + structured) for backward compatibility
- Theme support via CSS custom property fallbacks

#### Patterns intentionally avoided

- **No build step**: We inline JS directly, no Vite/npm required
- **No ext-apps SDK**: Custom ~80-line protocol implementation instead of 314 KB SDK
- **Per-tool resource URIs** (not per-tool HTML files): Single generic viewer template,
  but each tool gets its own `ui://` resource URI with the tool name injected into the HTML
  via `window.__mcpToolName`. This enables the dual-flow viewer (see [Section 10](#10-per-tool-resource-uris-and-dual-flow-viewer)).

### A.2 Implementation Status

#### Completed — Phase 1: Infrastructure + Configuration

| Feature | Location | Status |
|---------|----------|--------|
| `AppsEnabled bool` config field | `pkg/config/config.go` | Done |
| `IsAppsEnabled()` getter | `pkg/config/config.go` | Done |
| `--apps` CLI flag | `pkg/kubernetes-mcp-server/cmd/root.go` | Done |
| Extension registration | `pkg/mcp/mcp.go` | Done — `caps.AddExtension("io.modelcontextprotocol/ui", nil)` |
| Resources capability | `pkg/mcp/mcp.go` | Done — `caps.Resources = &mcp.ResourceCapabilities{}` |
| Vendor JS files | `pkg/mcpapps/vendor/` | Done — htm-preact + Chart.js committed |
| Makefile `vendor-js` target | `Makefile` | Done — downloads htm@3.1.1 + chart.js@4.4.8 |

#### Completed — Phase 2: Viewer + Pod Tools

| Feature | Location | Status |
|---------|----------|--------|
| MCP postMessage protocol | `viewer/protocol.js` (~80 lines) | Done — exposes `window.mcpProtocol` |
| Preact components | `viewer/components.js` (~150 lines) | Done — SortableTable, TableView, MetricsTable (data-driven), GenericView |
| App root + routing | `viewer/app.js` (~80 lines) | Done — dual-flow: spec-compliant `tool-result` + fallback `tool-input` → `serverTools` |
| CSS with theme fallbacks | `viewer/style.css` (~60 lines) | Done — includes `.chart-container` |
| HTML shell + assembly | `viewer/viewer.html` (~38 lines) | Done — `INJECT_*` placeholders including `INJECT_TOOL_NAME` |
| Chart.js bar chart | `viewer/components.js` — `MetricsTable` | Done — data-driven: reads columns, chart config, items from self-describing structured content |
| `pods_top` structured content | `pkg/toolsets/core/pods.go` | Done — `extractPodsTopStructured()` returns self-describing `map[string]any` with columns, chart, items |

#### Completed — Phase 3: Output Layer + Per-Tool URIs + All List Tools

| Feature | Location | Status |
|---------|----------|--------|
| Per-tool resource URIs | `pkg/mcpapps/mcpapps.go` | Done — `ViewerHTMLForTool(toolName)`, `ToolMetaForTool(toolName)`, `ToolResourceURI(toolName)` |
| Per-tool resource registration | `pkg/mcp/mcp.go` — `registerMCPAppResources(toolNames)` | Done — called from `reloadToolsets()`, one `ui://` resource per enabled tool |
| Centralized Meta injection | `pkg/mcp/tool_mutator.go` — `WithAppsMeta()` | Done — injects per-tool `_meta.ui.resourceUri` |
| `PrintObjStructured` on Output interface | `pkg/output/output.go` | Done — generic structured extraction for both YAML and Table output |
| `PrintResult` struct | `pkg/output/output.go` | Done — holds `Text` + `Structured` |
| `tableToStructured` helper | `pkg/output/output.go` | Done — converts `metav1.Table` → `[]map[string]any` using column definitions |
| `NewToolCallResultFull` constructor | `pkg/api/toolsets.go` | Done — `(text, structured, err)` for explicit text + structured |
| `structuredContent` object wrapping | `pkg/mcp/mcp.go` — `ensureStructuredObject()` | Done — wraps slice/array in `{"items": [...]}` for MCP spec compliance |
| Viewer unwraps `items` envelope | `viewer/app.js` | Done — extracts `structured.items` for plain wrappers only (preserves self-describing objects) |
| `namespaces_list` structured content | `pkg/toolsets/core/namespaces.go` | Done — uses `PrintObjStructured` + `NewToolCallResultFull` |
| `projects_list` structured content | `pkg/toolsets/core/namespaces.go` | Done — same pattern |
| `pods_list` / `pods_list_in_namespace` structured content | `pkg/toolsets/core/pods.go` | Done — uses `PrintObjStructured`, removed `extractPodListStructured` |
| `resources_list` structured content | `pkg/toolsets/core/resources.go` | Done — uses `PrintObjStructured` + `NewToolCallResultFull` |
| Hooks violation fix | `viewer/components.js` — `TableView` | Done — moved `useMemo` before guard clause |
| Typed nil interface fix | `pkg/output/output.go` — `PrintObjStructured` | Done — explicit nil check before assigning to `any` |
| Tests | `pkg/mcpapps/mcpapps_test.go` | Done — 19 tests covering per-tool HTML, embeds, placeholders, constants |
| Tests | `pkg/mcp/text_result_test.go` | Done — tests for array wrapping and map pass-through |
| Tests | `pkg/output/output_test.go` | Done — tests for `PrintObjStructured` (YAML and Table), `tableToStructured` |
| Tests | `pkg/api/toolsets_test.go` | Done — tests for `NewToolCallResultFull` |

#### Completed — Phase 3.5: Self-Describing Metrics Structured Content

| Feature | Location | Status |
|---------|----------|--------|
| Self-describing `pods_top` structured content | `pkg/toolsets/core/pods.go` | Done — `extractPodsTopStructured()` returns `map[string]any` with `columns`, `chart`, `items` |
| Self-describing `nodes_top` structured content | `pkg/toolsets/core/nodes.go` | Done — `extractNodesTopStructured()` with CPU/memory values and percentage utilization |
| Data-driven `MetricsTable` component | `viewer/components.js` | Done — reads columns, chart config, items from self-describing data; generic `parseUnit()` helper |
| Shape-based routing (replaces tool-name routing) | `viewer/app.js` | Done — detects `structured.chart && structured.columns && Array.isArray(structured.items)` |
| Selective `items` unwrapping | `viewer/app.js` | Done — only unwraps when `Object.keys(raw).length === 1` (plain wrapper) |
| Tests | `pkg/mcp/pods_top_test.go` | Done — structured content assertions for columns, chart, items |
| Tests | `pkg/mcp/nodes_top_test.go` | Done — structured content assertions for self-describing shape |

#### Completed — Phase 4: YAML Syntax Highlighting

| Feature | Location | Status |
|---------|----------|--------|
| Vendor Prism.js core + YAML grammar | `pkg/mcpapps/vendor/prism-core.min.js`, `prism-yaml.min.js` | Done — `make vendor-js` downloads prismjs@1.30.0 |
| Prism.js theme CSS (`light-dark()`) | `viewer/style.css` | Done — `.token.atrule`, `.token.string`, etc. with `light-dark()` fallbacks |
| `YamlView` component | `viewer/components.js` | Done — `Prism.highlight()` + `dangerouslySetInnerHTML` with `useMemo` |
| YAML detection + routing | `viewer/app.js` | Done — `looksLikeYaml()` heuristic + route to `YamlView` |
| HTML assembly (`viewer.html`) | `viewer/viewer.html` | Done — `INJECT_VENDOR_PRISM_CORE/YAML` placeholders, `window.Prism = { manual: true }` |
| Embed + assembly (`mcpapps.go`) | `pkg/mcpapps/mcpapps.go` | Done — Prism placeholder replacements in `buildBaseHTML()` |
| Tests for Prism embed/placeholder | `pkg/mcpapps/mcpapps_test.go` | Done — tests for Prism placeholders and vendor file embedding |

#### Pending — Phase 5: Polish

| Feature | Status |
|---------|--------|
| Structured content for remaining non-list tool handlers | Not started |
| Refresh via `callServerTool()` | Not started |
| Compact height management via `sendSizeChanged()` | Not started |
| Filtering and pagination for table views | Not started |
| Edge case handling (empty results, large datasets) | Not started |
| Documentation: `docs/configuration.md` update for `apps_enabled` | Not started |

#### Pre-existing infrastructure (unchanged)

| Feature | Location |
|---------|----------|
| `Tool.Meta map[string]any` | `pkg/api/toolsets.go` |
| Meta → go-sdk conversion | `pkg/mcp/gosdk.go` — `Meta: mcp.Meta(tool.Tool.Meta)` |
| `ToolCallResult.StructuredContent` | `pkg/api/toolsets.go` |
| `NewToolCallResultStructured()` | `pkg/api/toolsets.go` |
| `NewStructuredResult()` | `pkg/mcp/mcp.go` |
| Go-SDK v1.4.0 with Extensions | `go.mod` |

### A.3 Commit History

1. `989144b` — feat: add MCP Apps scaffolding with research, config, and vendor libs (Phase 1)
2. `4093abb` — feat: add MCP Apps viewer with Preact rendering and Chart.js (Phase 2)
3. `c3b2f48` — feat: add per-tool resource URIs, self-describing metrics, and dual-flow viewer (Phase 3)
4. `ce472e8` — fix: register MCP App resources before tools for VS Code compatibility
5. `518c3bf` — test: ensure no error when writing test harness and viewer HTML
6. `eadbcf8` — chore(docs): move MCP Apps research to specs as feature specification
7. `01d45c3` — docs(specs): add YAML syntax highlighting spec and tool rendering matrix
8. `e3e37da` — feat: add YAML syntax highlighting with Prism.js for MCP Apps viewer (Phase 4)

### A.4 End-to-End Verification

The feature has been verified working end-to-end with MCP Inspector: with `--apps` enabled,
tools expose per-tool `ui://kubernetes-mcp-server/tool/{toolName}` resources. The viewer
renders in a sandboxed iframe and supports both the spec-compliant flow (receiving `tool-result`
from the host) and the fallback flow (calling the tool via `serverTools` when only `tool-input`
is received). Array-typed `structuredContent` is correctly wrapped in `{"items": [...]}` to
satisfy the MCP spec requirement that `structuredContent` must be a JSON object.

### A.5 Output Layer Problem Analysis

#### Data flow before refinement (pods_list example)

```
K8s API → runtime.Unstructured
    ├── PrintObj(ret) → text string (YAML or Table)     ← for LLM/Content
    └── extractPodListStructured(ret) → []map[string]any ← for MCP Apps/StructuredContent
                                                            (manually walks same object again)
```

The handler then manually assembled the result:
```go
text, textErr := params.ListOutput.PrintObj(ret)
if structured := extractPodListStructured(ret); structured != nil {
    return &api.ToolCallResult{Content: text, StructuredContent: structured, Error: textErr}, nil
}
return api.NewToolCallResult(text, textErr), nil
```

#### Three distinct problems solved

1. **`PrintObj` returns text only** — `PrintObj(obj runtime.Unstructured) (string, error)`.
   The Kubernetes object is already being processed (YAML marshal or Table extraction),
   but the structured data is thrown away and must be re-extracted separately.

2. **No constructor for "text + structured"** — `NewToolCallResult` is text-only,
   `NewToolCallResultStructured` is structured-only (auto-serializes structured to JSON
   for Content, losing human-readable formatting). The pods handlers work around this
   by constructing `ToolCallResult` literally.

3. **Per-tool extraction functions don't scale** — `extractPodListStructured()` and
   `extractPodsTopStructured()` manually walk the Kubernetes object to pluck specific
   fields. Every tool would need its own extraction function.

#### What the refinement eliminated

- **All `extract*Structured()` functions** for standard Kubernetes list operations —
  `extractPodListStructured()` removed. The `Output` implementation handles extraction generically.

- **Manual `ToolCallResult` literal construction** — replaced by `NewToolCallResultFull`.

- **Per-tool structured content logic in toolset files** — the output layer handles it.

#### Tool patterns before and after

| Pattern | Tools | How they produce results |
|---------|-------|------------------------|
| Text-only via `PrintObj` | namespaces_list, projects_list, most list tools | `NewToolCallResult(params.ListOutput.PrintObj(ret))` |
| Text-only via custom formatting | events_list, helm_list | `NewToolCallResult(customFormat, nil)` |
| Text-only hardcoded | pods_get, pods_delete, pods_log | `NewToolCallResult("message", nil)` |
| Text + structured (manual) | pods_list, pods_list_in_namespace | `PrintObj` + `extractPodListStructured` + literal struct |
| Custom text + self-describing structured | pods_top, nodes_top | `TopCmdPrinter` + `extractPodsTopStructured`/`extractNodesTopStructured` → `map[string]any` with columns, chart, items |

### A.6 Files Modified Per Feature

#### Output layer refinement

| File | Changes |
|------|---------|
| `pkg/output/output.go` | Added `PrintResult` struct, `PrintObjStructured` to interface, implemented for yaml and table, added `tableToStructured` helper |
| `pkg/output/output_test.go` | Tests for `PrintObjStructured` (YAML and Table), `tableToStructured`, typed nil guard |
| `pkg/api/toolsets.go` | Added `NewToolCallResultFull(text, structured, err)` constructor |
| `pkg/api/toolsets_test.go` | Tests for `NewToolCallResultFull` |
| `pkg/toolsets/core/pods.go` | Simplified handlers to use `PrintObjStructured` + `NewToolCallResultFull`, removed `extractPodListStructured` |
| `pkg/toolsets/core/namespaces.go` | Switched from `PrintObj` → `PrintObjStructured` + `NewToolCallResultFull` |
| `pkg/toolsets/core/resources.go` | Switched from `PrintObj` → `PrintObjStructured` + `NewToolCallResultFull` |
| `pkg/mcpapps/viewer/components.js` | Generalized `TableView` to render any column set, fixed hooks violation |

#### Per-tool resource URIs

| File | Changes |
|------|---------|
| `pkg/mcpapps/mcpapps.go` | Replaced `ViewerHTML()` → `ViewerHTMLForTool(toolName)`, `ToolMeta()` → `ToolMetaForTool(toolName)`, added `ToolResourceURI(toolName)`, `sync.Map` for per-tool caching |
| `pkg/mcpapps/mcpapps_test.go` | Rewritten for per-tool API: 19 tests |
| `pkg/mcpapps/viewer/viewer.html` | Added `INJECT_TOOL_NAME` placeholder |
| `pkg/mcpapps/viewer/app.js` | Dual-flow: `tool-result` (spec) + `tool-input` → `serverTools` (fallback) |
| `pkg/mcpapps/viewer/protocol.js` | Added responses to unhandled requests, improved logging |
| `pkg/mcp/mcp.go` | Changed `registerMCPAppResources()` to per-tool, moved to `reloadToolsets()` |
| `pkg/mcp/tool_mutator.go` | Updated `WithAppsMeta()` to use `ToolMetaForTool(tool.Tool.Name)` |

#### structuredContent object wrapping

| File | Changes |
|------|---------|
| `pkg/mcp/mcp.go` | Added `ensureStructuredObject()`, called from `NewStructuredResult()` |
| `pkg/mcp/text_result_test.go` | Tests for array wrapping and map pass-through |
| `pkg/mcpapps/viewer/app.js` | Unwraps `{"items": [...]}` envelope |

#### VS Code compatibility fix

| File | Changes |
|------|---------|
| `pkg/mcp/mcp.go` | Moved `registerMCPAppResources()` before `reloadItems` for tools |
| `pkg/mcpapps/mcpapps.go` | Added legacy flat `"ui/resourceUri"` key to `ToolMetaForTool()` |
| `pkg/mcpapps/mcpapps_test.go` | Tests for both nested and legacy `_meta` keys |

### A.7 Frontend Browser Testing

Browser tests are **foundational infrastructure** — the test harness and first tests should be
set up alongside the initial viewer implementation, not deferred to a later phase. Each feature
(table rendering, metrics charts, theming, sorting) gets its browser tests written as part of
the same phase that implements the feature.

#### Why Browser Tests Are Needed

The MCP Apps viewer is a Preact-based SPA served as a self-contained HTML blob (~220KB with
vendored JS). It communicates with its parent frame via **postMessage JSON-RPC**, not HTTP.
Go unit tests (`mcpapps_test.go`) can verify HTML assembly and resource URIs, but they cannot
test rendering, component routing, user interactions, or the postMessage protocol flow. Only
a real browser can validate that the viewer works end-to-end.

#### Library Choice: Rod (go-rod/rod)

| | |
|---|---|
| **Repository** | [github.com/go-rod/rod](https://github.com/go-rod/rod) |
| **Stars** | ~6,700+ |
| **Actively maintained** | Yes (Feb 2026) |
| **Pure Go** | Yes (no Node.js, no Java) |
| **Browser** | Auto-downloads pinned Chromium version |

**Why Rod:**

1. **Auto-downloads the exact Chromium version** — no CI setup, no version mismatch.
   Each Rod release pins a specific Chromium + DevTools protocol version.
2. **Auto-wait on all interactions** — ideal for Preact async rendering + Chart.js canvas.
3. **Thread-safe** — works with Go test parallelism.
4. **`Must` prefix convention** maps naturally to test code (panics on error).
5. **First-class iframe support** — critical since MCP Apps run inside sandboxed iframes.
6. **Zero transitive Go dependencies** — only adds `github.com/go-rod/rod` to `go.mod`.
7. **CI-friendly** — headless by default, works on `ubuntu-latest` GitHub Actions out of the box.

```go
browser := rod.New().MustConnect()
page := browser.MustPage("file:///tmp/test-harness.html").MustWaitStable()
iframe := page.MustElement("iframe").MustFrame()
rows := iframe.MustElements("table tbody tr")
```

**Safe alternative: [chromedp](https://github.com/chromedp/chromedp)** (~11,500 stars) —
most popular Go browser library, battle-tested, pure Go with zero dependencies. Trade-offs:
more verbose DSL-like API, no auto-wait (manual `chromedp.WaitVisible` required), requires
system Chrome or Docker image (no auto-download).

#### Test Architecture: Wrapper Page with iframe

The viewer communicates via `window.parent.postMessage()` JSON-RPC. In production, the parent
is the MCP host (Claude Desktop, VS Code, etc.). In tests, we **simulate the MCP host** using
a wrapper HTML page that embeds the viewer in an `<iframe>` and handles the protocol.

A direct page load approach was considered (when there's no iframe, `window.parent === window`)
but rejected: the app initializes immediately on load, firing `ui/initialize` before a test
listener can be injected, creating a fragile race condition.

The wrapper iframe approach is better because:
- It replicates the actual iframe + postMessage flow as MCP hosts do it.
- No modifications to the production viewer HTML — used as-is via `srcdoc`.
- Go generates the harness HTML from a template — no extra files checked in.
- The `srcdoc` attribute avoids cross-origin issues (same-origin with parent).

#### Harness HTML (generated by Go test)

```html
<html><body>
<iframe id="viewer" srcdoc="...escaped viewer HTML..."></iframe>
<script>
// Act as MCP host: respond to ui/initialize
window.addEventListener('message', function(e) {
    var msg = e.data;
    if (!msg || msg.jsonrpc !== '2.0') return;
    if (msg.method === 'ui/initialize' && msg.id != null) {
        e.source.postMessage({
            jsonrpc: '2.0', id: msg.id,
            result: {
                hostContext: {
                    theme: window.__testTheme || 'light',
                    toolInfo: { tool: { name: window.__testToolName || 'test_tool' } }
                }
            }
        }, '*');
    }
});
// Expose function for Go test to inject tool results
window.sendToolResult = function(data) {
    document.getElementById('viewer').contentWindow.postMessage({
        jsonrpc: '2.0',
        method: 'ui/notifications/tool-result',
        params: data
    }, '*');
};
</script>
</body></html>
```

#### Test Flow

```
1. Go test calls mcpapps.ViewerHTMLForTool("pods_list")
2. Go test generates harness HTML with viewer embedded in <iframe srcdoc="...">
3. Harness written to temp file
4. Rod opens file:///tmp/harness-XXXX.html
5. Harness JS responds to ui/initialize from viewer
6. Viewer transitions to "ready" state ("Waiting for tool result...")
7. Go test calls page.MustEval("window.sendToolResult({...})")
8. Viewer renders the data (table, chart, generic view)
9. Go test navigates into iframe DOM and asserts on rendered elements
```

#### What to Test (incrementally, alongside each feature)

| Test Category | Phase |
|---|---|
| **Protocol handshake**: viewer initializes, transitions from "loading" to "ready" | Infrastructure setup |
| **Table rendering**: correct columns, row count, data values match input | Table component |
| **Table sorting**: click header → rows reorder, click again → reverse | Table component |
| **Metrics/Charts**: canvas element present, MetricsTable renders chart + table | Metrics component |
| **Data routing**: `{items:[...]}` → TableView, `{chart,columns,items}` → MetricsTable, text → GenericView | App routing |
| **Items unwrapping**: `{"items": [...]}` envelope unwrapped for plain arrays | structuredContent |
| **Theme application**: `data-theme` attribute set, `colorScheme` style matches | Theming |
| **Host context changes**: `host-context-changed` notification updates theme live | Theming |
| **Error handling**: error state renders error message in `.status` element | Error paths |

#### CI Considerations

- **Build tag**: Use `//go:build browser` to separate browser tests from unit tests.
  Browser tests are slower (launch Chromium) and should run in a dedicated CI step.
- **Rod auto-downloads Chromium**: No extra CI setup. Works on `ubuntu-latest`.
- **Timeouts**: Set reasonable timeouts (10-15s) for async rendering waits.
- **Viewport**: Use `rod.New().NoDefaultDevice()` for consistent viewport sizing.
- **Makefile target**: Add `make test-browser` that runs `go test -tags browser ./pkg/mcpapps/...`.
- **First run**: Rod downloads Chromium on first invocation (~150MB). Cache in CI via
  `~/.cache/rod` or equivalent.

#### Impact on go.mod

```
github.com/go-rod/rod v0.116.x  (zero transitive Go dependencies)
```

#### Decision

**Rod + Wrapper iframe approach**. Test harness infrastructure set up in Phase 1 alongside
the initial viewer. Tests added incrementally as each frontend feature is implemented.

### A.8 Why signals-core Was Removed

- Originally vendored (`@preact/signals-core@1.8.0`, ~4 KB) but **never used**
- The viewer exclusively uses Preact hooks (`useState`, `useEffect`, `useRef`, etc.)
- Its ESM `export{...}` statement caused a SyntaxError inside the IIFE wrapper
- Removed in Phase 2 restructuring
