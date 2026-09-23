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
