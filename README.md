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
- **🧩 Ecosystem tools as data**: Kiali, NetObserv, Tekton, KubeVirt, Loki, Prometheus and Tempo tools and prompts are delivered as `Tool` and `Prompt` custom resources (Helm charts under [`charts/dynamic-tools`](charts/dynamic-tools)) and picked up at runtime by the `dynamic` toolset. See [docs/specs/dynamic-tools.md](docs/specs/dynamic-tools.md).
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

- **context_list** - List the targets this server can run tools against. Named cluster_list when the server runs against an ACM-style provider.

- **configuration_view** - Show the kubeconfig the server runs with, as YAML: the current context alone by default, every context with minified false. Credentials in it are returned as they are.
  - `minified` (`boolean`) - Return a minified version of the configuration. If set to true, keeps only the current-context and the relevant pieces of the configuration for that context. If set to false, all contexts, clusters, auth-infos, and users are returned in the configuration. (Optional, default true)

</details>

<details>

<summary>core</summary>

- **events_list** - List the events of a namespace, or of the whole cluster when namespace is omitted: warnings, errors and state changes, each with the object it is about. Narrow with fieldSelector, e.g. type=Warning or involvedObject.name=<pod>.
  - `fieldSelector` (`string`) - Optional Kubernetes field selector to filter events by field values (e.g. 'type=Warning', 'involvedObject.name=my-pod'). Supported fields: involvedObject.kind, involvedObject.name, involvedObject.namespace, involvedObject.uid, involvedObject.apiVersion, involvedObject.resourceVersion, involvedObject.fieldPath, reason, reportingComponent, source, type. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/
  - `namespace` (`string`) - Optional Namespace to retrieve the events from. If not provided, will list events from all namespaces

- **namespaces_list** - List the namespaces of the cluster with their status and age. Narrow with fieldSelector on metadata.name or status.phase.
  - `fieldSelector` (`string`) - Optional Kubernetes field selector to filter namespaces by field values (e.g. 'metadata.name=default', 'status.phase=Active'). Supported fields: metadata.name, status.phase. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/

- **projects_list** - List the OpenShift projects: the namespaces as OpenShift presents them, with display names and descriptions. On a cluster without OpenShift, use namespaces_list.

- **nodes_log** - Read the system logs of a node through the kubelet: a service such as kubelet, or a log file under /var/log. The last 100 lines by default; tail 0 returns everything.
  - `name` (`string`) **(required)** - Name of the node to get logs from
  - `source` (`string`) **(required)** - A service name ('kubelet', 'kube-proxy') or a log file path on the node ('/var/log/kubelet.log').
  - `tail` (`integer`) - Number of lines to retrieve from the end of the logs (Optional, 0 means all logs)

- **nodes_stats_summary** - Show the kubelet's resource statistics of a node: CPU, memory, filesystem and network at the node, pod and container level, system containers included. nodes_top is the short CPU and memory table.
  - `name` (`string`) **(required)** - Name of the node to get stats from

- **nodes_top** - Show the CPU and memory consumption of the nodes as recorded by the Metrics Server: every node, one node with name, or the nodes matching labelSelector.
  - `labelSelector` (`string`) - Kubernetes label selector (e.g. 'node-role.kubernetes.io/worker=') to filter nodes by label (Optional, only applicable when name is not provided)
  - `name` (`string`) - Name of the Node to get the resource consumption from (Optional, all Nodes if not provided)

- **pods_list** - List the pods of a namespace, or of the whole cluster when namespace is omitted, with their status, restarts and age. Narrow with labelSelector or fieldSelector; pods_get shows one pod in full.
  - `fieldSelector` (`string`) - Optional Kubernetes field selector to filter pods by field values (e.g. 'status.phase=Running', 'spec.nodeName=node1'). Supported fields: metadata.name, metadata.namespace, spec.nodeName, spec.restartPolicy, spec.schedulerName, spec.serviceAccountName, status.phase (Pending/Running/Succeeded/Failed/Unknown), status.podIP, status.nominatedNodeName. Note: CrashLoopBackOff is a container state, not a pod phase, so it cannot be filtered directly. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/
  - `labelSelector` (`string`) - Optional Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label
  - `namespace` (`string`) - Namespace to list pods from. Omit it to list the pods of every namespace

- **pods_get** - Show one pod in full: its spec, status, conditions and container states. Find it with pods_list; read its output with pods_log.
  - `name` (`string`) **(required)** - Name of the Pod
  - `namespace` (`string`) - Namespace to get the Pod from

