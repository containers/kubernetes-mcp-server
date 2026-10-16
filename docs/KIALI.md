## Kiali integration

Kiali tools let an MCP-capable assistant query a live service mesh, and optionally change Istio configuration, by calling tools that Kiali already implements. The assistant does not talk to Kiali directly. It talks to this MCP server, and the server calls the Kiali API with the caller's Kubernetes token.

This is separate from the Kiali console chatbot. The chatbot runs inside the Kiali UI and uses Kiali's own tools. These MCP tools are for an assistant outside Kiali, for example an IDE, that should use the same mesh data.

Enable the `kiali` toolset and the tools are named with the `kiali_` prefix. The catalog below is generated from [`pkg/mcp/testdata/toolsets-kiali-tools.json`](../pkg/mcp/testdata/toolsets-kiali-tools.json).

### Prerequisites

- A Kiali URL this server can reach (Route, Ingress, or in-cluster Service).
- Kubernetes credentials (kubeconfig, or in-cluster configuration). Kiali authorizes that token. A token that cannot list namespaces, or cannot read pods and Istio resources, will see empty or failed tool calls.
- For writes (`kiali_manage_istio_config`), permission to create, update, and delete the Istio resources you want the assistant to change. Set `read_only = true` when the assistant should not be offered that tool.

### Enable the Kiali toolset

`kiali` is not on by default. When it is enabled, `url` is required and the server will not start without it.

```toml
toolsets = ["core", "kiali"]

[toolset_configs.kiali]
url = "https://kiali.example" # Route or Ingress for the Kiali console
# insecure = true  # optional: skip TLS verification (not recommended in production)
# certificate_authority = "/path/to/ca.crt"  # CA bundle for Kiali's TLS certificate
```

If `url` is `https://` and `insecure` is false, set `certificate_authority`. Relative CA paths are resolved from the directory that contains the TOML file.

### Connect a client

How you register the server depends on the client. Start the server with the kubeconfig and the TOML file:

```bash
kubernetes-mcp-server --config /path/to/config.toml
```

Set `read_only = true` in that file when the assistant must not create, patch, or delete Istio, Gateway API, and Inference API configuration. Leave it unset only when those writes are allowed.

### How authentication works

- The server uses your existing Kubernetes credentials (from kubeconfig or in-cluster) to set a bearer token for Kiali calls.
- If you pass an HTTP Authorization header to the MCP HTTP endpoint, that is not what Kiali sees. Kiali calls use the server's configured token.
- Kiali enforces access with that token (namespace get/list and, for writes and logs, resource RBAC). Tools advertise this as [RBAC metadata](tool-rbac-metadata.md):
  - **Bounded** for tools whose Kubernetes checks are derivable from arguments or a finite kind enum (logs, resource details, Istio config read/write, metrics/traces namespace gate, traffic graph). On OpenShift, resource/graph declarations also include `deploymentconfigs` and `routes` from the operator's viewer role.
  - **Unbounded** for mesh-wide status and mesh cluster listing, where Kiali aggregates control-plane and observability backends that cannot be expressed as a finite Kubernetes requirement set.

Binding the caller's identity to the operator-provided `kiali-viewer` (read) or `kiali` (read/write) ClusterRole remains the practical way to grant the full surface those tools may need.

### Multi-cluster support

Kiali can manage multiple Kubernetes clusters within an Istio service mesh. Most Kiali tools accept an optional `meshCluster` parameter to target a specific mesh cluster. When omitted, Kiali defaults to its home cluster (where Kiali is deployed).

Use `kiali_list_mesh_clusters` to discover available mesh cluster names before calling other tools. The `name` field from that response is the only valid value for `meshCluster`.

Kiali tools and prompts are not cluster-aware: the MCP server does not inject a `context` parameter on them. Use `meshCluster` to select mesh scope. Core Kubernetes tools still use `context` when multi-cluster is enabled.

### Tools

<!-- KIALI-TOOLS:START -->
Generated from [`pkg/mcp/testdata/toolsets-kiali-tools.json`](../pkg/mcp/testdata/toolsets-kiali-tools.json). Refresh this list with `make update-kiali-docs`.

