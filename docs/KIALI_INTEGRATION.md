## Kiali integration

This server can expose Kiali tools so assistants can query mesh information (e.g., mesh status/graph).

### Enable the Kiali toolset

You can enable the Kiali tools via config or flags.

Config (TOML):

```toml
toolsets = ["core", "kiali"]

[kiali]
url = "https://kiali.example"
# insecure = true  # optional: allow insecure TLS
```

Flags:

```bash
kubernetes-mcp-server \
  --toolsets core,kiali \
  --kiali-url https://kiali.example \
  [--kiali-insecure]
```

When the `kiali` toolset is enabled, a Kiali URL is required. Without it, the server will refuse to start.

### How authentication works

- The server uses your existing Kubernetes credentials (from kubeconfig or in-cluster) to set a bearer token for Kiali calls.
- If you pass an HTTP Authorization header to the MCP HTTP endpoint, that is not required for Kiali; Kiali calls use the server's configured token.

### Available tools (initial)

- `mesh_status`: retrieves mesh components status from Kiali’s mesh graph endpoint.

### Troubleshooting

- Error: "kiali-url is required when kiali tools are enabled" → provide `--kiali-url` or set `[kiali].url` in the config TOML.
- TLS issues against Kiali → try `--kiali-insecure` or `[kiali].insecure = true` for non-production environments.


