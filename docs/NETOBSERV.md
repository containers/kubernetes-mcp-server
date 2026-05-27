# NetObserv integration

This server exposes tools that call the [NetObserv](https://github.com/netobserv-network-observability/netobserv-operator) console plugin backend API (flows, metrics, alerts, export). The toolset targets OpenShift clusters with the NetObserv operator installed; other Kubernetes distributions work when you set `[toolset_configs.netobserv].url` explicitly.

## Prerequisites

- NetObserv operator and console plugin running (default plugin Service: `netobserv-plugin` in namespace `netobserv`, port `9001`).
- MCP server network access to:
  - the plugin API (in-cluster Service URL or explicit `url`),
  - and optionally OpenShift monitoring (Thanos / Alertmanager) for alert tools.

## Enable the NetObserv toolset

Add the toolset to your TOML configuration:

```toml
toolsets = ["core", "netobserv"]

# Optional on OpenShift in-cluster: omit keys below to use operator-aligned defaults.
[toolset_configs.netobserv]
# url = "https://netobserv-plugin.netobserv.svc.cluster.local:9001"
# namespace = "netobserv"
# service = "netobserv-plugin"
# port = 9001
```

When `netobserv` is listed in `toolsets`, configuration is loaded from `[toolset_configs.netobserv]` if present. On **in-cluster OpenShift** without `url`, the plugin base URL is synthesized as `https://netobserv-plugin.netobserv.svc.cluster.local:9001`. The pod service CA bundle at `/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt` is used automatically when present.

### Tools

| Tool | Description |
|------|-------------|
| `netobserv_list_flows` | Flow records from Loki |
| `netobserv_get_flow_metrics` | Aggregated flow metrics |
| `netobserv_export_flows` | Export flows as CSV |
| `netobserv_list_namespaces` | Namespaces seen in flow data |
| `netobserv_list_names` | Workload/resource names for filters |
| `netobserv_list_alerts` | Prometheus rules (plugin proxy or direct Thanos) |
| `netobserv_list_alert_silences` | Alertmanager silences (plugin proxy or direct) |

## How authentication works

- The server reads the bearer token from the **Kubernetes REST config** for the current tool call (`rest.Config.BearerToken`).
- That token is sent as `Authorization: Bearer …` to the NetObserv plugin (and to direct Prometheus/Alertmanager URLs when fallback is used).
- **HTTP `Authorization` from MCP clients is not required** when the server uses in-cluster ServiceAccount or kubeconfig credentials (typical Helm deployment).
- Do **not** enable `require_oauth` unless you also deploy a separate OAuth front end for MCP clients. Without that, HTTP clients cannot complete OAuth.

| Credential source | NetObserv API calls use |
|-------------------|-------------------------|
| In-cluster ServiceAccount (Helm default) | Pod SA token |
| `cluster_auth_mode = "kubeconfig"` | Always kubeconfig/SA (client Bearer ignored) |
| `cluster_auth_mode = "passthrough"` + client Bearer | Client token (only if you intentionally pass user tokens) |

For a **shared Helm deployment**, use the pod ServiceAccount for Kubernetes API and NetObserv access.

## Helm deployment (OpenShift)

Use the chart example values and expose the release via Route or Gateway API. The pod ServiceAccount provides credentials; leave `extraContainers` empty unless you add your own OAuth proxy.

Example install:

```bash
helm upgrade -i kubernetes-mcp-server oci://ghcr.io/containers/charts/kubernetes-mcp-server \
  -n kubernetes-mcp-server --create-namespace \
  -f charts/kubernetes-mcp-server/examples/values-openshift-netobserv.yaml \
  --set ingress.host=kubernetes-mcp-server.apps.<cluster-domain>
```

See [charts/kubernetes-mcp-server/examples/README.md](../charts/kubernetes-mcp-server/examples/README.md) for the full values file and RBAC notes.

Recommended settings (also in the example file):

- `openshift: true`
- `serviceAccount.automountToken: true`
- `config.toolsets` includes `netobserv`
- `config.cluster_auth_mode: kubeconfig`
- `require_oauth` left disabled (default)
- `extraContainers: []` (no separate OAuth proxy container)

### RBAC

Grant the release ServiceAccount permission to:

1. Use core Kubernetes tools (for example bind `view` or a custom ClusterRole).
2. Call the NetObserv plugin API (the plugin enforces Kubernetes RBAC for the token).
3. Read Prometheus rules and Alertmanager silences when using alert tools (for example `cluster-monitoring-view` on OpenShift).

### TLS

On OpenShift, synthesized plugin URLs use HTTPS. Mount the pod service account so the server can use the cluster service CA. For a custom `url` over HTTPS, set `certificate_authority` or `insecure = true` (development only).

## Configuration reference

| Field | Default (OpenShift in-cluster) | Description |
|-------|--------------------------------|-------------|
| `url` | built from `namespace` / `service` / `port` | Plugin base URL |
| `namespace` | `netobserv` | Plugin Service namespace |
| `service` | `netobserv-plugin` | Plugin Service name |
| `port` | `9001` | Plugin port |
| `prometheus_url` | Thanos querier (in-cluster OCP) | Direct rules API if plugin returns 404 |
| `alertmanager_url` | `alertmanager-main` (in-cluster OCP) | Direct silences API if plugin returns 404 |
| `insecure` | `true` if service CA file missing | Skip TLS verify (avoid in production) |
| `certificate_authority` | auto: service CA on OCP | CA file path for HTTPS |

Set `prometheus_url` and `alertmanager_url` explicitly when using a loopback `url` (local port-forward); in-cluster OpenShift defaults are skipped for `127.0.0.1` / `localhost`.

## Local development

```toml
toolsets = ["core", "netobserv"]

[toolset_configs.netobserv]
url = "https://127.0.0.1:9001"
insecure = true
prometheus_url = "https://127.0.0.1:9091"
alertmanager_url = "https://127.0.0.1:9094"
```

Port-forward the plugin and monitoring Services, then run MCP locally with `KUBECONFIG` after `oc login`. The server uses your kubeconfig token for NetObserv.

## Troubleshooting

| Symptom | What to check |
|---------|----------------|
| `netobserv plugin URL not configured` | Enable toolset; on non-OCP set `url` or `namespace` / `service` / `port` |
| `certificate_authority is required for https` | Set `certificate_authority`, ensure service CA is mounted, or `insecure = true` (dev only) |
| Alert tools: no monitoring fallback | Not on loopback `url` in-cluster; or set `prometheus_url` / `alertmanager_url` |
| 401/403 from plugin | ServiceAccount RBAC must allow NetObserv API access for that token |
| HTTP MCP 401 on all requests | `require_oauth = true` without an OAuth proxy in front of the server — disable `require_oauth` or add OAuth termination |

## Per-user OAuth (optional)

If you deploy a separate OAuth proxy in front of this server, clients authenticate there and may send a user Bearer token. NetObserv then uses that token when `cluster_auth_mode` is `passthrough`. That differs from the Helm example above, which uses the pod ServiceAccount via `cluster_auth_mode = kubeconfig`.

## OAuth and MCP Shield

[MCP Shield](https://github.com/jpinsonn/mcp-shield) can terminate OAuth for MCP clients and forward a user Bearer token to this server. NetObserv then uses that user token when `cluster_auth_mode` is `passthrough`. That is a separate deployment model from the Helm example above.

## Evaluations

Agent evaluation tasks for this toolset live under [`evals/tasks/netobserv/`](../evals/tasks/netobserv/). On Kind/CI a mock console plugin is used (`make setup-netobserv`); on OpenShift you can run the same tasks against a real FlowCollector.

```bash
make setup-netobserv
make run-server TOOLSETS=core,netobserv MCP_CONFIG_DIR=dev/config/mcp-configs
make run-evals EVAL_LABEL_SELECTOR=suite=netobserv
```

Maintainers can trigger CI with `/run-mcpchecker netobserv` on a pull request. See [evals/tasks/netobserv/README.md](../evals/tasks/netobserv/README.md).
