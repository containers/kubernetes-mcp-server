# Using Kubernetes MCP Server with Claude Code CLI

This guide shows you how to configure the Kubernetes MCP Server with Claude Code CLI.

> **Prerequisites:** Complete the [Getting Started with Kubernetes](GETTING_STARTED_KUBERNETES.md) guide first to create a ServiceAccount and kubeconfig file.

## Quick Setup

Add the MCP server using the `claude mcp add-json` command:

```bash
claude mcp add-json kubernetes-mcp-server \
  '{"command":"npx","args":["-y","kubernetes-mcp-server@latest","--read-only"],"env":{"KUBECONFIG":"'${HOME}'/.kube/mcp-viewer.kubeconfig"}}' \
  -s user
```

**What this does:**
- Adds the Kubernetes MCP Server to your Claude Code configuration
- Uses `npx` to automatically download and run the latest version
- Enables read-only mode for safety
- Uses the kubeconfig file you created in the Getting Started guide
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
    "--read-only"
]

[mcp_servers.env]
KUBECONFIG = "/home/YOUR_USERNAME/.kube/mcp-viewer.kubeconfig"
```

**Important:** Replace `/home/YOUR_USERNAME/` with your actual home directory path.

## Verify Connection

After adding the MCP server, verify it's connected:

```bash
claude mcp list
```

Expected output:
```
Checking MCP server health...

kubernetes-mcp-server: npx -y kubernetes-mcp-server@latest --read-only - ✓ Connected
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

Common command-line flags you can add to the `args` array:

| Flag | Description | Example |
|------|-------------|---------|
| `--read-only` | Enable read-only mode (recommended) | Already included above |
| `--kubeconfig <path>` | Path to kubeconfig file | Use `KUBECONFIG` env var instead |
| `--context <name>` | Use specific context from kubeconfig | `"--context", "my-cluster"` |
| `--namespace <ns>` | Default namespace for operations | `"--namespace", "production"` |

**Example with custom namespace:**

```bash
claude mcp add-json kubernetes-mcp-server \
  '{"command":"npx","args":["-y","kubernetes-mcp-server@latest","--read-only","--namespace","production"],"env":{"KUBECONFIG":"'${HOME}'/.kube/mcp-viewer.kubeconfig"}}' \
  -s user
```

## Next Steps

- Review the [Getting Started with Kubernetes](GETTING_STARTED_KUBERNETES.md) guide for more details on ServiceAccount setup
- Explore the [main README](../README.md) for more MCP server capabilities
