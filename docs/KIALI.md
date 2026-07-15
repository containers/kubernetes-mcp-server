## Kiali integration

This server can expose Kiali tools so assistants can query mesh information (e.g., mesh status/graph).

### Enable the Kiali toolset

Enable the Kiali tools via the server TOML configuration file.

Config (TOML):

```toml
toolsets = ["core", "kiali"]

[toolset_configs.kiali]
url = "https://kiali.example" # Endpoint/route to reach Kiali console
# insecure = true  # optional: allow insecure TLS (not recommended in production)
# certificate_authority = "/path/to/ca.crt"  # File path to CA certificate
# When url is https and insecure is false, certificate_authority is required.
```

When the `kiali` toolset is enabled, a Kiali toolset configuration is required via `[toolset_configs.kiali]`. If missing or invalid, the server will refuse to start.

### How authentication works

- The server uses your existing Kubernetes credentials (from kubeconfig or in-cluster) to set a bearer token for Kiali calls.
- If you pass an HTTP Authorization header to the MCP HTTP endpoint, that is not required for Kiali; Kiali calls use the server's configured token.

### Multi-cluster support

Kiali can manage multiple Kubernetes clusters within an Istio service mesh. Most Kiali tools accept an optional `meshCluster` parameter to target a specific mesh cluster. When omitted, Kiali defaults to its home cluster (where Kiali is deployed).

Use `<toolset>_list_mesh_clusters` (e.g. `kiali_list_mesh_clusters`) to discover available mesh cluster names before calling other tools. The `name` field from that response is the only valid value for `meshCluster`.

Kiali tools and prompts are not cluster-aware: the MCP server does not inject a `context` parameter on them. Use `meshCluster` to select mesh scope. Core Kubernetes tools still use `context` when multi-cluster is enabled.

### Multicluster evaluations

Multicluster mcpchecker tasks live under `evals/tasks/kiali/multicluster/`. They exercise Kiali tools against a primary-remote Kind setup (`east` home cluster, `west` remote cluster).

**Requirements:** Kiali **master** (dev image) — MCP tools such as `list_clusters` are not yet in released Kiali tags. Use a local Kiali checkout on master with `ai/mcp/list_clusters/`.

```bash
# Setup (~15–30 min)
KIALI_SRC=~/dev/kiali_sources/kiali make setup-kiali-multicluster

# MCP server (terminal 1)
TOOLSETS=core,config,kiali MCP_CONFIG_DIR=dev/config/mcp-configs-multicluster make run-server

# Evals (terminal 2) — same model credentials as other mcpchecker evals
export MODEL_BASE_URL="https://api.openai.com/v1"
export MODEL_KEY="sk-..."
make run-evals-multicluster
```

Useful targets (all in `build/kiali.mk`):

| Target | Purpose |
|--------|---------|
| `setup-kiali-multicluster` | Create east/west Kind clusters, deploy Kiali, write `dev/config/mcp-configs-multicluster/kiali.toml` |
| `redeploy-kiali-multicluster-dev` | Rebuild Kiali dev image when `list_clusters` is missing |
| `run-evals-multicluster` | Run the 6 multicluster eval tasks |
| `kind-delete-multicluster` | Tear down east/west clusters |

Verify `list_clusters` before running evals:

```bash
KIALI_URL=$(grep url dev/config/mcp-configs-multicluster/kiali.toml | cut -d'"' -f2)
curl -sk -X POST "${KIALI_URL}api/chat/mcp/list_clusters" \
  -H 'Content-Type: application/json' -d '{"mcp_mode":true}' | jq .
```

### Troubleshooting

- Missing Kiali configuration when `kiali` toolset is enabled → set `[toolset_configs.kiali].url` in the config TOML.
- Invalid URL → ensure `[toolset_configs.kiali].url` is a valid `http(s)://host` URL.
- TLS certificate validation:
  - If `[toolset_configs.kiali].url` uses HTTPS and `[toolset_configs.kiali].insecure` is false, you must set `[toolset_configs.kiali].certificate_authority` with the path to the CA certificate file. Relative paths are resolved relative to the directory containing the config file.
  - For non-production environments you can set `[toolset_configs.kiali].insecure = true` to skip certificate verification.
- Multicluster eval: `Tool 'list_clusters' not found` → Kiali too old; run `KIALI_SRC=~/path/to/kiali make redeploy-kiali-multicluster-dev`
- Multicluster eval: agent returns empty output → check `MODEL_BASE_URL` / `MODEL_KEY` (same as `evals/openai-agent/agent.yaml`)
