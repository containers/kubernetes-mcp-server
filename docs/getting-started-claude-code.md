# Using Kubernetes MCP Server with Claude Code CLI

This guide shows you how to configure the Kubernetes MCP Server with Claude Code CLI.

> **Prerequisites:** Complete the [Getting Started with Kubernetes](getting-started-kubernetes.md) guide first to create a ServiceAccount and kubeconfig file.

## Quick Setup

Add the MCP server using the `claude mcp add-json` command:

Create a small TOML config (runtime flags such as `--read-only` are no longer accepted):

```bash
cat > "$HOME/.config/kubernetes-mcp-server.toml" <<'EOF'
read_only = true
EOF
```

Add the MCP server using the `claude mcp add-json` command:

```bash
claude mcp add-json kubernetes-mcp-server \
  '{"command":"npx","args":["-y","kubernetes-mcp-server@latest","--config","'"${HOME}"'/.config/kubernetes-mcp-server.toml"],"env":{"KUBECONFIG":"'${HOME}'/.kube/mcp-viewer.kubeconfig"}}' \
  -s user
```

**What this does:**
- Adds the Kubernetes MCP Server to your Claude Code configuration
- Uses `npx` to automatically download and run the latest version
- Enables read-only mode via TOML (`read_only = true`)
- Uses the kubeconfig file you created in the Getting Started guide (`KUBECONFIG`)
- `-s user` makes it available in all your projects

## Manual Configuration (Alternative)

If you prefer to edit the config file manually:

**Location:** `~/.config/claude-code/config.toml`

```toml
[[mcp_servers]]
name = "kubernetes-mcp-server"
command = "npx"
args = [
    "-y",
    "kubernetes-mcp-server@latest",
    "--config",
    "/home/YOUR_USERNAME/.config/kubernetes-mcp-server.toml"
]

[mcp_servers.env]
KUBECONFIG = "/home/YOUR_USERNAME/.kube/mcp-viewer.kubeconfig"
```

**Important:** Replace `/home/YOUR_USERNAME/` with your actual home directory path. Write `read_only = true` in `~/.config/kubernetes-mcp-server.toml`.

## Verify Connection

After adding the MCP server, verify it's connected:

```bash
claude mcp list
```

Expected output:
```
Checking MCP server health...

kubernetes-mcp-server: npx -y kubernetes-mcp-server@latest --config .../kubernetes-mcp-server.toml - ✓ Connected
```

## Using the MCP Server

Once connected, interact with your Kubernetes cluster using natural language! Claude will use the Kubernetes MCP Server
to fetch and display the relevant information from your cluster:

```bash
> List all namespaces in my cluster 

● I'll list all the namespaces in your Kubernetes cluster.
  ⎿  APIVERSION   KIND        NAME                 STATUS   AGE   LABELS
     v1           Namespace   default              Active   77m   kubernetes.io/metadata.name=default
     v1           Namespace   kube-node-lease      Active   77m   kubernetes.io/metadata.name=kube-node-lease
     … +4 lines (ctrl+o to expand)

● Your cluster has 6 namespaces:

  1. default - The default namespace for resources without a specified namespace
  2. kube-node-lease - Used for node heartbeat/lease objects
  3. kube-public - Publicly accessible namespace, typically for cluster information
  4. kube-system - System namespace for Kubernetes control plane components
  5. local-path-storage - Likely used for local storage provisioning
  6. mcp - Custom namespace (created ~75 minutes ago)

  All namespaces are in Active status and the cluster appears to be running for about 77 minutes.
```

## Configuration Options

CLI that remains: `--config` (and optionally `--config-dir`). Put `read_only`, `kubeconfig`, `server_instructions`, and other settings in the TOML file. `$KUBECONFIG` is still the client-go fallback when `kubeconfig` is empty. See the [Configuration Reference](configuration.md) and [Configuration Changes](configuration-changes.md).

## MCP Tool Search

Claude Code supports [**MCP Tool Search**](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool), a feature that dynamically loads tools based on relevance to your query. This helps when you have many MCP servers connected.

You can configure `server_instructions` to help Claude know when to use this server's tools. See the [Server Instructions](../README.md#server-instructions) section in the main README for details.

## Next Steps

- Review the [Getting Started with Kubernetes](getting-started-kubernetes.md) guide for more details on ServiceAccount setup
- Explore the [main README](../README.md) for more MCP server capabilities