| Tool | Access | Purpose |
| --- | --- | --- |
| [`kiali_get_logs`](#kiali_get_logs) | Read-only | Workload: Logs |
| [`kiali_get_mesh_status`](#kiali_get_mesh_status) | Read-only | Get Mesh Status |
| [`kiali_get_mesh_traffic_graph`](#kiali_get_mesh_traffic_graph) | Read-only | Get Mesh Traffic Graph |
| [`kiali_get_metrics`](#kiali_get_metrics) | Read-only | Get Metrics for a Resource |
| [`kiali_get_pod_performance`](#kiali_get_pod_performance) | Read-only | Get Pod Performance |
| [`kiali_get_resource_details`](#kiali_get_resource_details) | Read-only | List or Resource Details |
| [`kiali_get_trace_details`](#kiali_get_trace_details) | Read-only | Get Trace Details |
| [`kiali_list_mesh_clusters`](#kiali_list_mesh_clusters) | Read-only | List Mesh Clusters |
| [`kiali_list_traces`](#kiali_list_traces) | Read-only | List Traces by Service Name |
| [`kiali_manage_istio_config`](#kiali_manage_istio_config) | Writes config | Manage Istio, Gateway API, and Inference API Config: Create, Patch, Delete |
| [`kiali_manage_istio_config_read`](#kiali_manage_istio_config_read) | Read-only | Manage Istio, Gateway API, and Inference API Config: List or Get |

#### `kiali_get_logs`

**Workload: Logs.** Read-only.

Get the logs of a Kubernetes Pod (or workload name that will be resolved to a pod) in a namespace. Output is plain text, matching kubernetes-mcp-server pods_log. The line_count field tells you the total number of log lines returned. Analyze ALL of them, but summarize the results unless the user explicitly asks for the raw output. Do not omit any error or warning lines.

| Parameter | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` | string | yes |  | Name of the Pod to get the logs from. If it does not exist, it will be treated as a workload name and a running pod will be selected. |
| `namespace` | string | yes |  | Namespace to get the Pod logs from |
| `container` | string | no |  | Optional. Name of the Pod container to get the logs from. |
| `format` | string (`codeblock`, `plain`) | no |  | Output formatting for chat. 'codeblock' wraps logs in ~~~ fences (recommended). 'plain' returns raw text like kubernetes-mcp-server pods_log. |
| `meshCluster` | string | no |  | Optional Istio mesh cluster name from kiali_list_mesh_clusters (e.g. west). When omitted, Kiali defaults to its home cluster. |
| `previous` | boolean | no |  | Optional. Return previous terminated container logs |
| `severity` | string | no |  | Optional severity filter applied client-side. Accepts 'ERROR', 'WARN' or combinations like 'ERROR,WARN'. |
| `tail` | integer | no | `50` | Number of lines to retrieve from the end of the logs (Optional, defaults to 50). Cannot exceed 200 lines. Minimum: 1. |
| `workload` | string | no |  | Optional. Workload name override (used when name lookup fails). |

#### `kiali_get_mesh_status`

**Get Mesh Status.** Read-only.

Retrieves the high-level health, topology, and environment details of the Istio service mesh. Returns multi-cluster control plane status (istiod), data plane namespace health (including ambient mesh status), observability stack health (Prometheus, Grafana...), and component connectivity. Use this tool as the first step to diagnose mesh-wide issues, verify Istio/Kiali versions, or check overall health before drilling into specific workloads.

This tool takes no parameters.

#### `kiali_get_mesh_traffic_graph`

**Get Mesh Traffic Graph.** Read-only.

Returns service-to-service traffic topology, dependencies, and network metrics (throughput, response time, mTLS) for the specified namespaces. Use this to diagnose routing issues, latency, or find upstream/downstream dependencies.

| Parameter | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `namespaces` | string | yes |  | Comma-separated list of namespaces to map |
| `graphType` | string (`app`, `versionedApp`, `service`, `workload`) | no | `versionedApp` | Granularity of the graph. 'app' aggregates by app name, 'versionedApp' separates by versions, 'workload' maps specific pods/deployments. Default: versionedApp. |
| `meshCluster` | string | no |  | Optional Istio mesh cluster name from kiali_list_mesh_clusters (e.g. west). When omitted, Kiali defaults to its home cluster. |

#### `kiali_get_metrics`

**Get Metrics for a Resource.** Read-only.

Returns a compact JSON summary of Istio metrics (latency quantiles, traffic trends, throughput, payload sizes) for the given resource.

| Parameter | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `namespace` | string | yes |  | Namespace to get metrics from |
| `resourceName` | string | yes |  | Name of the resource to get metrics for |
| `resourceType` | string (`service`, `workload`, `app`) | yes |  | Type of resource to get metrics |
| `byLabels` | string | no |  | Comma-separated list of labels to group metrics by (e.g., 'source_workload,destination_service'). Optional |
| `direction` | string (`inbound`, `outbound`) | no | `outbound` | Traffic direction. Optional, defaults to 'outbound' |
| `meshCluster` | string | no |  | Optional Istio mesh cluster name from kiali_list_mesh_clusters (e.g. west). When omitted, Kiali defaults to its home cluster. |
| `quantiles` | string | no | `0.5,0.95,0.99,0.999` | Comma-separated list of quantiles for histogram metrics (e.g., '0.5,0.95,0.99'). Optional |
| `rateInterval` | string | no | `10m` | Rate interval for metrics (e.g., '1m', '5m'). Optional, defaults to '10m' |
| `reporter` | string | no | `source` | Metrics reporter(s). Comma-separated list of: 'source', 'destination', 'waypoint', or the special value 'both' (no reporter filter). Optional, defaults to 'source'. Example: 'source,waypoint' |
| `requestProtocol` | string | no |  | Filter by request protocol (e.g., 'http', 'grpc', 'tcp'). Optional |
| `step` | string | no | `15` | Step between data points in seconds (e.g., '15'). Optional, defaults to 15 seconds |

#### `kiali_get_pod_performance`

**Get Pod Performance.** Read-only.

Returns a human-readable text summary with current Pod CPU/memory usage (from Prometheus) compared to Kubernetes requests/limits (from the Pod spec). Useful to answer questions like 'Is this workload using too much memory?'

| Parameter | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `namespace` | string | yes |  | Kubernetes namespace of the Pod. |
| `meshCluster` | string | no |  | Optional Istio mesh cluster name from kiali_list_mesh_clusters (e.g. west). When omitted, Kiali defaults to its home cluster. |
| `podName` | string | no |  | Kubernetes Pod name. If workloadName is provided, the tool will attempt to resolve a Pod from that workload first. |
| `queryTime` | string | no |  | Optional end timestamp (RFC3339) for the query. Defaults to now. |
| `timeRange` | string | no | `10m` | Time window used to compute CPU rate (Prometheus duration like '5m', '10m', '1h', '1d'). Defaults to '10m'. |
| `workloadName` | string | no |  | Kubernetes Workload name (e.g. Deployment/StatefulSet/etc). Tool will look up the workload and pick one of its Pods. If not found, it will fall back to treating this value as a podName. |

#### `kiali_get_resource_details`

**List or Resource Details.** Read-only.

Fetches a list of resources OR retrieves detailed data for a specific resource. If 'resourceName' is omitted, it returns a list. If 'resourceName' is provided, it returns details for that specific resource.

| Parameter | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `resourceType` | string (`app`, `argoapp`, `namespace`, `service`, `workload`) | yes |  | The type of resource to query. Use 'app' for Kiali applications (grouped by the Kubernetes 'app' label). Use 'argoapp' for ArgoCD Application CRDs (requires ArgoCD installed and the Kiali service account must have read permissions on applications.argoproj.io). |
| `meshCluster` | string | no |  | Optional Istio mesh cluster name from kiali_list_mesh_clusters (e.g. west). When omitted, Kiali defaults to its home cluster. |
| `namespaces` | string | no |  | Comma-separated list of namespaces to query (e.g., 'bookinfo' or 'bookinfo,default'). If not provided, it will query across all accessible namespaces. |
| `resourceName` | string | no |  | Optional. The specific name of the resource. If left empty, the tool returns a list of all resources of the specified type. If provided, the tool returns deep details for this specific resource. |

#### `kiali_get_trace_details`

**Get Trace Details.** Read-only.

Fetches a single distributed trace by trace_id and returns its call hierarchy (service tree with duration, status, and nested calls). Use this after list_traces to drill into a specific trace.

| Parameter | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `traceId` | string | yes |  | Trace ID to fetch and summarize. If provided, namespace/service_name are ignored. |

#### `kiali_list_mesh_clusters`

**List Mesh Clusters.** Read-only.

Returns the list of Istio mesh clusters that Kiali can access. Each entry includes its name and whether it is the home cluster (where Kiali is deployed). Call this tool before using meshCluster on other Kiali tools when the target cluster is unknown.

This tool takes no parameters.

#### `kiali_list_traces`

**List Traces by Service Name.** Read-only.

Lists distributed traces for a service in a namespace. Returns a summary (namespace, service, total_found, avg_duration_ms) and a list of traces with id, duration_ms, spans_count, root_op, slowest_service, has_errors. Use get_trace_details with a trace id to get full hierarchy.

| Parameter | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `namespace` | string | yes |  | Kubernetes namespace of the service. |
| `serviceName` | string | yes |  | Service name to search traces for (required). Returns multiple traces up to limit. |
| `errorOnly` | boolean | no | `false` | If true, only consider traces that contain errors. Default false. |
| `limit` | integer | no | `10` | Maximum number of traces to return. Default 10. |
| `lookbackSeconds` | integer | no | `600` | How far back to search. Default 600 (10m). |
| `meshCluster` | string | no |  | Optional Istio mesh cluster name from kiali_list_mesh_clusters (e.g. west). When omitted, Kiali defaults to its home cluster. |

#### `kiali_manage_istio_config`

**Manage Istio, Gateway API, and Inference API Config: Create, Patch, Delete.** Writes config.

Create, patch, or delete Istio, Gateway API, and Inference API config. Supports Istio resources (networking.istio.io, security.istio.io), Gateway API resources (gateway.networking.k8s.io), and Inference API resources (inference.networking.k8s.io) when installed on the cluster. For list and get (read-only) use manage_istio_config_read.

| Parameter | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `action` | string (`create`, `patch`, `delete`) | yes |  | Action to perform (write) |
| `group` | string (`networking.istio.io`, `security.istio.io`, `gateway.networking.k8s.io`, `inference.networking.k8s.io`) | yes |  | API group of the Istio object. Use 'gateway.networking.k8s.io' for Gateway API resources. Use 'inference.networking.k8s.io' for Inference API resources. |
| `kind` | string (`VirtualService`, `DestinationRule`, `Gateway`, `ServiceEntry`, `Sidecar`, `WorkloadEntry`, `WorkloadGroup`, `EnvoyFilter`, `AuthorizationPolicy`, `PeerAuthentication`, `RequestAuthentication`, `HTTPRoute`, `GRPCRoute`, `ReferenceGrant`, `TCPRoute`, `TLSRoute`, `InferencePool`) | yes |  | Kind of the Istio object (e.g., 'VirtualService', 'DestinationRule'). |
| `namespace` | string | yes |  | Namespace containing the Istio object. |
| `object` | string | yes |  | Name of the Istio object. |
| `version` | string | yes |  | API version. Use 'v1' for all resource types. |
| `data` | string | no |  | JSON or YAML data for the resource. Required for create and patch actions. For create, you can provide partial content (e.g. only spec) and it will be merged onto a valid template with defaults. Arrays (like servers, http, etc.) are REPLACED entirely, so include ALL elements you want. |
| `meshCluster` | string | no |  | Optional Istio mesh cluster name from kiali_list_mesh_clusters (e.g. west). When omitted, Kiali defaults to its home cluster. |

#### `kiali_manage_istio_config_read`

**Manage Istio, Gateway API, and Inference API Config: List or Get.** Read-only.

Read Istio, Gateway API, and Inference API config. 'list' groups by namespace→'group/version/kind'→{valid:[...],invalid:[...]} where valid/invalid arrays contain resource names; omit group/kind to retrieve ALL config types in a single call. Supports Istio (networking.istio.io, security.istio.io), Gateway API (gateway.networking.k8s.io), and Inference API (inference.networking.k8s.io) when installed. 'get' returns full YAML. For writes use manage_istio_config.

| Parameter | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `action` | string (`list`, `get`) | yes |  | Action to perform (read-only) |
| `group` | string (`networking.istio.io`, `security.istio.io`, `gateway.networking.k8s.io`, `inference.networking.k8s.io`) | no |  | API group of the Istio object. Required ONLY for 'get' action. For 'list', OMIT group and kind to retrieve ALL config types in a single call. Use 'gateway.networking.k8s.io' for Gateway API resources. Use 'inference.networking.k8s.io' for Inference API resources. |
| `kind` | string (`VirtualService`, `DestinationRule`, `Gateway`, `ServiceEntry`, `Sidecar`, `WorkloadEntry`, `WorkloadGroup`, `EnvoyFilter`, `AuthorizationPolicy`, `PeerAuthentication`, `RequestAuthentication`, `HTTPRoute`, `GRPCRoute`, `ReferenceGrant`, `TCPRoute`, `TLSRoute`, `InferencePool`) | no |  | Kind of the Istio object. Required ONLY for 'get' action. For 'list', OMIT to return all kinds at once — do NOT call separately for each kind. |
| `meshCluster` | string | no |  | Optional Istio mesh cluster name from kiali_list_mesh_clusters (e.g. west). When omitted, Kiali defaults to its home cluster. |
| `namespace` | string | no |  | Namespace containing the Istio object. For 'list', if not provided, returns objects across all namespaces. For 'get', required. |
| `object` | string | no |  | Name of the Istio object. Required for 'get' action. |
| `serviceName` | string | no |  | Filter Istio configurations (VirtualServices, DestinationRules, and their referenced Gateways) that affect a specific service. Only applicable for 'list' action |
| `version` | string | no |  | API version. Use 'v1' for all resource types. Required for 'get' action. |
<!-- KIALI-TOOLS:END -->

### Troubleshooting

- The server exits because the Kiali toolset is enabled but not configured. Set `[toolset_configs.kiali].url`.
- The URL is rejected. Use an `http://` or `https://` URL with a host.
- HTTPS fails certificate verification. Set `certificate_authority` to the CA file, or, outside production, `insecure = true`.
- Tool calls return empty data or authorization errors. The token presented to Kiali cannot read that namespace or resource.
- `kiali_manage_istio_config` does not appear. The server was started with `read_only = true`.
