# Dynamic tools

Tools defined as `Tool` custom resources in the cluster and picked up by the
`dynamic` toolset at runtime. Design notes and open questions live in
[`docs/specs/dynamic-tools.md`](../../docs/specs/dynamic-tools.md).

```bash
# 1. Install the CRD
kubectl apply -f crd.yaml

# 2. Check what you are about to install, then install it: hand-written
#    example Tools, or a whole ecosystem toolset. `check` runs what the
#    server runs at registration (validation, CEL compilation, the
#    house-style lints).
kubernetes-mcp-server check tools/
kubectl apply -f tools/
helm template kiali-tools kiali-mcp-tools --set url=https://kiali.example.com/kiali | kubernetes-mcp-server check -
helm install kiali-tools kiali-mcp-tools --set url=https://kiali.example.com/kiali

# 2b. Let the server read them: it lists Tool, Prompt and ToolSet resources
#    with its own ServiceAccount. The server chart's rbac.dynamicTools value
#    renders the ClusterRole (list on the three resources, update on their status).
helm upgrade kubernetes-mcp-server charts/kubernetes-mcp-server --reuse-values --set rbac.dynamicTools=true

# 2c. Optional: bound which Tool resources a server reads at all. Label the
#     installs a server should serve with the chart's extraLabels value, and
#     give the server the matching dynamic_tools_selector; resources outside
#     the selector are not read, compiled or given a status.
helm install kiali-tools kiali-mcp-tools --set url=https://kiali.example.com/kiali --set extraLabels.served-by=prod-mcp
#     server TOML: dynamic_tools_selector = "served-by=prod-mcp"

# 3. Start the server with the toolset enabled (TOML). Each chart is a
#    family (a ToolSet resource): enable them all with "dynamic", or pick
#    toolsets = ["core", "config", "dynamic/kiali", "dynamic/prometheus"]

# 4. Tools appear in tools/list; create, change or delete a Tool and the
#    server re-applies its toolsets within the poll interval (15s). The
#    READY and REASON columns say what the server did with each resource.
kubectl get toolsets,tools,prompts
```

## Layout

| Path | Shows |
|------|-------|
| `crd.yaml` | The `Tool`, `Prompt` and `ToolSet` CRDs with their schemas and CEL validation rules |
| `tools/` | Hand-written Tools that stand alone, without a family: a CEL projection, a subresource patch, a subresource create |
| `kiali-mcp-tools` | The Kiali tools and prompts: mesh status, traffic graph, metrics, traces, logs, Istio configuration, as `http` Tools against the Kiali API |
| `netobserv-mcp-tools` | The NetObserv tools: flows, flow metrics and CSV export against the console plugin API |
| `tekton-mcp-tools` | The Tekton tools (`tekton_start`, `tekton_restart`, `tekton_cancel`, `tekton_logs`, each with a `kind` variant) and the `tekton-pipeline-troubleshoot` prompt |
| `kubevirt-mcp-tools` | The KubeVirt tools and prompts (`vm-troubleshoot`, `vm-hco-status`, `vm-windows-golden-image`, `vm-create`), and `confirmation-rules.toml` with the Windows EULA rule |
| `loki-mcp-tools` | The Loki tools: LokiStack discovery, optionally through OpenShift Routes (`useRoute`), or a fixed `url` |
| `tempo-mcp-tools` | The Tempo tools: TempoStack and TempoMonolithic discovery with tenant checks, optionally through Routes, or a fixed `url` |
| `prometheus-mcp-tools` | The Prometheus tools: Prometheus/Thanos at `url`, Alertmanager tools when `alertmanagerUrl` is set, `rangeQueryFullResponse` |

Every chart ships a `templates/toolset.yaml`: the `ToolSet` resource that
names the family (`dynamic/<name>` in the toolsets configuration), the
prefix of its tools and the instructions the server contributes to the MCP
server instructions; each Tool and Prompt names it with `spec.toolset`. See
[`docs/specs/tools-for-models.md`](../../docs/specs/tools-for-models.md).

The charts are maintained by hand: they are the source of truth for the
tools they ship.

`pkg/toolsets/dynamic` tests validate every file here, and
`pkg/mcp/dynamic_test.go` runs the Tekton and KubeVirt tools, the KubeVirt
and Tekton prompts, the CRD schemas and the EULA confirmation rule against
envtest. `pkg/mcp/dynamic_observability_test.go` renders the
Loki, Tempo and Prometheus charts with helm and runs them against envtest
(LokiStack, TempoStack and Route resources) and a fake backend.

Confirmation rules are server configuration rather than cluster resources:
the KubeVirt chart's `confirmation-rules.toml` goes into the server's
`--config-dir`, or the Windows EULA check is not enforced.
`make dynamic-tools-install` collects the rules of the charts it installs
into `_output/dynamic-tools-config.d/` for `make run-server`.
