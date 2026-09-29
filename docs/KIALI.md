## Kiali integration

This server can expose Kiali tools so assistants can query mesh information (e.g., mesh status/graph).

### Enable the Kiali toolset

Enable the Kiali tools via the server TOML configuration file.

Config (TOML):

```toml
toolsets = ["core", "kiali"]
experimental_enable_target_compatibility_tool_filters = true

[toolset_configs.kiali]
url = "https://kiali.example" # Endpoint/route to reach Kiali console
# insecure = true  # optional: allow insecure TLS (not recommended in production)
# certificate_authority = "/path/to/ca.crt"  # File path to CA certificate
# When url is https and insecure is false, certificate_authority is required.
```

Reachability checks and in-cluster URL discovery require `experimental_enable_target_compatibility_tool_filters = true` (default `false`). Without it, Kiali tools are always registered and no `/api/status` probe or CR-based discovery runs.

When that flag is enabled:

- If `url` is set, the server probes `GET {url}/api/status` and only exposes Kiali tools if that call succeeds.
- If `url` is unset and a Kiali CR is present, the server tries to discover the in-cluster Service URL (`http://<instance>.<namespace>.svc:<port>[/web_root]`), probes `/api/status`, and injects a working URL into the live config.

When the `kiali` toolset is enabled with an explicit HTTPS URL and `insecure = false`, `certificate_authority` is still required at config load time.

### How authentication works

- The server uses your existing Kubernetes credentials (from kubeconfig or in-cluster) to set a bearer token for Kiali calls.
- If you pass an HTTP Authorization header to the MCP HTTP endpoint, that is not required for Kiali; Kiali calls use the server's configured token.

### Multi-cluster support

Kiali can manage multiple Kubernetes clusters within an Istio service mesh. Most Kiali tools accept an optional `meshCluster` parameter to target a specific mesh cluster. When omitted, Kiali defaults to its home cluster (where Kiali is deployed).

Use `<toolset>_list_mesh_clusters` (e.g. `kiali_list_mesh_clusters`) to discover available mesh cluster names before calling other tools. The `name` field from that response is the only valid value for `meshCluster`.

Kiali tools and prompts are not cluster-aware: the MCP server does not inject a `context` parameter on them. Use `meshCluster` to select mesh scope. Core Kubernetes tools still use `context` when multi-cluster is enabled.

### Troubleshooting

- Kiali tools missing from `tools/list` → enable `experimental_enable_target_compatibility_tool_filters = true`, then set `[toolset_configs.kiali].url` or ensure a Kiali CR is installed so the in-cluster Service URL can be discovered.
- Invalid or unreachable URL → ensure `[toolset_configs.kiali].url` is a valid `http(s)://host` URL and that `GET {url}/api/status` succeeds from the MCP server.
- TLS certificate validation:
  - If `[toolset_configs.kiali].url` uses HTTPS and `[toolset_configs.kiali].insecure` is false, you must set `[toolset_configs.kiali].certificate_authority` with the path to the CA certificate file. Relative paths are resolved relative to the directory containing the config file.
  - For non-production environments you can set `[toolset_configs.kiali].insecure = true` to skip certificate verification.