- **pods_delete** - Delete one pod. A pod owned by a Deployment or another controller is recreated; a bare pod is gone.
  - `name` (`string`) **(required)** - Name of the Pod to delete
  - `namespace` (`string`) - Namespace to delete the Pod from

- **pods_top** - Show the CPU and memory consumption of pods as recorded by the Kubernetes Metrics Server: one pod, the pods of a namespace, or every pod of the cluster when namespace is omitted
  - `labelSelector` (`string`) - Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the pods by label (Optional, only applicable when name is not provided)
  - `name` (`string`) - Name of one pod in the namespace (Optional, all pods if not provided)
  - `namespace` (`string`) - Namespace of the pods. Omit it for the pods of every namespace

- **pods_exec** - Run a command inside a container of a pod and return its output, as kubectl exec does. command is the program and its arguments as a list; container picks one when the pod has several.
  - `command` (`array`) **(required)** - Command to execute in the Pod container. The first item is the command to be run, and the rest are the arguments to that command. Example: ["ls", "-l", "/tmp"]
  - `container` (`string`) - Name of the Pod container where the command will be executed (Optional)
  - `name` (`string`) **(required)** - Name of the Pod where the command will be executed
  - `namespace` (`string`) - Namespace of the Pod where the command will be executed

- **pods_log** - Read the logs of a pod, or of one of its containers: the last 100 lines by default, and with previous the logs of the container that crashed before the last restart.
  - `container` (`string`) - Name of the Pod container to get the logs from (Optional)
  - `name` (`string`) **(required)** - Name of the Pod to get the logs from
  - `namespace` (`string`) - Namespace to get the Pod logs from
  - `previous` (`boolean`) - Return previous terminated container logs (Optional)
  - `tail` (`integer`) - Number of lines to retrieve from the end of the logs (Optional, default: 100)

- **pods_run** - Create a pod from a container image, with a Service (and a Route on OpenShift) for its port when one is given. For anything beyond a quick test, create a Deployment with resources_create_or_update.
  - `image` (`string`) **(required)** - Container Image to run in the Pod
  - `name` (`string`) - Name of the Pod (Optional, random name if not provided)
  - `namespace` (`string`) - Namespace to run the Pod in
  - `port` (`number`) - TCP/IP port to expose from the Pod container (Optional, no port exposed if not provided)

- **resources_list** - List the objects of any kind by apiVersion and kind, in one namespace or, when namespace is omitted, across the cluster, with their names and status columns. Narrow with labelSelector or fieldSelector. (common apiVersion and kind include: v1 Pod, v1 Service, v1 Node, apps/v1 Deployment, networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)
  - `apiVersion` (`string`) **(required)** - apiVersion of the resources (examples of valid apiVersion are: v1, apps/v1, networking.k8s.io/v1)
  - `fieldSelector` (`string`) - Optional Kubernetes field selector to filter resources by field values (e.g. 'status.phase=Running', 'metadata.name=myresource'). Supported fields vary by resource type. For Pods: metadata.name, metadata.namespace, spec.nodeName, spec.restartPolicy, spec.schedulerName, spec.serviceAccountName, status.phase (Pending/Running/Succeeded/Failed/Unknown), status.podIP, status.nominatedNodeName. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/
  - `kind` (`string`) **(required)** - kind of the resources (examples of valid kind are: Pod, Service, Deployment, Ingress)
  - `labelSelector` (`string`) - Optional Kubernetes label selector (e.g. 'app=myapp,env=prod' or 'app in (myapp,yourapp)'), use this option when you want to filter the resources by label
  - `namespace` (`string`) - Optional Namespace to retrieve the namespaced resources from (ignored in case of cluster scoped resources). If not provided, will list resources from all namespaces

- **resources_get** - Show one object of any kind in full, by apiVersion, kind, name and, for a namespaced kind, namespace. (common apiVersion and kind include: v1 Pod, v1 Service, v1 Node, apps/v1 Deployment, networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)
  - `apiVersion` (`string`) **(required)** - apiVersion of the resource (examples of valid apiVersion are: v1, apps/v1, networking.k8s.io/v1)
  - `kind` (`string`) **(required)** - kind of the resource (examples of valid kind are: Pod, Service, Deployment, Ingress)
  - `name` (`string`) **(required)** - Name of the resource
  - `namespace` (`string`) - Optional Namespace to retrieve the namespaced resource from (ignored in case of cluster scoped resources). If not provided, will get resource from configured namespace

