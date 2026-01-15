# Kubernetes MCP Server

[![GitHub License](https://img.shields.io/github/license/containers/kubernetes-mcp-server)](https://github.com/containers/kubernetes-mcp-server/blob/main/LICENSE)
[![npm](https://img.shields.io/npm/v/kubernetes-mcp-server)](https://www.npmjs.com/package/kubernetes-mcp-server)
[![PyPI - Version](https://img.shields.io/pypi/v/kubernetes-mcp-server)](https://pypi.org/project/kubernetes-mcp-server/)
[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/containers/kubernetes-mcp-server?sort=semver)](https://github.com/containers/kubernetes-mcp-server/releases/latest)
[![Build](https://github.com/containers/kubernetes-mcp-server/actions/workflows/build.yaml/badge.svg)](https://github.com/containers/kubernetes-mcp-server/actions/workflows/build.yaml)

[✨ Features](#features) | [🚀 Getting Started](#getting-started) | [🎥 Demos](#demos) | [⚙️ Configuration](#configuration) | [🛠️ Tools](#tools-and-functionalities) | [🧑‍💻 Development](#development)

https://github.com/user-attachments/assets/be2b67b3-fc1c-4d11-ae46-93deba8ed98e

## ✨ Features <a id="features"></a>

A powerful and flexible Kubernetes [Model Context Protocol (MCP)](https://blog.marcnuri.com/model-context-protocol-mcp-introduction) server implementation with support for **Kubernetes** and **OpenShift**.

- **✅ Configuration**:
  - Automatically detect changes in the Kubernetes configuration and update the MCP server.
  - **View** and manage the current [Kubernetes `.kube/config`](https://blog.marcnuri.com/where-is-my-default-kubeconfig-file) or in-cluster configuration.
- **✅ Generic Kubernetes Resources**: Perform operations on **any** Kubernetes or OpenShift resource.
  - Any CRUD operation (Create or Update, Get, List, Delete).
- **✅ Pods**: Perform Pod-specific operations.
  - **List** pods in all namespaces or in a specific namespace.
  - **Get** a pod by name from the specified namespace.
  - **Delete** a pod by name from the specified namespace.
  - **Show logs** for a pod by name from the specified namespace.
  - **Top** gets resource usage metrics for all pods or a specific pod in the specified namespace.
  - **Exec** into a pod and run a command.
  - **Run** a container image in a pod and optionally expose it.
- **✅ Namespaces**: List Kubernetes Namespaces.
- **✅ Events**: View Kubernetes events in all namespaces or in a specific namespace.
- **✅ Projects**: List OpenShift Projects.
- **☸️ Helm**:
  - **Install** a Helm chart in the current or provided namespace.
  - **List** Helm releases in all namespaces or in a specific namespace.
  - **Uninstall** a Helm release in the current or provided namespace.

Unlike other Kubernetes MCP server implementations, this **IS NOT** just a wrapper around `kubectl` or `helm` command-line tools.
It is a **Go-based native implementation** that interacts directly with the Kubernetes API server.

There is **NO NEED** for external dependencies or tools to be installed on the system.
If you're using the native binaries you don't need to have Node or Python installed on your system.

- **✅ Lightweight**: The server is distributed as a single native binary for Linux, macOS, and Windows.
- **✅ High-Performance / Low-Latency**: Directly interacts with the Kubernetes API server without the overhead of calling and waiting for external commands.
- **✅ Multi-Cluster**: Can interact with multiple Kubernetes clusters simultaneously (as defined in your kubeconfig files).
- **✅ Cross-Platform**: Available as a native binary for Linux, macOS, and Windows, as well as an npm package, a Python package, and container/Docker image.
- **✅ Configurable**: Supports [command-line arguments](#configuration)  to configure the server behavior.
- **✅ Well tested**: The server has an extensive test suite to ensure its reliability and correctness across different Kubernetes environments.

## 🚀 Getting Started <a id="getting-started"></a>

### Requirements

- Access to a Kubernetes cluster.

<details>
<summary><b>Claude Code</b></summary>

Follow the [dedicated Claude Code getting started guide](docs/GETTING_STARTED_CLAUDE_CODE.md) in our [user documentation](docs/).

For a secure production setup with dedicated ServiceAccount and read-only access, also review the [Kubernetes setup guide](docs/GETTING_STARTED_KUBERNETES.md).

</details>

### Claude Desktop

#### Using npx

If you have npm installed, this is the fastest way to get started with `kubernetes-mcp-server` on Claude Desktop.

Open your `claude_desktop_config.json` and add the mcp server to the list of `mcpServers`:
``` json
{
  "mcpServers": {
    "kubernetes": {
      "command": "npx",
      "args": [
        "-y",
        "kubernetes-mcp-server@latest"
      ]
    }
  }
}
```

### VS Code / VS Code Insiders

Install the Kubernetes MCP server extension in VS Code Insiders by pressing the following link:

[<img src="https://img.shields.io/badge/VS_Code-VS_Code?style=flat-square&label=Install%20Server&color=0098FF" alt="Install in VS Code">](https://insiders.vscode.dev/redirect?url=vscode%3Amcp%2Finstall%3F%257B%2522name%2522%253A%2522kubernetes%2522%252C%2522command%2522%253A%2522npx%2522%252C%2522args%2522%253A%255B%2522-y%2522%252C%2522kubernetes-mcp-server%2540latest%2522%255D%257D)
[<img alt="Install in VS Code Insiders" src="https://img.shields.io/badge/VS_Code_Insiders-VS_Code_Insiders?style=flat-square&label=Install%20Server&color=24bfa5">](https://insiders.vscode.dev/redirect?url=vscode-insiders%3Amcp%2Finstall%3F%257B%2522name%2522%253A%2522kubernetes%2522%252C%2522command%2522%253A%2522npx%2522%252C%2522args%2522%253A%255B%2522-y%2522%252C%2522kubernetes-mcp-server%2540latest%2522%255D%257D)

Alternatively, you can install the extension manually by running the following command:

```shell
# For VS Code
code --add-mcp '{"name":"kubernetes","command":"npx","args":["kubernetes-mcp-server@latest"]}'
# For VS Code Insiders
code-insiders --add-mcp '{"name":"kubernetes","command":"npx","args":["kubernetes-mcp-server@latest"]}'
```

### Cursor

Install the Kubernetes MCP server extension in Cursor by pressing the following link:

[![Install MCP Server](https://cursor.com/deeplink/mcp-install-dark.svg)](https://cursor.com/en/install-mcp?name=kubernetes-mcp-server&config=eyJjb21tYW5kIjoibnB4IC15IGt1YmVybmV0ZXMtbWNwLXNlcnZlckBsYXRlc3QifQ%3D%3D)

Alternatively, you can install the extension manually by editing the `mcp.json` file:

```json
{
  "mcpServers": {
    "kubernetes-mcp-server": {
      "command": "npx",
      "args": ["-y", "kubernetes-mcp-server@latest"]
    }
  }
}
```

### Goose CLI

[Goose CLI](https://blog.marcnuri.com/goose-on-machine-ai-agent-cli-introduction) is the easiest (and cheapest) way to get rolling with artificial intelligence (AI) agents.

#### Using npm

If you have npm installed, this is the fastest way to get started with `kubernetes-mcp-server`.

Open your goose `config.yaml` and add the mcp server to the list of `mcpServers`:
```yaml
extensions:
  kubernetes:
    command: npx
    args:
      - -y
      - kubernetes-mcp-server@latest

```

## 🎥 Demos <a id="demos"></a>

### Diagnosing and automatically fixing an OpenShift Deployment

Demo showcasing how Kubernetes MCP server is leveraged by Claude Desktop to automatically diagnose and fix a deployment in OpenShift without any user assistance.

https://github.com/user-attachments/assets/a576176d-a142-4c19-b9aa-a83dc4b8d941

### _Vibe Coding_ a simple game and deploying it to OpenShift

In this demo, I walk you through the process of _Vibe Coding_ a simple game using VS Code and how to leverage [Podman MCP server](https://github.com/manusa/podman-mcp-server) and Kubernetes MCP server to deploy it to OpenShift.

<a href="https://www.youtube.com/watch?v=l05jQDSrzVI" target="_blank">
 <img src="docs/images/vibe-coding.jpg" alt="Vibe Coding: Build & Deploy a Game on Kubernetes" width="240"  />
</a>

### Supercharge GitHub Copilot with Kubernetes MCP Server in VS Code - One-Click Setup!

In this demo, I'll show you how to set up Kubernetes MCP server in VS code just by clicking a link.

<a href="https://youtu.be/AI4ljYMkgtA" target="_blank">
 <img src="docs/images/kubernetes-mcp-server-github-copilot.jpg" alt="Supercharge GitHub Copilot with Kubernetes MCP Server in VS Code - One-Click Setup!" width="240"  />
</a>

## ⚙️ Configuration <a id="configuration"></a>

The Kubernetes MCP server can be configured using command line (CLI) arguments.

You can run the CLI executable either by using `npx`, `uvx`, or by downloading the [latest release binary](https://github.com/containers/kubernetes-mcp-server/releases/latest).

```shell
# Run the Kubernetes MCP server using npx (in case you have npm and node installed)
npx kubernetes-mcp-server@latest --help
```

```shell
# Run the Kubernetes MCP server using uvx (in case you have uv and python installed)
uvx kubernetes-mcp-server@latest --help
```

```shell
# Run the Kubernetes MCP server using the latest release binary
./kubernetes-mcp-server --help
```

### Configuration Options

| Option                    | Description                                                                                                                                                                                                                                                                                   |
|---------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--port`                  | Starts the MCP server in Streamable HTTP mode (path /mcp) and Server-Sent Event (SSE) (path /sse) mode and listens on the specified port .                                                                                                                                                    |
| `--log-level`             | Sets the logging level (values [from 0-9](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md)). Similar to [kubectl logging levels](https://kubernetes.io/docs/reference/kubectl/quick-reference/#kubectl-output-verbosity-and-debugging). |
| `--config`                | (Optional) Path to the main TOML configuration file. See [Drop-in Configuration](#drop-in-configuration) section below for details.                                                                                                                                                          |
| `--config-dir`            | (Optional) Path to drop-in configuration directory. Files are loaded in lexical (alphabetical) order. Defaults to `conf.d` relative to the main config file if `--config` is specified. See [Drop-in Configuration](#drop-in-configuration) section below for details.                       |
| `--kubeconfig`            | Path to the Kubernetes configuration file. If not provided, it will try to resolve the configuration (in-cluster, default location, etc.).                                                                                                                                                    |
| `--list-output`           | Output format for resource list operations (one of: yaml, table) (default "table")                                                                                                                                                                                                            |
| `--read-only`             | If set, the MCP server will run in read-only mode, meaning it will not allow any write operations (create, update, delete) on the Kubernetes cluster. This is useful for debugging or inspecting the cluster without making changes.                                                          |
| `--disable-destructive`   | If set, the MCP server will disable all destructive operations (delete, update, etc.) on the Kubernetes cluster. This is useful for debugging or inspecting the cluster without accidentally making changes. This option has no effect when `--read-only` is used.                            |
| `--stateless`             | If set, the MCP server will run in stateless mode, disabling tool and prompt change notifications. This is useful for container deployments, load balancing, and serverless environments where maintaining client state is not desired. |
| `--toolsets`              | Comma-separated list of toolsets to enable. Check the [🛠️ Tools and Functionalities](#tools-and-functionalities) section for more information.                                                                                                                                               |
| `--disable-multi-cluster` | If set, the MCP server will disable multi-cluster support and will only use the current context from the kubeconfig file. This is useful if you want to restrict the MCP server to a single cluster.                                                                                          |

### Drop-in Configuration <a id="drop-in-configuration"></a>

The Kubernetes MCP server supports flexible configuration through both a main config file and drop-in files. **Both are optional** - you can use either, both, or neither (server will use built-in defaults).

#### Configuration Loading Order

Configuration values are loaded and merged in the following order (later sources override earlier ones):

1. **Internal Defaults** - Always loaded (hardcoded default values)
2. **Main Configuration File** - Optional, loaded via `--config` flag
3. **Drop-in Files** - Optional, loaded from `--config-dir` in **lexical (alphabetical) order**

#### How Drop-in Files Work

- **Default Directory**: If `--config-dir` is not specified, the server looks for drop-in files in `conf.d/` relative to the main config file's directory (when `--config` is provided)
- **File Naming**: Use numeric prefixes to control loading order (e.g., `00-base.toml`, `10-cluster.toml`, `99-override.toml`)
- **File Extension**: Only `.toml` files are processed; dotfiles (starting with `.`) are ignored
- **Partial Configuration**: Drop-in files can contain only a subset of configuration options
- **Merge Behavior**: Values present in a drop-in file override previous values; missing values are preserved

#### Dynamic Configuration Reload

To reload configuration after modifying config files, send a `SIGHUP` signal to the running server process.

**Prerequisite**: SIGHUP reload requires the server to be started with either the `--config` flag or `--config-dir` flag (or both). If neither is specified, SIGHUP signals will be ignored.

**How to reload:**

```shell
# Find the process ID
ps aux | grep kubernetes-mcp-server

# Send SIGHUP to reload configuration
kill -HUP <pid>

# Or use pkill
pkill -HUP kubernetes-mcp-server
```

The server will:
- Reload the main config file and all drop-in files
- Update configuration values (log level, output format, etc.)
- Rebuild the toolset registry with new tool configurations
- Log the reload status

**Note**: Changing `kubeconfig` or cluster-related settings requires a server restart. Only tool configurations, log levels, and output formats can be reloaded dynamically.

**Note**: SIGHUP reload is not available on Windows. On Windows, restart the server to reload configuration.

#### Example: Using Both Config Methods

**Command (using default `conf.d` directory):**
```shell
kubernetes-mcp-server --config /etc/kubernetes-mcp-server/config.toml
```

**Directory structure:**
```
/etc/kubernetes-mcp-server/
├── config.toml              # Main configuration
└── conf.d/                  # Default drop-in directory (automatically loaded)
    ├── 00-base.toml         # Base overrides
    ├── 10-toolsets.toml     # Toolset-specific config
    └── 99-local.toml        # Local overrides
```

**Command (with explicit `--config-dir`):**
```shell
kubernetes-mcp-server --config /etc/kubernetes-mcp-server/config.toml \
                      --config-dir /etc/kubernetes-mcp-server/config.d/
```

**Example drop-in file** (`10-toolsets.toml`):
```toml
# Override only the toolsets - all other config preserved
toolsets = ["core", "config", "helm", "logs"]
```

**Example drop-in file** (`99-local.toml`):
```toml
# Local development overrides
log_level = 9
read_only = true
```

**To apply changes:**
```shell
# Edit config files
vim /etc/kubernetes-mcp-server/conf.d/99-local.toml

# Reload without restarting
pkill -HUP kubernetes-mcp-server
```

### MCP Prompts

1. The server supports MCP prompts for workflow templates. Define custom prompts in `config.toml`:

```toml
[[prompts]]
name = "my-workflow"
title = "my workflow"
description = "Custom workflow"

[[prompts.arguments]]
name = "resource_name"
required = true

[[prompts.messages]]
role = "user"
content = "Help me with {{resource_name}}"
```

2. Toolset prompts implemented by toolset developers

See docs/PROMPTS.md for detailed documentation.

## 📊 MCP Logging <a id="mcp-logging"></a>

The server supports the MCP logging capability, allowing clients to receive debugging information via structured log messages.

### For Clients

Clients can control log verbosity by sending a `logging/setLevel` request:

```json
{
  "method": "logging/setLevel",
  "params": { "level": "info" }
}
```

**Available log levels** (in order of increasing severity):
- `debug` - Detailed debugging information
- `info` - General informational messages (default)
- `notice` - Normal but significant events
- `warning` - Warning messages
- `error` - Error conditions
- `critical` - Critical conditions
- `alert` - Action must be taken immediately
- `emergency` - System is unusable

### For Developers

Toolsets can optionally send debug information to clients using helper functions from the `mcplog` package:

**Recommended approach for Kubernetes errors** (automatically categorizes errors and sends appropriate messages):

```go
import "github.com/containers/kubernetes-mcp-server/pkg/mcplog"

// In your tool handler:
ret, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
if err != nil {
    mcplog.HandleK8sError(ctx, err, "pod access")
    return api.NewToolCallResult("", fmt.Errorf("failed to get pod: %v", err)), nil
}
```

**Manual logging** (for custom messages):

```go
import "github.com/containers/kubernetes-mcp-server/pkg/mcplog"

// In your tool handler:
if err != nil {
    mcplog.SendMCPLog(ctx, "error", "Operation failed - check permissions")
    return api.NewToolCallResult("", err)
}
```

**Key Points:**
- Logging is **optional** - toolsets work fine without sending MCP logs
- Uses a dedicated named logger (`logger="mcp"`) for complete separation from server logs
- Server logs (klog) remain detailed and unaffected
- Client logs are high-level, helpful hints for debugging
- Authentication failures send generic messages to clients (no security info leaked)
- Sensitive data is automatically redacted with 28 pattern types:
  - Generic fields (password, token, secret, api_key, etc.)
  - Authorization headers (Bearer, Basic)
  - Cloud credentials (AWS, GCP, Azure)
  - API tokens (GitHub, GitLab, OpenAI, Anthropic)
  - Cryptographic keys (JWT, SSH, PGP, RSA)
  - Database connection strings (PostgreSQL, MySQL, MongoDB)

## 🛠️ Tools and Functionalities <a id="tools-and-functionalities"></a>

The Kubernetes MCP server supports enabling or disabling specific groups of tools and functionalities (tools, resources, prompts, and so on) via the `--toolsets` command-line flag or `toolsets` configuration option.
This allows you to control which Kubernetes functionalities are available to your AI tools.
Enabling only the toolsets you need can help reduce the context size and improve the LLM's tool selection accuracy.

### Available Toolsets

The following sets of tools are available (toolsets marked with ✓ in the Default column are enabled by default):

<!-- AVAILABLE-TOOLSETS-START -->

| Toolset  | Description                                                                                                                                                          | Default |
|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------|
| config   | View and manage the current local Kubernetes configuration (kubeconfig)                                                                                              | ✓       |
| core     | Most common tools for Kubernetes management (Pods, Generic Resources, Events, etc.)                                                                                  | ✓       |
| kiali    | Most common tools for managing Kiali, check the [Kiali documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/KIALI.md) for more details. |         |
| kubevirt | KubeVirt virtual machine management tools                                                                                                                            |         |
| helm     | Tools for managing Helm charts and releases                                                                                                                          | ✓       |

<!-- AVAILABLE-TOOLSETS-END -->

### Tools

In case multi-cluster support is enabled (default) and you have access to multiple clusters, all applicable tools will include an additional `context` argument to specify the Kubernetes context (cluster) to use for that operation.

<!-- AVAILABLE-TOOLSETS-TOOLS-START -->

<details>

<summary>config</summary>

- **configuration_contexts_list** - List all available context names and associated server urls from the kubeconfig file

- **targets_list** - List all available targets

- **configuration_view** - Get the current Kubernetes configuration content as a kubeconfig YAML
  - `minified` (`boolean`) - Return a minified version of the configuration. If set to true, keeps only the current-context and the relevant pieces of the configuration for that context. If set to false, all contexts, clusters, auth-infos, and users are returned in the configuration. (Optional, default true)

</details>

<details>

<summary>core</summary>

- **events_list** - List all the Kubernetes events in the current cluster from all namespaces
  - `namespace` (`string`) - Optional Namespace to retrieve the events from. If not provided, will list events from all namespaces

- **namespaces_list** - List all the Kubernetes namespaces in the current cluster

- **projects_list** - List all the OpenShift projects in the current cluster

- **nodes_log** - Get logs from a Kubernetes node (kubelet, kube-proxy, or other system logs). This accesses node logs through the Kubernetes API proxy to the kubelet
  - `name` (`string`) **(required)** - Name of the node to get logs from
  - `query` (`string`) **(required)** - query specifies services(s) or files from which to return logs (required). Example: "kubelet" to fetch kubelet logs, "/<log-file-name>" to fetch a specific log file from the node (e.g., "/var/log/kubelet.log" or "/var/log/kube-proxy.log")
  - `tailLines` (`integer`) - Number of lines to retrieve from the end of the logs (Optional, 0 means all logs)

- **nodes_stats_summary** - Get detailed resource usage statistics from a Kubernetes node via the kubelet's Summary API. Provides comprehensive metrics including CPU, memory, filesystem, and network usage at the node, pod, and container levels. On systems with cgroup v2 and kernel 4.20+, also includes PSI (Pressure Stall Information) metrics that show resource pressure for CPU, memory, and I/O. See https://kubernetes.io/docs/reference/instrumentation/understand-psi-metrics/ for details on PSI metrics
  - `name` (`string`) **(required)** - Name of the node to get stats from

- **nodes_top** - List the resource consumption (CPU and memory) as recorded by the Kubernetes Metrics Server for the specified Kubernetes Nodes or all nodes in the cluster
  - `label_selector` (`string`) - Kubernetes label selector (e.g. 'node-role.kubernetes.io/worker=') to filter nodes by label (Optional, only applicable when name is not provided)
  - `name` (`string`) - Name of the Node to get the resource consumption from (Optional, all Nodes if not provided)

- **pods_list** - List all the Kubernetes pods in the current cluster from all namespaces
  - `fieldSelector` (`string`) - Optional Kubernetes field selector to filter pods by field values (e.g. 'status.phase=Running', 'spec.nodeName=node1'). Supported fields: metadata.name, metadata.namespace, spec.nodeName, spec.restartPolicy, spec.schedulerName, spec.serviceAccountName, status.phase (Pending/Running/Succeeded/Failed/Unknown), status.podIP, status.nominatedNodeName. Note: CrashLoopBackOff is a container state, not a pod phase, so it cannot be filtered directly. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/
  - `labelSelector` (`string`) - Optional Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label

- **pods_list_in_namespace** - List all the Kubernetes pods in the specified namespace in the current cluster
  - `fieldSelector` (`string`) - Optional Kubernetes field selector to filter pods by field values (e.g. 'status.phase=Running', 'spec.nodeName=node1'). Supported fields: metadata.name, metadata.namespace, spec.nodeName, spec.restartPolicy, spec.schedulerName, spec.serviceAccountName, status.phase (Pending/Running/Succeeded/Failed/Unknown), status.podIP, status.nominatedNodeName. Note: CrashLoopBackOff is a container state, not a pod phase, so it cannot be filtered directly. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/
  - `labelSelector` (`string`) - Optional Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label
  - `namespace` (`string`) **(required)** - Namespace to list pods from

- **pods_get** - Get a Kubernetes Pod in the current or provided namespace with the provided name
  - `name` (`string`) **(required)** - Name of the Pod
  - `namespace` (`string`) - Namespace to get the Pod from

- **pods_delete** - Delete a Kubernetes Pod in the current or provided namespace with the provided name
  - `name` (`string`) **(required)** - Name of the Pod to delete
  - `namespace` (`string`) - Namespace to delete the Pod from

- **pods_top** - List the resource consumption (CPU and memory) as recorded by the Kubernetes Metrics Server for the specified Kubernetes Pods in the all namespaces, the provided namespace, or the current namespace
  - `all_namespaces` (`boolean`) - If true, list the resource consumption for all Pods in all namespaces. If false, list the resource consumption for Pods in the provided namespace or the current namespace
  - `label_selector` (`string`) - Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label (Optional, only applicable when name is not provided)
  - `name` (`string`) - Name of the Pod to get the resource consumption from (Optional, all Pods in the namespace if not provided)
  - `namespace` (`string`) - Namespace to get the Pods resource consumption from (Optional, current namespace if not provided and all_namespaces is false)

- **pods_exec** - Execute a command in a Kubernetes Pod in the current or provided namespace with the provided name and command
  - `command` (`array`) **(required)** - Command to execute in the Pod container. The first item is the command to be run, and the rest are the arguments to that command. Example: ["ls", "-l", "/tmp"]
  - `container` (`string`) - Name of the Pod container where the command will be executed (Optional)
  - `name` (`string`) **(required)** - Name of the Pod where the command will be executed
  - `namespace` (`string`) - Namespace of the Pod where the command will be executed

- **pods_log** - Get the logs of a Kubernetes Pod in the current or provided namespace with the provided name
  - `container` (`string`) - Name of the Pod container to get the logs from (Optional)
  - `name` (`string`) **(required)** - Name of the Pod to get the logs from
  - `namespace` (`string`) - Namespace to get the Pod logs from
  - `previous` (`boolean`) - Return previous terminated container logs (Optional)
  - `tail` (`integer`) - Number of lines to retrieve from the end of the logs (Optional, default: 100)

- **pods_run** - Run a Kubernetes Pod in the current or provided namespace with the provided container image and optional name
  - `image` (`string`) **(required)** - Container Image to run in the Pod
  - `name` (`string`) - Name of the Pod (Optional, random name if not provided)
  - `namespace` (`string`) - Namespace to run the Pod in
  - `port` (`number`) - TCP/IP port to expose from the Pod container (Optional, no port exposed if not provided)

- **resources_list** - List Kubernetes resources and objects in the current cluster by providing their apiVersion and kind and optionally the namespace and label selector
(common apiVersion and kind include: v1 Pod, v1 Service, v1 Node, apps/v1 Deployment, networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)
  - `apiVersion` (`string`) **(required)** - apiVersion of the resources (examples of valid apiVersion are: v1, apps/v1, networking.k8s.io/v1)
  - `fieldSelector` (`string`) - Optional Kubernetes field selector to filter resources by field values (e.g. 'status.phase=Running', 'metadata.name=myresource'). Supported fields vary by resource type. For Pods: metadata.name, metadata.namespace, spec.nodeName, spec.restartPolicy, spec.schedulerName, spec.serviceAccountName, status.phase (Pending/Running/Succeeded/Failed/Unknown), status.podIP, status.nominatedNodeName. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/
  - `kind` (`string`) **(required)** - kind of the resources (examples of valid kind are: Pod, Service, Deployment, Ingress)
  - `labelSelector` (`string`) - Optional Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the resources by label
  - `namespace` (`string`) - Optional Namespace to retrieve the namespaced resources from (ignored in case of cluster scoped resources). If not provided, will list resources from all namespaces

- **resources_get** - Get a Kubernetes resource in the current cluster by providing its apiVersion, kind, optionally the namespace, and its name
(common apiVersion and kind include: v1 Pod, v1 Service, v1 Node, apps/v1 Deployment, networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)
  - `apiVersion` (`string`) **(required)** - apiVersion of the resource (examples of valid apiVersion are: v1, apps/v1, networking.k8s.io/v1)
  - `kind` (`string`) **(required)** - kind of the resource (examples of valid kind are: Pod, Service, Deployment, Ingress)
  - `name` (`string`) **(required)** - Name of the resource
  - `namespace` (`string`) - Optional Namespace to retrieve the namespaced resource from (ignored in case of cluster scoped resources). If not provided, will get resource from configured namespace

- **resources_create_or_update** - Create or update a Kubernetes resource in the current cluster by providing a YAML or JSON representation of the resource
(common apiVersion and kind include: v1 Pod, v1 Service, v1 Node, apps/v1 Deployment, networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)
  - `resource` (`string`) **(required)** - A JSON or YAML containing a representation of the Kubernetes resource. Should include top-level fields such as apiVersion,kind,metadata, and spec

- **resources_delete** - Delete a Kubernetes resource in the current cluster by providing its apiVersion, kind, optionally the namespace, and its name
(common apiVersion and kind include: v1 Pod, v1 Service, v1 Node, apps/v1 Deployment, networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)
  - `apiVersion` (`string`) **(required)** - apiVersion of the resource (examples of valid apiVersion are: v1, apps/v1, networking.k8s.io/v1)
  - `kind` (`string`) **(required)** - kind of the resource (examples of valid kind are: Pod, Service, Deployment, Ingress)
  - `name` (`string`) **(required)** - Name of the resource
  - `namespace` (`string`) - Optional Namespace to delete the namespaced resource from (ignored in case of cluster scoped resources). If not provided, will delete resource from configured namespace

- **resources_scale** - Get or update the scale of a Kubernetes resource in the current cluster by providing its apiVersion, kind, name, and optionally the namespace. If the scale is set in the tool call, the scale will be updated to that value. Always returns the current scale of the resource
  - `apiVersion` (`string`) **(required)** - apiVersion of the resource (examples of valid apiVersion are apps/v1)
  - `kind` (`string`) **(required)** - kind of the resource (examples of valid kind are: StatefulSet, Deployment)
  - `name` (`string`) **(required)** - Name of the resource
  - `namespace` (`string`) - Optional Namespace to get/update the namespaced resource scale from (ignored in case of cluster scoped resources). If not provided, will get/update resource scale from configured namespace
  - `scale` (`integer`) - Optional scale to update the resources scale to. If not provided, will return the current scale of the resource, and not update it

</details>

<details>

<summary>kiali</summary>

- **kiali_mesh_graph** - Returns the topology of a specific namespaces, health, status of the mesh and namespaces. Includes a mesh health summary overview with aggregated counts of healthy, degraded, and failing apps, workloads, and services. Use this for high-level overviews
  - `graphType` (`string`) - Optional type of graph to return: 'versionedApp', 'app', 'service', 'workload', 'mesh'
  - `namespace` (`string`) - Optional single namespace to include in the graph (alternative to namespaces)
  - `namespaces` (`string`) - Optional comma-separated list of namespaces to include in the graph
  - `rateInterval` (`string`) - Optional rate interval for fetching (e.g., '10m', '5m', '1h').

- **kiali_manage_istio_config** - Manages Istio configuration objects (Gateways, VirtualServices, etc.). Can list (objects and validations), get, create, patch, or delete objects
  - `action` (`string`) **(required)** - Action to perform: list, get, create, patch, or delete
  - `group` (`string`) - API group of the Istio object (e.g., 'networking.istio.io', 'gateway.networking.k8s.io')
  - `json_data` (`string`) - JSON data to apply or create the object
  - `kind` (`string`) - Kind of the Istio object (e.g., 'DestinationRule', 'VirtualService', 'HTTPRoute', 'Gateway')
  - `name` (`string`) - Name of the Istio object
  - `namespace` (`string`) - Namespace containing the Istio object
  - `version` (`string`) - API version of the Istio object (e.g., 'v1', 'v1beta1')

- **kiali_get_resource_details** - Gets lists or detailed info for Kubernetes resources (services, workloads) within the mesh
  - `namespaces` (`string`) - Comma-separated list of namespaces to get services from (e.g. 'bookinfo' or 'bookinfo,default'). If not provided, will list services from all accessible namespaces
  - `resource_name` (`string`) - Name of the resource to get details for (optional string - if provided, gets details; if empty, lists all).
  - `resource_type` (`string`) - Type of resource to get details for (service, workload)

- **kiali_get_metrics** - Gets lists or detailed info for Kubernetes resources (services, workloads) within the mesh
  - `byLabels` (`string`) - Comma-separated list of labels to group metrics by (e.g., 'source_workload,destination_service'). Optional
  - `direction` (`string`) - Traffic direction: 'inbound' or 'outbound'. Optional, defaults to 'outbound'
  - `duration` (`string`) - Time range to get metrics for (optional string - if provided, gets metrics (e.g., '1m', '5m', '1h'); if empty, get default 30m).
  - `namespace` (`string`) **(required)** - Namespace to get resources from
  - `quantiles` (`string`) - Comma-separated list of quantiles for histogram metrics (e.g., '0.5,0.95,0.99'). Optional
  - `rateInterval` (`string`) - Rate interval for metrics (e.g., '1m', '5m'). Optional, defaults to '10m'
  - `reporter` (`string`) - Metrics reporter: 'source', 'destination', or 'both'. Optional, defaults to 'source'
  - `requestProtocol` (`string`) - Filter by request protocol (e.g., 'http', 'grpc', 'tcp'). Optional
  - `resource_name` (`string`) **(required)** - Name of the resource to get details for (optional string - if provided, gets details; if empty, lists all).
  - `resource_type` (`string`) **(required)** - Type of resource to get details for (service, workload)
  - `step` (`string`) - Step between data points in seconds (e.g., '15'). Optional, defaults to 15 seconds

- **kiali_workload_logs** - Get logs for a specific workload's pods in a namespace. Only requires namespace and workload name - automatically discovers pods and containers. Optionally filter by container name, time range, and other parameters. Container is auto-detected if not specified.
  - `container` (`string`) - Optional container name to filter logs. If not provided, automatically detects and uses the main application container (excludes istio-proxy and istio-init)
  - `namespace` (`string`) **(required)** - Namespace containing the workload
  - `since` (`string`) - Time duration to fetch logs from (e.g., '5m', '1h', '30s'). If not provided, returns recent logs
  - `tail` (`integer`) - Number of lines to retrieve from the end of logs (default: 100)
  - `workload` (`string`) **(required)** - Name of the workload to get logs for

- **kiali_get_traces** - Gets traces for a specific resource (app, service, workload) in a namespace, or gets detailed information for a specific trace by its ID. If traceId is provided, it returns detailed trace information and other parameters are not required.
  - `clusterName` (`string`) - Cluster name for multi-cluster environments (optional, only used when traceId is not provided)
  - `endMicros` (`string`) - End time for traces in microseconds since epoch (optional, defaults to 10 minutes after startMicros if not provided, only used when traceId is not provided)
  - `limit` (`integer`) - Maximum number of traces to return (default: 100, only used when traceId is not provided)
  - `minDuration` (`integer`) - Minimum trace duration in microseconds (optional, only used when traceId is not provided)
  - `namespace` (`string`) - Namespace to get resources from. Required if traceId is not provided.
  - `resource_name` (`string`) - Name of the resource to get traces for. Required if traceId is not provided.
  - `resource_type` (`string`) - Type of resource to get traces for (app, service, workload). Required if traceId is not provided.
  - `startMicros` (`string`) - Start time for traces in microseconds since epoch (optional, defaults to 10 minutes before current time if not provided, only used when traceId is not provided)
  - `tags` (`string`) - JSON string of tags to filter traces (optional, only used when traceId is not provided)
  - `traceId` (`string`) - Unique identifier of the trace to retrieve detailed information for. If provided, this will return detailed trace information and other parameters (resource_type, namespace, resource_name) are not required.

</details>

<details>

<summary>kubevirt</summary>

- **vm_create** - Create a VirtualMachine in the cluster with the specified configuration, automatically resolving instance types, preferences, and container disk images. VM will be created in Halted state by default; use autostart parameter to start it immediately.
  - `autostart` (`boolean`) - Optional flag to automatically start the VM after creation (sets runStrategy to Always instead of Halted). Defaults to false.
  - `instancetype` (`string`) - Optional instance type name for the VM (e.g., 'u1.small', 'u1.medium', 'u1.large')
  - `name` (`string`) **(required)** - The name of the virtual machine
  - `namespace` (`string`) **(required)** - The namespace for the virtual machine
  - `performance` (`string`) - Optional performance family hint for the VM instance type (e.g., 'u1' for general-purpose, 'o1' for overcommitted, 'c1' for compute-optimized, 'm1' for memory-optimized). Defaults to 'u1' (general-purpose) if not specified.
  - `preference` (`string`) - Optional preference name for the VM
  - `size` (`string`) - Optional workload size hint for the VM (e.g., 'small', 'medium', 'large', 'xlarge'). Used to auto-select an appropriate instance type if not explicitly specified.
  - `storage` (`string`) - Optional storage size for the VM's root disk when using DataSources (e.g., '30Gi', '50Gi', '100Gi'). Defaults to 30Gi. Ignored when using container disks.
  - `workload` (`string`) - The workload for the VM. Accepts OS names (e.g., 'fedora' (default), 'ubuntu', 'centos', 'centos-stream', 'debian', 'rhel', 'opensuse', 'opensuse-tumbleweed', 'opensuse-leap') or full container disk image URLs

- **vm_lifecycle** - Manage VirtualMachine lifecycle: start, stop, or restart a VM
  - `action` (`string`) **(required)** - The lifecycle action to perform: 'start' (changes runStrategy to Always), 'stop' (changes runStrategy to Halted), or 'restart' (stops then starts the VM)
  - `name` (`string`) **(required)** - The name of the virtual machine
  - `namespace` (`string`) **(required)** - The namespace of the virtual machine

</details>

<details>

<summary>helm</summary>

- **helm_install** - Install a Helm chart in the current or provided namespace
  - `chart` (`string`) **(required)** - Chart reference to install (for example: stable/grafana, oci://ghcr.io/nginxinc/charts/nginx-ingress)
  - `name` (`string`) - Name of the Helm release (Optional, random name if not provided)
  - `namespace` (`string`) - Namespace to install the Helm chart in (Optional, current namespace if not provided)
  - `values` (`object`) - Values to pass to the Helm chart (Optional)

- **helm_list** - List all the Helm releases in the current or provided namespace (or in all namespaces if specified)
  - `all_namespaces` (`boolean`) - If true, lists all Helm releases in all namespaces ignoring the namespace argument (Optional)
  - `namespace` (`string`) - Namespace to list Helm releases from (Optional, all namespaces if not provided)

- **helm_uninstall** - Uninstall a Helm release in the current or provided namespace
  - `name` (`string`) **(required)** - Name of the Helm release to uninstall
  - `namespace` (`string`) - Namespace to uninstall the Helm release from (Optional, current namespace if not provided)

</details>


<!-- AVAILABLE-TOOLSETS-TOOLS-END -->

## Helm Chart

A [Helm Chart](https://helm.sh) is available to simplify the deployment of the Kubernetes MCP server. Additional details can be found in the [chart README](./charts/kubernetes-mcp-server/README.md).

## 🧑‍💻 Development <a id="development"></a>

### Running with mcp-inspector

Compile the project and run the Kubernetes MCP server with [mcp-inspector](https://modelcontextprotocol.io/docs/tools/inspector) to inspect the MCP server.

```shell
# Compile the project
make build
# Run the Kubernetes MCP server with mcp-inspector
npx @modelcontextprotocol/inspector@latest $(pwd)/kubernetes-mcp-server
```

---

mcp-name: io.github.containers/kubernetes-mcp-server