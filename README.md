# Kubernetes MCP Server

[![GitHub License](https://img.shields.io/github/license/containers/kubernetes-mcp-server)](https://github.com/containers/kubernetes-mcp-server/blob/main/LICENSE)
[![npm](https://img.shields.io/npm/v/kubernetes-mcp-server)](https://www.npmjs.com/package/kubernetes-mcp-server)
[![PyPI - Version](https://img.shields.io/pypi/v/kubernetes-mcp-server)](https://pypi.org/project/kubernetes-mcp-server/)
[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/containers/kubernetes-mcp-server?sort=semver)](https://github.com/containers/kubernetes-mcp-server/releases/latest)
[![Build](https://github.com/containers/kubernetes-mcp-server/actions/workflows/build.yaml/badge.svg)](https://github.com/containers/kubernetes-mcp-server/actions/workflows/build.yaml)

[✨ Features](#features) | [🚀 Getting Started](#getting-started) | [🎥 Demos](#demos) | [⚙️ Configuration](#configuration) | [🛠️ Tools](#tools-and-functionalities) | [💬 Community](#community) | [🧑‍💻 Development](#development)

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
- **🧩 Ecosystem tools as data**: Kiali, NetObserv, Tekton and KubeVirt tools and prompts are delivered as `Tool` and `Prompt` custom resources (Helm charts under [`examples/dynamic-tools/charts`](examples/dynamic-tools/charts)) and picked up at runtime by the `dynamic` toolset. See [docs/specs/dynamic-tools.md](docs/specs/dynamic-tools.md).
- **🔭 Observability**: Optional OpenTelemetry distributed tracing and metrics with custom sampling rates. Includes `/stats` endpoint for real-time statistics. See [OTEL.md](docs/OTEL.md).

Unlike other Kubernetes MCP server implementations, this **IS NOT** just a wrapper around `kubectl` or `helm` command-line tools.
It is a **Go-based native implementation** that interacts directly with the Kubernetes API server.

There is **NO NEED** for external dependencies or tools to be installed on the system.
If you're using the native binaries you don't need to have Node or Python installed on your system.

- **✅ Lightweight**: The server is distributed as a single native binary for Linux, macOS, and Windows.
- **✅ High-Performance / Low-Latency**: Directly interacts with the Kubernetes API server without the overhead of calling and waiting for external commands.
- **✅ Multi-Cluster**: Can interact with multiple Kubernetes clusters simultaneously (as defined in your kubeconfig files).
- **✅ Cross-Platform**: Available as a native binary for Linux, macOS, and Windows, as well as an npm package, a Python package, and container/Docker image.
- **✅ Configurable**: Supports [command-line arguments](#configuration), [TOML configuration files](docs/configuration.md), and environment variables.
- **✅ Well tested**: The server has an extensive test suite to ensure its reliability and correctness across different Kubernetes environments.
- **📚 Documentation**: Comprehensive [user documentation](docs/) including setup guides, configuration reference, and observability.

## 🚀 Getting Started <a id="getting-started"></a>

### Requirements

- Access to a Kubernetes cluster.

<details>
<summary><b>Claude Code</b></summary>

Follow the [dedicated Claude Code getting started guide](docs/getting-started-claude-code.md) in our [user documentation](docs/).

For a secure production setup with dedicated ServiceAccount and read-only access, also review the [Kubernetes setup guide](docs/getting-started-kubernetes.md).

</details>

### Claude Desktop

#### Using npx

If you have npm installed, this is the fastest way to get started with `kubernetes-mcp-server` on Claude Desktop.

Open your `claude_desktop_config.json` and add the mcp server to the list of `mcpServers`:

```json
{
  "mcpServers": {
    "kubernetes": {
      "command": "npx",
      "args": ["-y", "kubernetes-mcp-server@latest"]
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

| Option         | Description |
| -------------- | ----------- |
| `--version`    | Print version information and quit. |
| `--config`     | Path to the main TOML configuration file. See [Configuration Reference](docs/configuration.md) for all options (port, toolsets, read_only, kubeconfig, …). |
| `--config-dir` | Directory of lexical `.toml` files. Usable alone or with `--config`. Omitted means no drop-ins. Relative paths are resolved against the working directory. |

> **Note**: Runtime settings are TOML (or existing env names), not CLI flags. See [Configuration Changes](docs/configuration-changes.md) for the flag-to-TOML mapping.

### TOML Configuration Files

For complex or persistent configurations, use TOML configuration files instead of CLI arguments:

```shell
kubernetes-mcp-server --config /etc/kubernetes-mcp-server/config.toml
```

**Example configuration:**

```toml
log_level = 2
read_only = true
toolsets = ["core", "config", "helm", "kubevirt"]

# Deny access to sensitive resources
[[denied_resources]]
group = ""
version = "v1"
kind = "Secret"

[telemetry]
endpoint = "http://localhost:4317"
```

For comprehensive TOML configuration documentation, including:

- All configuration options and their defaults
- Drop-in configuration files for modular settings
- Dynamic configuration reload via SIGHUP
- Denied resources for restricting access to sensitive resource types
- Server instructions for MCP Tool Search
- [Custom MCP prompts](docs/prompts.md)
- OAuth/OIDC authentication for HTTP mode ([Keycloak](docs/KEYCLOAK_OIDC_SETUP.md), [Microsoft Entra ID](docs/ENTRA_ID_SETUP.md))

See the **[Configuration Reference](docs/configuration.md)**.

## 📊 MCP Logging <a id="mcp-logging"></a>

The server supports the MCP logging capability, allowing clients to receive debugging information via structured log messages.
Kubernetes API errors are automatically categorized and logged to clients with appropriate severity levels.
Sensitive data (tokens, keys, passwords, cloud credentials) is automatically redacted before being sent to clients.

See the **[MCP Logging Guide](docs/logging.md)**.

## 🛠️ Tools and Functionalities <a id="tools-and-functionalities"></a>

The Kubernetes MCP server supports enabling or disabling specific groups of tools and functionalities (tools, resources, prompts, and so on) via the `toolsets` configuration option.
This allows you to control which Kubernetes functionalities are available to your AI tools.
Enabling only the toolsets you need can help reduce the context size and improve the LLM's tool selection accuracy.

### Validated Kubernetes Ecosystem Projects

The following CNCF and Kubernetes ecosystem projects are covered by
automated evaluation scenarios in [`evals/tasks`](evals/tasks). Most scenarios
work with just the `core` toolset. The dedicated toolsets below are optional
and only needed for the project-specific scenarios noted.

<!-- VALIDATED-PROJECTS-START -->

| Project | Optional toolset(s) | Eval scenarios |
|---------|---------------------|----------------|
| [Helm](https://helm.sh) | `helm` | 3 |
| [Istio](https://istio.io) | `dynamic` | 5 |
| [Kiali](https://kiali.io) | `dynamic` | 16 |
| [Kubernetes](https://kubernetes.io) | - | 32 |
| [KubeVirt](https://kubevirt.io) | `dynamic` | 27 |
| [NetObserv](https://netobserv.io) | `dynamic` | 4 |
| [Tekton](https://tekton.dev) | `dynamic` | 9 |

<!-- VALIDATED-PROJECTS-END -->

### Available Toolsets

The following sets of tools are available (toolsets marked with ✓ in the Default column are enabled by default):

<!-- AVAILABLE-TOOLSETS-START -->

| Toolset               | Description                                                                                                                                                            | Default |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------|
| config                | View and manage the current local Kubernetes configuration (kubeconfig)                                                                                                | ✓       |
| core                  | Most common tools for Kubernetes management (Pods, Generic Resources, Events, etc.)                                                                                    | ✓       |
| dynamic               | Tools and prompts defined declaratively as Tool and Prompt custom resources (tools.kubernetes-mcp-server.io) installed in the cluster, see docs/specs/dynamic-tools.md |         |
| helm                  | Tools for managing Helm charts and releases                                                                                                                            |         |
| observability/logs    | Toolset for querying Loki logs                                                                                                                                         |         |
| observability/metrics | Toolset for querying Prometheus and Alertmanager endpoints in efficient ways.                                                                                          |         |
| observability/otelcol | Toolset for OpenTelemetry Collector configuration assistance including schema validation, component documentation, and version management.                             |         |
| observability/traces  | Distributed tracing tools for discovering Tempo instances, searching and retrieving traces, and exploring trace attributes.                                            |         |

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

- **events_list** - List Kubernetes events (warnings, errors, state changes) for debugging and troubleshooting in the current cluster from all namespaces
  - `fieldSelector` (`string`) - Optional Kubernetes field selector to filter events by field values (e.g. 'type=Warning', 'involvedObject.name=my-pod'). Supported fields: involvedObject.kind, involvedObject.name, involvedObject.namespace, involvedObject.uid, involvedObject.apiVersion, involvedObject.resourceVersion, involvedObject.fieldPath, reason, reportingComponent, source, type. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/
  - `namespace` (`string`) - Optional Namespace to retrieve the events from. If not provided, will list events from all namespaces

- **namespaces_list** - List all the Kubernetes namespaces in the current cluster
  - `fieldSelector` (`string`) - Optional Kubernetes field selector to filter namespaces by field values (e.g. 'metadata.name=default', 'status.phase=Active'). Supported fields: metadata.name, status.phase. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/

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

- **pods_exec** - Execute a command in a Kubernetes Pod (shell access, run commands in container) in the current or provided namespace with the provided name and command
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

- **resources_create_or_update** - Create or update a Kubernetes resource via Server-Side Apply. The manifest is the complete desired state: any field this tool previously set and the new manifest omits is removed. To edit an existing resource, fetch it with resources_get, modify it, then re-apply the full resource.
(common apiVersion and kind include: v1 Pod, v1 Service, v1 Node, apps/v1 Deployment, networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)
  - `resource` (`string`) **(required)** - Complete YAML or JSON representation of the Kubernetes resource (full desired state, not a partial patch). Include apiVersion, kind, metadata, and the full spec.

- **resources_delete** - Delete a Kubernetes resource in the current cluster by providing its apiVersion, kind, optionally the namespace, and its name
(common apiVersion and kind include: v1 Pod, v1 Service, v1 Node, apps/v1 Deployment, networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)
  - `apiVersion` (`string`) **(required)** - apiVersion of the resource (examples of valid apiVersion are: v1, apps/v1, networking.k8s.io/v1)
  - `gracePeriodSeconds` (`integer`) - Optional duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used
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

<summary>dynamic</summary>

</details>

<details>

<summary>helm</summary>

- **helm_install** - Install (deploy) a Helm chart to create a release in the current or provided namespace
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

<details>

<summary>observability/logs</summary>

- **loki_list_instances** - List LokiStack instances available in the Kubernetes cluster.
Call this first when using Loki Operator managed stacks so you can pass lokiNamespace and lokiName to other Loki tools.

- **loki_label_names** - List available Loki label names for a time range. Use this before writing LogQL queries.
  - `end` (`string`) - End time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).
  - `lokiName` (`string`) - Name of the LokiStack. Use loki_list_instances to discover valid values.
  - `lokiNamespace` (`string`) - Kubernetes namespace of the LokiStack. Use loki_list_instances to discover valid values.
  - `start` (`string`) - Start time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).
  - `tenant` (`string`) - Loki tenant ID (X-Scope-OrgID). For LokiStack gateway modes (e.g. openshift-network) this selects the `/api/logs/v1/<tenant>` path; use `network` for openshift-network.

- **loki_label_values** - List possible values for a Loki label key. Use this to build precise label matchers in LogQL.
  - `end` (`string`) - End time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).
  - `label` (`string`) **(required)** - Label key to inspect (for example namespace, pod, container).
  - `lokiName` (`string`) - Name of the LokiStack. Use loki_list_instances to discover valid values.
  - `lokiNamespace` (`string`) - Kubernetes namespace of the LokiStack. Use loki_list_instances to discover valid values.
  - `start` (`string`) - Start time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).
  - `tenant` (`string`) - Loki tenant ID (X-Scope-OrgID). For LokiStack gateway modes (e.g. openshift-network) this selects the `/api/logs/v1/<tenant>` path; use `network` for openshift-network.

- **loki_query_range** - Execute a Loki LogQL range query and return matching log streams and lines.

Use precise label matchers and a short time window first.
  - `direction` (`string`) - Search direction: backward (default) or forward.
  - `duration` (`string`) - Lookback duration from now when start/end are omitted (for example 5m, 1h). Defaults to 15m.
  - `end` (`string`) - End time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).
  - `limit` (`integer`) - Maximum number of log lines to return. Defaults to 100, max 1000.
  - `lokiName` (`string`) - Name of the LokiStack. Use loki_list_instances to discover valid values.
  - `lokiNamespace` (`string`) - Kubernetes namespace of the LokiStack. Use loki_list_instances to discover valid values.
  - `query` (`string`) **(required)** - LogQL query string.
  - `start` (`string`) - Start time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).
  - `tenant` (`string`) - Loki tenant ID (X-Scope-OrgID). For LokiStack gateway modes (e.g. openshift-network) this selects the `/api/logs/v1/<tenant>` path; use `network` for openshift-network.

</details>

<details>

<summary>observability/metrics</summary>

- **list_metrics** - MANDATORY FIRST STEP: List all available metric names in Prometheus.

YOU MUST CALL THIS TOOL BEFORE ANY OTHER QUERY TOOL

This tool MUST be called first for EVERY observability question to:
1. Discover what metrics actually exist in this environment
2. Find the EXACT metric name to use in queries
3. Avoid querying non-existent metrics
4. The 'name_regex' parameter should always be provided, and be a best guess of what the metric would be named like.
5. Do not use a blanket regex like .* or .+ in the 'name_regex' parameter. Use specific ones like kube.*, node.*, etc.

REGEX PATTERN GUIDANCE:
- Prometheus metrics are typically prefixed (e.g., 'prometheus_tsdb_head_series', 'kube_pod_status_phase')
- To match metrics CONTAINING a substring, use wildcards: '.*tsdb.*' matches 'prometheus_tsdb_head_series'
- Without wildcards, the pattern matches EXACTLY: 'tsdb' only matches a metric literally named 'tsdb' (which rarely exists)
- Common patterns: 'kube_pod.*' (pods), '.*memory.*' (memory-related), 'node_.*' (node metrics)
- If you get empty results, try adding '.*' before/after your search term

NEVER skip this step. NEVER guess metric names. Metric names vary between environments.

After calling this tool:
1. Search the returned list for relevant metrics
2. Use the EXACT metric name found in subsequent queries
3. If no relevant metric exists, inform the user
  - `name_regex` (`string`) **(required)** - Regex pattern to filter metric names. IMPORTANT: Metric names are typically prefixed (e.g., 'prometheus_tsdb_head_series'). Use wildcards to match substrings: '.*tsdb.*' matches any metric containing 'tsdb', while 'tsdb' only matches the exact string 'tsdb'. Examples: 'http_.*' (starts with http_), '.*memory.*' (contains memory), 'node_.*' (starts with node_). This parameter is required. Don't pass in blanket regex like '.*' or '.+'.

- **execute_instant_query** - Execute a PromQL instant query to get current/point-in-time values.

PREREQUISITE: You MUST call list_metrics first to verify the metric exists

WHEN TO USE:
- Current state questions: "What is the current error rate?"
- Point-in-time snapshots: "How many pods are running?"
- Latest values: "Which pods are in Pending state?"

The 'query' parameter MUST use metric names that were returned by list_metrics.
  - `query` (`string`) **(required)** - PromQL query string using metric names verified via list_metrics
  - `time` (`string`) - Evaluation time as RFC3339 or Unix timestamp. Omit or use 'NOW' for current time.

- **execute_range_query** - Execute a PromQL range query to get time-series data over a period.

PREREQUISITE: You MUST call list_metrics first to verify the metric exists

WHEN TO USE:
- Trends over time: "What was CPU usage over the last hour?"
- Rate calculations: "How many requests per second?"
- Historical analysis: "Were there any restarts in the last 5 minutes?"

TIME PARAMETERS:
- 'duration': Look back from now (e.g., "5m", "1h", "24h")
- 'step': Data point resolution (e.g., "1m" for 1-hour duration, "5m" for 24-hour duration)

The 'query' parameter MUST use metric names that were returned by list_metrics.
  - `duration` (`string`) - Duration to look back from now (e.g., '1h', '30m', '1d', '2w') (optional)
  - `end` (`string`) - End time as RFC3339 or Unix timestamp (optional). Use `NOW` for current time.
  - `query` (`string`) **(required)** - PromQL query string using metric names verified via list_metrics
  - `start` (`string`) - Start time as RFC3339 or Unix timestamp (optional)
  - `step` (`string`) **(required)** - Query resolution step width (e.g., '15s', '1m', '1h'). Choose based on time range: shorter ranges use smaller steps.

- **show_timeseries** - Display the results as an interactive timeseries chart.

This tool works like execute_range_query but renders the results as a visual chart in the UI clients.
Use it when the user wants to see a graph or visualization of time-series data and to use visuals to provide the answer.
Use the show_timeseries as the last tool call after all the other Prometheus tool calls where finalized.

TIME PARAMETERS:
- 'duration': Look back from now (e.g., "5m", "1h", "24h")
- 'step': Data point resolution (e.g., "1m" for 1-hour duration, "5m" for 24-hour duration)
- 'title': A descriptive chart title (e.g., "API Error Rate Over Last Hour")
- 'description': An explanation of the chart's meaning or context (e.g., "Shows the rate of HTTP 5xx errors per second, broken down by pod")

The 'query' parameter MUST be a range query and must use metric names that were returned by list_metrics.
  - `description` (`string`) - Explanation of the chart's meaning or context (e.g., 'Shows the rate of HTTP 5xx errors per second, broken down by pod'). Displayed below the title when provided.
  - `duration` (`string`) - Duration to look back from now (e.g., '1h', '30m', '1d', '2w') (optional)
  - `end` (`string`) - End time as RFC3339 or Unix timestamp (optional). Use `NOW` for current time.
  - `query` (`string`) **(required)** - PromQL query string using metric names verified via list_metrics
  - `start` (`string`) - Start time as RFC3339 or Unix timestamp (optional)
  - `step` (`string`) **(required)** - Query resolution step width (e.g., '15s', '1m', '1h'). Choose based on time range: shorter ranges use smaller steps.
  - `title` (`string`) - Human-readable chart title describing what the query shows (e.g., 'API Error Rate Over Last Hour'). Displayed above the chart when provided.

- **get_label_names** - Get all label names (dimensions) available for filtering a metric.

WHEN TO USE (after calling list_metrics):
- To discover how to filter metrics (by namespace, pod, service, etc.)
- Before constructing label matchers in PromQL queries

The 'metric' parameter should use a metric name from list_metrics output.
  - `end` (`string`) - End time for label discovery as RFC3339 or Unix timestamp (optional, defaults to now)
  - `metric` (`string`) - Metric name (from list_metrics) to get label names for. Leave empty for all metrics.
  - `start` (`string`) - Start time for label discovery as RFC3339 or Unix timestamp (optional, defaults to 1 hour ago)

- **get_label_values** - Get all unique values for a specific label.

WHEN TO USE (after calling list_metrics and get_label_names):
- To find exact label values for filtering (namespace names, pod names, etc.)
- To see what values exist before constructing queries

The 'metric' parameter should use a metric name from list_metrics output.
  - `end` (`string`) - End time for label value discovery as RFC3339 or Unix timestamp (optional, defaults to now)
  - `label` (`string`) **(required)** - Label name (from get_label_names) to get values for
  - `metric` (`string`) - Metric name (from list_metrics) to scope the label values to. Leave empty for all metrics.
  - `start` (`string`) - Start time for label value discovery as RFC3339 or Unix timestamp (optional, defaults to 1 hour ago)

- **get_series** - Get time series matching selectors and preview cardinality.

WHEN TO USE (optional, after calling list_metrics):
- To verify label filters match expected series before querying
- To check cardinality and avoid slow queries

CARDINALITY GUIDANCE:
- <100 series: Safe
- 100-1000: Usually fine
- >1000: Add more label filters

The selector should use metric names from list_metrics output.
  - `end` (`string`) - End time for series discovery as RFC3339 or Unix timestamp (optional, defaults to now)
  - `matches` (`string`) **(required)** - PromQL series selector using metric names from list_metrics
  - `start` (`string`) - Start time for series discovery as RFC3339 or Unix timestamp (optional, defaults to 1 hour ago)

- **get_alerts** - Get alerts from Alertmanager.

WHEN TO USE:
- START HERE when investigating issues: if the user asks about things breaking, errors, failures, outages, services being down, or anything going wrong in the cluster
- When the user mentions a specific alert name - use this tool to get the alert's full labels (namespace, pod, service, etc.) which are essential for further investigation with other tools
- To see currently firing alerts in the cluster
- To check which alerts are active, silenced, or inhibited
- To understand what's happening before diving into metrics or logs

INVESTIGATION TIP: Alert labels often contain the exact identifiers (pod names, namespaces, job names) needed for targeted queries with prometheus tools.

FILTERING:
- Use 'active' to filter for only active alerts (not resolved)
- Use 'silenced' to filter for silenced alerts
- Use 'inhibited' to filter for inhibited alerts
- Use 'filter' to apply label matchers (e.g., "alertname=HighCPU")
- Use 'receiver' to filter alerts by receiver name

All filter parameters are optional. Without filters, all alerts are returned.
  - `active` (`boolean`) - Filter for active alerts only (true/false, optional)
  - `filter` (`string`) - Label matchers to filter alerts (e.g., 'alertname=HighCPU', optional)
  - `inhibited` (`boolean`) - Filter for inhibited alerts only (true/false, optional)
  - `receiver` (`string`) - Receiver name to filter alerts (optional)
  - `silenced` (`boolean`) - Filter for silenced alerts only (true/false, optional)
  - `unprocessed` (`boolean`) - Filter for unprocessed alerts only (true/false, optional)

- **get_silences** - Get silences from Alertmanager.

WHEN TO USE:
- To see which alerts are currently silenced
- To check active, pending, or expired silences
- To investigate why certain alerts are not firing notifications

FILTERING:
- Use 'filter' to apply label matchers to find specific silences

Silences are used to temporarily mute alerts based on label matchers. This tool helps you understand what is currently silenced in your environment.
  - `filter` (`string`) - Label matchers to filter silences (e.g., 'alertname=HighCPU', optional)

</details>

<details>

<summary>observability/otelcol</summary>

- **otelcol_list_components** - List available OpenTelemetry Collector components (receivers, processors, exporters, extensions, connectors) for a given version.
  - `version` (`string`) - Collector version (e.g., 'v0.100.0'). Defaults to latest available.

- **otelcol_get_component_schema** - Get the JSON schema for an OpenTelemetry Collector component's configuration options.
  - `component_name` (`string`) **(required)** - Component name from otelcol_list_components (e.g., 'otlp', 'batch', 'debug')
  - `component_type` (`string`) **(required)** - Component type: receiver, processor, exporter, extension, connector
  - `version` (`string`) - Collector version (e.g., 'v0.100.0'). Defaults to latest available.

- **otelcol_validate_config** - Validate an OpenTelemetry Collector component configuration against its JSON schema.
  - `component_name` (`string`) **(required)** - Component name from otelcol_list_components (e.g., 'otlp', 'batch', 'debug')
  - `component_type` (`string`) **(required)** - Component type: receiver, processor, exporter, extension, connector
  - `config` (`string`) **(required)** - Configuration to validate as YAML or JSON string
  - `format` (`string`) - Config format: 'yaml' (default) or 'json'
  - `version` (`string`) - Collector version (e.g., 'v0.100.0'). Defaults to latest available.

- **otelcol_get_versions** - List available OpenTelemetry Collector versions and identify the latest.

</details>

<details>

<summary>observability/traces</summary>

- **tempo_list_instances** - List all Tempo instances available in the Kubernetes cluster.
Call this tool first to discover available Tempo instances before using other Tempo tools,
as the returned namespace, name, and tenant values are required parameters for all other Tempo tools.
Always print the output of this tool in a table.

- **tempo_get_trace_by_id** - Retrieve a single distributed trace by its trace ID from Tempo.
Returns the full trace with all its spans, including service names, operation names, durations, and attributes.
Use this tool when you already have a specific trace ID, e.g. from search results or logs.
  - `end` (`string`) - Optional end of the time range in RFC 3339 format, e.g. "2025-01-02T00:00:00Z".
Narrows the time range to improve query performance.
  - `start` (`string`) - Optional start of the time range in RFC 3339 format, e.g. "2025-01-01T00:00:00Z".
Narrows the time range to improve query performance.
  - `tempoName` (`string`) **(required)** - The name of the Tempo instance to query. Use tempo_list_instances to discover available instance names.
  - `tempoNamespace` (`string`) **(required)** - The Kubernetes namespace where the Tempo instance is deployed. Use tempo_list_instances to discover available namespaces.
  - `tenant` (`string`) - The tenant to query. This parameter is required for multi-tenant instances. Use tempo_list_instances to discover available tenants for each instance.
  - `traceid` (`string`) **(required)** - The trace ID to retrieve, e.g. "26dad4a0e2b0dd9a440dd5ff203a24a4".

- **tempo_search_traces** - Search for distributed traces in Tempo using TraceQL.
Use this tool to find traces matching specific criteria such as service name, HTTP status code, duration, or other span or resource attributes.

IMPORTANT — "slow" or "long" trace requests: Do NOT guess a duration threshold.
First call this tool WITHOUT a duration filter to establish a latency baseline, then use that baseline to set a sensible threshold.
Both steps are required — do NOT skip the second search with the duration filter.
Skip this two-step process only when the user provides an explicit duration (e.g. "find traces slower than 2s").

  - `end` (`string`) - End of the time range in RFC 3339 format, e.g. "2025-01-01T00:00:00Z".
Use "NOW" for current time.
Both start and end should be provided to search the full time range; if omitted, only a small window of recent data is searched.
  - `limit` (`integer`) - Maximum number of traces to return. Defaults to the server-side limit if not specified.
  - `query` (`string`) **(required)** - A TraceQL query expression. Format:
query: "{ <filters joined by &&> }"

Filters:
- service name:     resource.service.name="<value>" (string, use quotes)
- HTTP status code: span.http.response.status_code=<code> (number, no quotes)
- duration:         duration><value like 100ms, 2s, 5m> (no quotes)
- error status:     status=error (keyword, NO quotes — do NOT write status="error")

IMPORTANT: status values (error, ok, unset) are keywords, NOT strings. Write status=error, NEVER status="error".

Operators: =, !=, >, <, >=, <=

Common attributes:
- resource.service.name (service name)
- resource.k8s.namespace.name (Kubernetes namespace)
- resource.k8s.deployment.name (Kubernetes deployment)
- resource.k8s.statefulset.name (Kubernetes statefulset)
- resource.k8s.daemonset.name (Kubernetes daemonset)
- resource.k8s.replicaset.name (Kubernetes replicaset)
- resource.k8s.pod.name (Kubernetes pod)
- resource.k8s.container.name (Kubernetes container)
- resource.k8s.job.name (Kubernetes job)
- resource.k8s.cronjob.name (Kubernetes cronjob)
- resource.k8s.node.name (Kubernetes node)
- resource.k8s.cluster.name (Kubernetes cluster)
- span.http.response.status_code (HTTP response code)
- span.http.request.method (HTTP method like GET, POST)
- span.url.full (request URL)
- name (span name / operation name, e.g. "GET /api/users")
- duration (trace duration, e.g. 100ms, 2s)
- status (trace status: ok, error, unset)

Note: older instrumentation may use legacy HTTP attribute names (e.g. span.http.status_code instead of span.http.response.status_code).
If a query returns no results, try tempo_search_tags to check which attributes exist.

IMPORTANT:
- Always wrap filters in curly braces { }.
- Do NOT use SQL, PromQL, or Lucene syntax.
- Do NOT omit the "resource." or "span." prefix from attribute names
- When the user refers to a Kubernetes resource type (deployment, pod, namespace, etc.), use the matching resource.k8s.* attribute, NOT resource.service.name.

Examples:
- { resource.service.name="frontend" }
- { resource.k8s.deployment.name="checkout" && span.http.response.status_code>=500 }
- { status=error && duration>2s }

If unsure which attributes to filter on, use tempo_search_tags to discover available attributes before building a query.

  - `spss` (`integer`) - Maximum number of matching spans to return per trace.
  - `start` (`string`) - Start of the time range in RFC 3339 format, e.g. "2025-01-01T00:00:00Z".
Use "NOW" for current time.
Both start and end should be provided to search the full time range; if omitted, only a small window of recent data is searched.
  - `tempoName` (`string`) **(required)** - The name of the Tempo instance to query. Use tempo_list_instances to discover available instance names.
  - `tempoNamespace` (`string`) **(required)** - The Kubernetes namespace where the Tempo instance is deployed. Use tempo_list_instances to discover available namespaces.
  - `tenant` (`string`) - The tenant to query. This parameter is required for multi-tenant instances. Use tempo_list_instances to discover available tenants for each instance.

- **tempo_search_tags** - List available tag names (attribute keys) in Tempo, grouped by scope.
Use this tool to discover which attributes are available for building TraceQL queries with tempo_search_traces.
For example, this tool may reveal tag names like "service.name" (in the "resource" scope) or "http.response.status_code" (in the "span" scope).
To use these in TraceQL queries, prefix them with their scope, e.g. "resource.service.name" or "span.http.response.status_code".
  - `end` (`string`) - Optional end of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing tags.
  - `limit` (`integer`) - Maximum number of tag names to return per scope.
  - `maxStaleValues` (`integer`) - Maximum number of consecutive blocks without new tag names before the search stops early. Higher values are more thorough but slower.
  - `query` (`string`) - Optional TraceQL query to filter which traces are considered when listing tags,
e.g. '{ resource.service.name="payment-service" }' to only show tags present in traces from the 'payment-service' service.
  - `scope` (`string`) - Filter tags to a specific scope. One of:
"resource" (service-level attributes like service.name),
"span" (individual span attributes like http.response.status_code),
"intrinsic" (built-in fields like duration, status, name).
If omitted, tags from all scopes are returned.
  - `start` (`string`) - Optional start of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing tags.
  - `tempoName` (`string`) **(required)** - The name of the Tempo instance to query. Use tempo_list_instances to discover available instance names.
  - `tempoNamespace` (`string`) **(required)** - The Kubernetes namespace where the Tempo instance is deployed. Use tempo_list_instances to discover available namespaces.
  - `tenant` (`string`) - The tenant to query. This parameter is required for multi-tenant instances. Use tempo_list_instances to discover available tenants for each instance.

- **tempo_search_tag_values** - List the known values for a specific tag (attribute key) in Tempo.
Use this tool to discover what values exist for a given tag, e.g. to find all service names (values of "resource.service.name") or all HTTP methods (values of "span.http.request.method").
This is useful for building accurate TraceQL queries with tempo_search_traces.
  - `end` (`string`) - Optional end of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing values.
  - `limit` (`integer`) - Maximum number of tag values to return.
  - `maxStaleValues` (`integer`) - Maximum number of consecutive blocks without new values before the search stops early. Higher values are more thorough but slower.
  - `query` (`string`) - Optional TraceQL query to filter which traces are considered when listing values,
e.g. '{ resource.service.name="payment-service" }' to only show tag values from the 'payment-service' service.
  - `start` (`string`) - Optional start of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing values.
  - `tag` (`string`) **(required)** - The fully qualified tag name to get values for, including its scope prefix, e.g. "resource.service.name" or "span.http.response.status_code".
Use tempo_search_tags to discover available tag names.
  - `tempoName` (`string`) **(required)** - The name of the Tempo instance to query. Use tempo_list_instances to discover available instance names.
  - `tempoNamespace` (`string`) **(required)** - The Kubernetes namespace where the Tempo instance is deployed. Use tempo_list_instances to discover available namespaces.
  - `tenant` (`string`) - The tenant to query. This parameter is required for multi-tenant instances. Use tempo_list_instances to discover available tenants for each instance.

</details>


<!-- AVAILABLE-TOOLSETS-TOOLS-END -->

### Prompts

<!-- AVAILABLE-TOOLSETS-PROMPTS-START -->

<details>

<summary>core</summary>

- **cluster-health-check** - Perform comprehensive health assessment of Kubernetes/OpenShift cluster
  - `namespace` (`string`) - Optional namespace to limit health check scope (default: all namespaces)
  - `check_events` (`string`) - Include recent warning/error events (true/false, default: true)

</details>


<!-- AVAILABLE-TOOLSETS-PROMPTS-END -->

### Resources

<!-- AVAILABLE-TOOLSETS-RESOURCES-START -->


<!-- AVAILABLE-TOOLSETS-RESOURCES-END -->

### Resource Templates

<!-- AVAILABLE-TOOLSETS-RESOURCES-TEMPLATES-START -->


<!-- AVAILABLE-TOOLSETS-RESOURCES-TEMPLATES-END -->

## Helm Chart

A [Helm Chart](https://helm.sh) is available to simplify the deployment of the Kubernetes MCP server.

```shell
helm install kubernetes-mcp-server oci://ghcr.io/containers/charts/kubernetes-mcp-server
```

For configuration options including OAuth, telemetry, and resource limits, see the [chart README](./charts/kubernetes-mcp-server/README.md) and [values.yaml](./charts/kubernetes-mcp-server/values.yaml).

## 💬 Community <a id="community"></a>

Join the conversation and connect with other users and contributors:

- [Slack](https://cloud-native.slack.com/archives/C0AHQJVR725) - Ask questions, share feedback, and discuss the Kubernetes MCP server in the `#kubernetes-mcp-server` channel on the CNCF Slack workspace. If you're not already a member, you can [request an invitation](https://slack.cncf.io).

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
