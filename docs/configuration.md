# Configuration Reference

This document provides comprehensive reference documentation for configuring the Kubernetes MCP Server via TOML configuration files.

The server supports two configuration methods:
- **Command-line arguments** - For quick configuration and overrides
- **TOML configuration files** - For complex, persistent, and shareable configurations

This reference focuses on TOML file configuration. For CLI arguments, see the [Configuration Options](#cli-configuration-options) section or run `kubernetes-mcp-server --help`.

## Table of Contents

- [Configuration Loading](#configuration-loading)
- [Drop-in Configuration](#drop-in-configuration)
- [Dynamic Configuration Reload](#dynamic-configuration-reload)
- [Configuration Reference](#configuration-reference-1)
  - [Server Settings](#server-settings)
  - [Kubernetes Connection](#kubernetes-connection)
  - [Access Control](#access-control)
  - [Toolsets](#toolsets)
  - [Tool Filtering](#tool-filtering)
  - [Denied Resources](#denied-resources)
  - [Server Instructions](#server-instructions)
  - [Prompts](#prompts)
  - [OAuth and Authorization](#oauth-and-authorization)
  - [Telemetry](#telemetry)
  - [Toolset-Specific Configuration](#toolset-specific-configuration)
  - [Cluster Provider Configuration](#cluster-provider-configuration)
- [CLI Configuration Options](#cli-configuration-options)
- [Complete Example](#complete-example)

## Configuration Loading

Configuration values are loaded and merged in the following order (later sources override earlier ones):

1. **Internal Defaults** - Built-in default values
2. **Main Configuration File** - Loaded via `--config` flag
3. **Drop-in Files** - Loaded from `--config-dir` in lexical (alphabetical) order

### Usage

```bash
# Use a main configuration file
kubernetes-mcp-server --config /etc/kubernetes-mcp-server/config.toml

# Use only drop-in configuration files (no main config)
kubernetes-mcp-server --config-dir /etc/kubernetes-mcp-server/conf.d/

# Use both main config and explicit drop-in directory
kubernetes-mcp-server --config /etc/kubernetes-mcp-server/config.toml \
                      --config-dir /etc/kubernetes-mcp-server/config.d/
```

## Drop-in Configuration

Drop-in files allow you to split configuration into multiple files and override specific settings without modifying the main configuration file.

### How Drop-in Files Work

- **Default Directory**: If `--config-dir` is not specified, the server looks for drop-in files in `conf.d/` relative to the main config file's directory (when `--config` is provided)
- **File Naming**: Use numeric prefixes to control loading order (e.g., `00-base.toml`, `10-cluster.toml`, `99-override.toml`)
- **File Extension**: Only `.toml` files are processed; dotfiles (starting with `.`) are ignored
- **Partial Configuration**: Drop-in files can contain only a subset of configuration options
- **Merge Behavior**: Values present in a drop-in file override previous values; missing values are preserved

### Example Directory Structure

```
/etc/kubernetes-mcp-server/
├── config.toml              # Main configuration
└── conf.d/                  # Default drop-in directory
    ├── 00-base.toml         # Base overrides
    ├── 10-toolsets.toml     # Toolset-specific config
    └── 99-local.toml        # Local overrides (highest priority)
```

### Example Drop-in Files

**`10-toolsets.toml`** - Override only the toolsets:
```toml
toolsets = ["core", "config", "helm", "kubevirt"]
```

**`99-local.toml`** - Local development overrides:
```toml
log_level = 9
read_only = true
```

## Dynamic Configuration Reload

Configuration can be reloaded at runtime by sending a `SIGHUP` signal to the running server process.

**Prerequisite**: SIGHUP reload requires the server to be started with either the `--config` flag or `--config-dir` flag (or both). If neither is specified, SIGHUP signals are ignored.

### How to Reload

```bash
# Find the process ID
ps aux | grep kubernetes-mcp-server

# Send SIGHUP to reload configuration
kill -HUP <pid>

# Or use pkill
pkill -HUP kubernetes-mcp-server
```

### What Gets Reloaded

The server will:
- Reload the main config file and all drop-in files
- Update configuration values (log level, output format, etc.)
- Rebuild the toolset registry with new tool configurations
- Log the reload status

### Limitations

- **Requires restart**: `kubeconfig` or cluster-related settings
- **Not available on Windows**: Restart the server to reload configuration

## Configuration Reference

### Server Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `log_level` | integer | `0` | Logging verbosity level (0-9). Higher values produce more verbose output. Similar to [kubectl logging levels](https://kubernetes.io/docs/reference/kubectl/quick-reference/#kubectl-output-verbosity-and-debugging). |
| `port` | string | `""` | When set, starts the MCP server in HTTP mode (Streamable HTTP at `/mcp`, SSE at `/sse`) on the specified port. |
| `sse_base_url` | string | `""` | Base URL for Server-Sent Events (SSE) connections. Used when the server is behind a reverse proxy. |
| `list_output` | string | `"table"` | Output format for resource list operations. Valid values: `yaml`, `table`. |
| `stateless` | boolean | `false` | When `true`, disables tool and prompt change notifications. Useful for container deployments, load balancing, and serverless environments. |

**Example:**
```toml
log_level = 2
port = "8080"
list_output = "yaml"
stateless = true
```

### Kubernetes Connection

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `kubeconfig` | string | `""` | Path to the Kubernetes configuration file. If not provided, the server uses the in-cluster configuration or the default kubeconfig location (`~/.kube/config`). |
| `cluster_provider_strategy` | string | auto-detect | How the server finds clusters. Valid values: `kubeconfig`, `in-cluster`, `kcp`, `disabled`. |

**Example:**
```toml
kubeconfig = "/home/user/.kube/config"
cluster_provider_strategy = "kubeconfig"
```

### Access Control

Control what operations the MCP server can perform on your Kubernetes cluster. These options help enforce the principle of least privilege, ensuring AI assistants only have the permissions they need for their intended tasks.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `read_only` | boolean | `false` | When `true`, only exposes tools annotated with `readOnlyHint=true`. Prevents any write operations on the cluster. |
| `disable_destructive` | boolean | `false` | When `true`, disables tools annotated with `destructiveHint=true` (delete, update operations). Has no effect when `read_only` is `true`. |

**Example:**
```toml
# Production-safe configuration
read_only = true

# Or allow writes but prevent deletions
disable_destructive = true
```

### Toolsets

Toolsets group related tools together. Enable only the toolsets you need to reduce context size and improve LLM tool selection accuracy.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `toolsets` | string[] | `["core", "config", "helm"]` | List of toolsets to enable. |

**Available Toolsets:**

<!-- AVAILABLE-TOOLSETS-START -->

| Toolset  | Description                                                                                                                                                          | Default |
|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------|
| config   | View and manage the current local Kubernetes configuration (kubeconfig)                                                                                              | ✓       |
| core     | Most common tools for Kubernetes management (Pods, Generic Resources, Events, etc.)                                                                                  | ✓       |
| helm     | Tools for managing Helm charts and releases                                                                                                                          | ✓       |
| kcp      | Manage kcp workspaces and multi-tenancy features                                                                                                                     |         |
| kiali    | Most common tools for managing Kiali, check the [Kiali documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/KIALI.md) for more details. |         |
| kubevirt | KubeVirt virtual machine management tools                                                                                                                            |         |

<!-- AVAILABLE-TOOLSETS-END -->

**Example:**
```toml
# Enable specific toolsets
toolsets = ["core", "config", "helm", "kubevirt"]
```

### Tool Filtering

Fine-grained control over individual tools within enabled toolsets.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled_tools` | string[] | `[]` | Allowlist of specific tools to enable. When set, only these tools are available. |
| `disabled_tools` | string[] | `[]` | Denylist of specific tools to disable. Applied after `enabled_tools`. |

**Example:**
```toml
# Only enable specific tools
enabled_tools = ["pods_list", "pods_get", "pods_log"]

# Or disable specific tools from enabled toolsets
disabled_tools = ["resources_delete", "pods_delete"]
```

### Denied Resources

Prevent access to specific Kubernetes resource types.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `denied_resources` | array | `[]` | List of GroupVersionKind objects that should not be accessible. |

**Example:**
```toml
# Deny access to Secrets and ConfigMaps
[[denied_resources]]
group = ""
version = "v1"
kind = "Secret"

[[denied_resources]]
group = ""
version = "v1"
kind = "ConfigMap"

# Deny access to RBAC resources for additional security
[[denied_resources]]
group = "rbac.authorization.k8s.io"
version = "v1"
kind = "Role"

[[denied_resources]]
group = "rbac.authorization.k8s.io"
version = "v1"
kind = "RoleBinding"

[[denied_resources]]
group = "rbac.authorization.k8s.io"
version = "v1"
kind = "ClusterRole"

[[denied_resources]]
group = "rbac.authorization.k8s.io"
version = "v1"
kind = "ClusterRoleBinding"
```

### Server Instructions

Provide hints to MCP clients (like Claude Code) about when to use this server's tools. Useful for clients that support **MCP Tool Search**.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `server_instructions` | string | `""` | Instructions for MCP clients on when to use this server. |

**Example:**
```toml
server_instructions = """
Use this server for Kubernetes and OpenShift cluster management tasks including:
- Pods: list, get details, logs, exec commands, delete
- Resources: get, list, create, update, delete any Kubernetes resource
- Namespaces and projects: list, create, switch context
- Nodes: list, view logs, get resource usage statistics
- Events: view cluster events for debugging
- Helm: install, upgrade, uninstall charts and releases
- KubeVirt: create and manage virtual machines
- Cluster config: view and switch kubeconfig contexts
"""
```

### Prompts

Define custom MCP prompts for workflow templates. See [prompts.md](prompts.md) for detailed documentation.

| Field | Type | Description |
|-------|------|-------------|
| `prompts` | array | List of prompt definitions. |

**Prompt Fields:**
- `name` (required): Unique identifier
- `title` (optional): Human-readable display name
- `description` (required): Brief explanation
- `arguments` (optional): List of parameters
- `messages` (required): Conversation template

**Example:**
```toml
[[prompts]]
name = "check-pod-logs"
title = "Check Pod Logs"
description = "Quick way to check pod logs"

[[prompts.arguments]]
name = "pod_name"
description = "Name of the pod"
required = true

[[prompts.arguments]]
name = "namespace"
description = "Namespace of the pod"
required = false

[[prompts.messages]]
role = "user"
content = "Show me the logs for pod {{pod_name}} in {{namespace}}"

[[prompts.messages]]
role = "assistant"
content = "I'll retrieve and analyze the logs for you."
```

### OAuth and Authorization

Configure OAuth/OIDC authentication for HTTP mode deployments.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `require_oauth` | boolean | `false` | When `true`, requires OAuth authentication for all requests. |
| `oauth_audience` | string | `""` | Valid audience for OAuth tokens (for offline JWT claim validation). |
| `authorization_url` | string | `""` | URL of the OIDC authorization server for token validation and STS exchange. |
| `disable_dynamic_client_registration` | boolean | `false` | When `true`, disables dynamic client registration in `.well-known` endpoints. |
| `oauth_scopes` | string[] | `[]` | Supported client scopes for the OAuth flow. |
| `sts_client_id` | string | `""` | OAuth client ID for backend token exchange. |
| `sts_client_secret` | string | `""` | OAuth client secret for backend token exchange. |
| `sts_audience` | string | `""` | Audience for STS token exchange. |
| `sts_scopes` | string[] | `[]` | Scopes for STS token exchange. |
| `certificate_authority` | string | `""` | Path to CA certificate for validating authorization server connections. |
| `server_url` | string | `""` | Public URL of the MCP server (used for OAuth metadata). |

**Example:**
```toml
require_oauth = true
authorization_url = "https://keycloak.example.com/realms/mcp"
oauth_audience = "kubernetes-mcp-server"
oauth_scopes = ["openid", "profile"]

sts_client_id = "mcp-backend"
sts_client_secret = "your-client-secret"
sts_audience = "kubernetes-api"
```

For a complete OIDC setup guide, see [KEYCLOAK_OIDC_SETUP.md](KEYCLOAK_OIDC_SETUP.md).

### Telemetry

Configure OpenTelemetry distributed tracing and metrics. See [OTEL.md](OTEL.md) for detailed documentation.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `telemetry.enabled` | boolean | auto | Explicitly enable/disable telemetry. Auto-enabled when `endpoint` is set. |
| `telemetry.endpoint` | string | `""` | OTLP endpoint URL (e.g., `http://localhost:4317`). Can be overridden by `OTEL_EXPORTER_OTLP_ENDPOINT`. |
| `telemetry.protocol` | string | `"grpc"` | OTLP protocol: `grpc` or `http/protobuf`. Can be overridden by `OTEL_EXPORTER_OTLP_PROTOCOL`. |
| `telemetry.traces_sampler` | string | `""` | Trace sampling strategy. Can be overridden by `OTEL_TRACES_SAMPLER`. |
| `telemetry.traces_sampler_arg` | float | - | Sampling ratio (0.0-1.0) for ratio-based samplers. Can be overridden by `OTEL_TRACES_SAMPLER_ARG`. |

**Available Samplers:**
- `always_on` - Sample all traces
- `always_off` - Disable tracing
- `traceidratio` - Sample a percentage of traces
- `parentbased_always_on` - Respect parent span, default to always_on
- `parentbased_always_off` - Respect parent span, default to always_off
- `parentbased_traceidratio` - Respect parent span, default to ratio

**Example:**
```toml
[telemetry]
endpoint = "http://localhost:4317"
traces_sampler = "traceidratio"
traces_sampler_arg = 0.1  # 10% sampling
```

### Toolset-Specific Configuration

Some toolsets accept additional configuration via the `toolset_configs` map.

| Field | Type | Description |
|-------|------|-------------|
| `toolset_configs` | map | Toolset-specific configuration sections. |

**Example (Kiali):**
```toml
[toolset_configs.kiali]
url = "https://kiali.example.com"
token = "your-kiali-token"
```

Refer to individual toolset documentation for available options:
- [Kiali Configuration](KIALI.md)

### Cluster Provider Configuration

Configure cluster provider-specific settings via the `cluster_provider_configs` map.

| Field | Type | Description |
|-------|------|-------------|
| `cluster_provider_configs` | map | Provider-specific configuration sections. |

**Example:**
```toml
[cluster_provider_configs.kcp]
# kcp-specific configuration
```

## CLI Configuration Options

The following options can be set via command-line arguments. CLI arguments override TOML configuration values.

| Option | Description |
|--------|-------------|
| `--port` | Start in HTTP mode on the specified port |
| `--log-level` | Logging verbosity (0-9) |
| `--config` | Path to main TOML configuration file |
| `--config-dir` | Path to drop-in configuration directory |
| `--kubeconfig` | Path to Kubernetes configuration file |
| `--list-output` | Output format for list operations (`yaml` or `table`) |
| `--read-only` | Enable read-only mode |
| `--disable-destructive` | Disable destructive operations |
| `--stateless` | Enable stateless mode (no notifications) |
| `--toolsets` | Comma-separated list of toolsets to enable |
| `--disable-multi-cluster` | Disable multi-cluster support |
| `--cluster-provider` | Cluster provider strategy (`kubeconfig`, `in-cluster`, `kcp`, `disabled`) |

## Complete Example

A comprehensive configuration file demonstrating all major options:

```toml
# Server settings
log_level = 2
port = "8080"
list_output = "table"
stateless = false

# Kubernetes connection
kubeconfig = "/home/user/.kube/config"
cluster_provider_strategy = "kubeconfig"

# Access control
read_only = false
disable_destructive = true

# Toolsets
toolsets = ["core", "config", "helm", "kubevirt"]

# Tool filtering
disabled_tools = ["resources_delete"]

# Denied resources
[[denied_resources]]
group = ""
version = "v1"
kind = "Secret"

# Server instructions for MCP clients
server_instructions = """
Use this server for Kubernetes cluster management including pods, deployments,
services, and Helm releases. This server is configured with read-only access
to Secrets.
"""

# Custom prompts
[[prompts]]
name = "debug-pod"
description = "Debug a failing pod"

[[prompts.arguments]]
name = "pod_name"
required = true

[[prompts.messages]]
role = "user"
content = "Help me debug the pod {{pod_name}}"

# Telemetry (OpenTelemetry)
[telemetry]
endpoint = "http://localhost:4317"
traces_sampler = "traceidratio"
traces_sampler_arg = 0.1

# Toolset-specific configuration
[toolset_configs.kiali]
url = "https://kiali.example.com"
```

## Related Documentation

- [prompts.md](prompts.md) - MCP Prompts configuration
- [OTEL.md](OTEL.md) - OpenTelemetry observability
- [KIALI.md](KIALI.md) - Kiali toolset configuration
- [KEYCLOAK_OIDC_SETUP.md](KEYCLOAK_OIDC_SETUP.md) - OAuth/OIDC setup guide
- [GETTING_STARTED_KUBERNETES.md](GETTING_STARTED_KUBERNETES.md) - Kubernetes setup guide
