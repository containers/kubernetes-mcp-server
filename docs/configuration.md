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
    - [Cross-Cluster Access from a Pod](#cross-cluster-access-from-a-pod)
  - [Access Control](#access-control)
  - [Toolsets](#toolsets)
  - [Tool Filtering](#tool-filtering)
  - [Tool Overrides](#tool-overrides)
  - [Denied Resources](#denied-resources)
  - [Server Instructions](#server-instructions)
  - [Prompts](#prompts)
  - [OAuth and Authorization](#oauth-and-authorization)
  - [Telemetry](#telemetry)
  - [Validation](#validation)
  - [Confirmation Rules](#confirmation-rules)
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
| `tls_cert` | string | `""` | Path to TLS certificate file for HTTPS. When set along with `tls_key`, the server serves HTTPS instead of HTTP. |
| `tls_key` | string | `""` | Path to TLS private key file for HTTPS. Must be set together with `tls_cert`. |
| `require_tls` | boolean | `false` | When `true`, enforces TLS for all connections. Server refuses to start without TLS certificates, and outbound connections to non-HTTPS endpoints (e.g., Kiali) are rejected. |

**Example:**
```toml
log_level = 2
port = "8080"
list_output = "yaml"
stateless = true

# Enable TLS for HTTPS
tls_cert = "/etc/tls/tls.crt"
tls_key = "/etc/tls/tls.key"

# Enforce TLS for all connections (requires tls_cert and tls_key)
require_tls = true
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

#### Cross-Cluster Access from a Pod

When the MCP server runs inside a Kubernetes pod, it automatically detects the in-cluster environment and uses the `in-cluster` provider strategy to connect to the **local** cluster's API server.

If you need the server to connect to a **different** cluster instead, you must explicitly provide both `kubeconfig` and `cluster_provider_strategy`. This overrides the automatic in-cluster detection.

**Required configuration:**

```toml
kubeconfig = "/etc/kubernetes-mcp-server/external-kubeconfig"
cluster_provider_strategy = "kubeconfig"
```

Or via CLI flags:

```bash
kubernetes-mcp-server --kubeconfig /etc/kubernetes-mcp-server/external-kubeconfig --cluster-provider kubeconfig
```

> **Important:** Both settings are required. Setting `--cluster-provider kubeconfig` alone (without `--kubeconfig`) will fail because the server still detects the in-cluster environment. The explicit `--kubeconfig` path overrides this detection.

**Mounting the kubeconfig in a pod:**

To make an external kubeconfig available inside a pod, mount it from a Secret or ConfigMap:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: external-kubeconfig
  namespace: mcp
type: Opaque
data:
  kubeconfig: <base64-encoded-kubeconfig>
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernetes-mcp-server
  namespace: mcp
spec:
  template:
    spec:
      containers:
        - name: kubernetes-mcp-server
          args:
            - --kubeconfig
            - /etc/kubernetes-mcp-server/kubeconfig
            - --cluster-provider
            - kubeconfig
          volumeMounts:
            - name: external-kubeconfig
              mountPath: /etc/kubernetes-mcp-server
              readOnly: true
      volumes:
        - name: external-kubeconfig
          secret:
            secretName: external-kubeconfig
```

**Troubleshooting cross-cluster access:**

If the server starts but operations fail (e.g., `failed to list namespaces: unknown`), verify:

1. **Network connectivity** — The pod must be able to reach the external cluster's API server. Check network policies, firewalls, and DNS resolution.
2. **Kubeconfig validity** — Ensure the kubeconfig contains valid credentials (token, client certificate, etc.) and points to the correct API server address.
3. **Permissions** — The credentials in the kubeconfig must have sufficient RBAC permissions on the target cluster.
4. **TLS certificates** — If the external cluster uses a private CA, the CA certificate must be included in the kubeconfig or mounted separately.

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

| Toolset  | Description                                                                                                                                                                     | Default |
|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------|
| config   | View and manage the current local Kubernetes configuration (kubeconfig)                                                                                                         | ✓       |
| core     | Most common tools for Kubernetes management (Pods, Generic Resources, Events, etc.)                                                                                             | ✓       |
| helm     | Tools for managing Helm charts and releases                                                                                                                                     |         |
| kcp      | Manage kcp workspaces and multi-tenancy features                                                                                                                                |         |
| kiali    | Most common tools for managing Kiali, check the [Kiali documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/KIALI.md) for more details.            |         |
| kubevirt | KubeVirt virtual machine management tools, check the [KubeVirt documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/kubevirt.md) for more details. |         |

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

### Tool Overrides

Customize tool descriptions shown to MCP clients without modifying source code. This enables adding domain-specific guidance to tool descriptions (e.g., "Prefer using label selectors over listing all pods").

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tool_overrides` | map | `{}` | Map of tool name to override configuration. |

**Override Fields:**
- `description` (optional): Custom description for the tool. Empty strings are ignored.

Tools are keyed by their flat name (e.g., `pods_list`, `resources_get`), consistent with `enabled_tools` and `disabled_tools`.

**Example:**
```toml
[tool_overrides.pods_list]
description = "List pods in the cluster. Prefer using label selectors over listing all pods when the namespace has many workloads."

[tool_overrides.resources_get]
description = "Get a Kubernetes resource by name. Always specify the namespace explicitly rather than relying on the default."
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

### Validation

Pre-execution validation catches errors before they reach the Kubernetes API, providing clearer error messages for issues like typos in resource names, invalid fields, and missing permissions.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `validation_enabled` | boolean | `false` | When `true`, enables schema validation and RBAC pre-checks for all API requests. Resource existence is always checked regardless of this setting. |

When enabled, the validation layer runs at the HTTP RoundTripper level, intercepting all Kubernetes API calls (including those from plugins like Helm, KubeVirt, and Kiali). It performs:

- **Schema validation** — Validates resource manifests against the cluster's OpenAPI schema for create/update operations
- **RBAC pre-checks** — Verifies permissions using `SelfSubjectAccessReview` before attempting operations

Resource existence validation (catching typos like "Deploymnt" instead of "Deployment") runs as part of access control regardless of this setting.

For detailed information about the validation flow, error codes, and behavior, see the [validation specification](specs/validation.md).

**Example:**
```toml
validation_enabled = true
```

### Confirmation Rules

Prompt users for confirmation before dangerous actions. Rules operate at two levels:

- **Tool-level** — matches on tool name or `DestructiveHint` annotation. Fires once before the tool handler runs.
- **Kube-level** — matches on Kubernetes API verb, kind, group, version, name, or namespace. Fires per API call during handler execution.

When a client doesn't support elicitation, the `confirmation_fallback` determines behavior: `"allow"` proceeds silently (with a warning log), `"deny"` blocks the action. The default is `"allow"`.

If multiple rules match at the same level, their messages are merged into a single prompt.

| Field | Type | Level | Description |
|-------|------|-------|-------------|
| `confirmation_fallback` | string | global | Default fallback: `"allow"` or `"deny"` (default: `"allow"`) |
| `tool` | string | tool | Tool name to match (e.g. `"helm_uninstall"`) |
| `destructive` | boolean | tool | Match tools with `DestructiveHint` annotation |
| `verb` | string | kube | Kubernetes verb (`"get"`, `"delete"`, `"list"`, etc.) |
| `kind` | string | kube | Resource kind (`"Secret"`, `"Deployment"`, etc.) |
| `group` | string | kube | API group (`"apps"`, `""` for core, etc.) |
| `version` | string | kube | API version (`"v1"`, `"v1beta1"`, etc.) |
| `name` | string | kube | Resource name to match |
| `namespace` | string | kube | Namespace to match |
| `message` | string | both | Message shown in the confirmation prompt |

A rule must be either tool-level or kube-level. It must not mix tool-level fields (`tool`, `destructive`) with kube-level fields (`verb`, `kind`, `group`, `version`, `name`, `namespace`), and must set at least one of these fields.

**Examples:**

```toml
confirmation_fallback = "deny"

# Confirm before uninstalling any Helm release
[[confirmation_rules]]
tool = "helm_uninstall"
message = "This will uninstall a Helm release."

# Confirm all destructive tool operations
[[confirmation_rules]]
destructive = true
message = "Destructive operation."

# Confirm Kubernetes delete calls in kube-system
[[confirmation_rules]]
verb = "delete"
namespace = "kube-system"
message = "Deleting in kube-system."

# Confirm reading Secrets
[[confirmation_rules]]
verb = "get"
kind = "Secret"
message = "Accessing a Secret."
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

**Example (Helm):**
```toml
[toolset_configs.helm]
allowed_registries = ["oci://ghcr.io/myorg", "https://charts.example.com"]
storage_driver = "configmap"
```

#### Helm Configuration

| Field | Type | Description |
|-------|------|-------------|
| `allowed_registries` | string array | Optional list of permitted chart registry URL prefixes. Only `oci://` and `https://` schemes are accepted. |
| `storage_driver` | string | Optional default storage driver for Helm operations. Supported values: `secret` (default) and `configmap`. |

The Helm toolset supports an optional `allowed_registries` allowlist to restrict which registries
`helm_install` can fetch charts from.

**Behavior:**

- `file://` and `http://` chart references are always blocked regardless of configuration.
- When `allowed_registries` is **not configured**, any `oci://` or `https://` chart reference is allowed, as well as non-URL references (e.g. `stable/grafana`) that resolve through Helm's local repository configuration.
- When `allowed_registries` **is configured**, chart references must be URL-based and prefix-match an entry in the list. Non-URL references (local paths, repo/chart names) are rejected.

**Accepted risk:** bare filesystem paths (e.g. `/absolute/path`, `./relative/path`) are not blocked when no allowlist is configured, because they are indistinguishable from Helm repository references at the string level. When the server runs in a container, the blast radius is limited to the container filesystem. To fully restrict chart sources, configure `allowed_registries`.

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
| `--tls-cert` | Path to TLS certificate file for HTTPS (must be used with `--tls-key`) |
| `--tls-key` | Path to TLS private key file for HTTPS (must be used with `--tls-cert`) |
| `--require-tls` | Enforce TLS for server and all outbound connections |

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

# Tool overrides
[tool_overrides.pods_list]
description = "List pods in the cluster. Prefer using label selectors over listing all pods when the namespace has many workloads."

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

[toolset_configs.helm]
allowed_registries = ["oci://ghcr.io/myorg", "https://charts.example.com"]
```

## Related Documentation

- [prompts.md](prompts.md) - MCP Prompts configuration
- [OTEL.md](OTEL.md) - OpenTelemetry observability
- [KIALI.md](KIALI.md) - Kiali toolset configuration
- [KEYCLOAK_OIDC_SETUP.md](KEYCLOAK_OIDC_SETUP.md) - OAuth/OIDC setup guide
- [getting-started-kubernetes.md](getting-started-kubernetes.md) - Kubernetes setup guide