- **resources_create_or_update** - Create or update a Kubernetes resource via Server-Side Apply. The manifest is the complete desired state: any field this tool previously set and the new manifest omits is removed. To edit an existing resource, fetch it with resources_get, modify it, then re-apply the full resource.
(common apiVersion and kind include: v1 Pod, v1 Service, v1 Node, apps/v1 Deployment, networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)
  - `resource` (`string`) **(required)** - Complete YAML or JSON representation of the Kubernetes resource (full desired state, not a partial patch). Include apiVersion, kind, metadata, and the full spec.

- **resources_delete** - Delete one object of any kind by apiVersion, kind, name and namespace. gracePeriodSeconds shortens or lengthens the time the object gets to shut down; 0 deletes it immediately. (common apiVersion and kind include: v1 Pod, v1 Service, v1 Node, apps/v1 Deployment, networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)
  - `apiVersion` (`string`) **(required)** - apiVersion of the resource (examples of valid apiVersion are: v1, apps/v1, networking.k8s.io/v1)
  - `gracePeriodSeconds` (`integer`) - Optional duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used
  - `kind` (`string`) **(required)** - kind of the resource (examples of valid kind are: Pod, Service, Deployment, Ingress)
  - `name` (`string`) **(required)** - Name of the resource
  - `namespace` (`string`) - Optional Namespace to delete the namespaced resource from (ignored in case of cluster scoped resources). If not provided, will delete resource from configured namespace

- **resources_scale** - Show the replica count of a Deployment, StatefulSet or ReplicaSet, or set it when replicas is given.
  - `apiVersion` (`string`) **(required)** - apiVersion of the resource (examples of valid apiVersion are apps/v1)
  - `kind` (`string`) **(required)** - kind of the resource (examples of valid kind are: StatefulSet, Deployment)
  - `name` (`string`) **(required)** - Name of the resource
  - `namespace` (`string`) - Optional Namespace to get/update the namespaced resource scale from (ignored in case of cluster scoped resources). If not provided, will get/update resource scale from configured namespace
  - `replicas` (`integer`) - The replica count to set. Omit it to read the current count without changing it.

</details>

<details>

<summary>dynamic</summary>

</details>

<details>

<summary>helm</summary>

- **helm_install** - Install a Helm chart as a release, with values when given, in a namespace or the current one. The release name is generated when omitted.
  - `chart` (`string`) **(required)** - Chart reference to install (for example: stable/grafana, oci://ghcr.io/nginxinc/charts/nginx-ingress)
  - `name` (`string`) - Name of the Helm release (Optional, random name if not provided)
  - `namespace` (`string`) - Namespace to install the Helm chart in (Optional, current namespace if not provided)
  - `values` (`object`) - Values to pass to the Helm chart (Optional)

- **helm_list** - List the Helm releases of a namespace, or of every namespace when namespace is omitted
  - `namespace` (`string`) - Namespace to list Helm releases from. Omit it for the releases of every namespace

- **helm_uninstall** - Uninstall a Helm release and the objects it created.
  - `name` (`string`) **(required)** - Name of the Helm release to uninstall
  - `namespace` (`string`) - Namespace to uninstall the Helm release from (Optional, current namespace if not provided)

</details>

<details>

<summary>observability/otelcol</summary>

- **otelcol_list_components** - List the components of an OpenTelemetry Collector version by name: receivers, processors, exporters, extensions and connectors. Defaults to the latest version.
  - `version` (`string`) - Collector version (e.g., 'v0.100.0'). Defaults to latest available.

- **otelcol_get_component_schema** - Show the JSON schema of one Collector component's configuration: every option with its type and default. Use it before writing or validating a component's config.
  - `componentName` (`string`) **(required)** - Component name from otelcol_list_components (e.g., 'otlp', 'batch', 'debug')
  - `componentType` (`string`) **(required)** - Component type: receiver, processor, exporter, extension, connector
  - `version` (`string`) - Collector version (e.g., 'v0.100.0'). Defaults to latest available.

- **otelcol_validate_config** - Validate one Collector component's configuration, as YAML or JSON, against its schema and report what is wrong. Use it before applying a Collector config.
  - `componentName` (`string`) **(required)** - Component name from otelcol_list_components (e.g., 'otlp', 'batch', 'debug')
  - `componentType` (`string`) **(required)** - Component type: receiver, processor, exporter, extension, connector
  - `config` (`string`) **(required)** - Configuration to validate as YAML or JSON string
  - `format` (`string`) - Config format: 'yaml' (default) or 'json'
  - `version` (`string`) - Collector version (e.g., 'v0.100.0'). Defaults to latest available.

- **otelcol_get_versions** - List the OpenTelemetry Collector versions whose component schemas are available, and which one is the latest.

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
