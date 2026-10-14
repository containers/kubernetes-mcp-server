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
| observability/otelcol | Toolset for OpenTelemetry Collector configuration assistance including schema validation, component documentation, and version management.                             |         |

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
