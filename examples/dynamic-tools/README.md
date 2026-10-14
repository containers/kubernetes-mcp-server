# Dynamic tools (proof of concept)

Tools defined as `Tool` custom resources in the cluster and picked up by the
`dynamic` toolset at runtime. Design notes and open questions live in
[`docs/specs/dynamic-tools.md`](../../docs/specs/dynamic-tools.md).

```bash
# 1. Install the CRD
kubectl apply -f crd.yaml

# 2. Install some tools: hand-written examples, or a whole ecosystem toolset
kubectl apply -f tools/
helm install kiali-tools charts/kiali-mcp-tools --set url=https://kiali.example.com/kiali

# 3. Start the server with the toolset enabled (TOML)
#    toolsets = ["core", "config", "dynamic"]

# 4. Tools appear in tools/list; create, change or delete a Tool and the
#    server re-applies its toolsets within the poll interval (15s).
kubectl get tools
```

## Layout

| Path | Shows |
|------|-------|
| `crd.yaml` | The `Tool` and `Prompt` CRDs with their schemas and CEL validation rules |
| `tools/` | Hand-written examples: a CEL projection, a subresource patch, a subresource create |
| `charts/kiali-mcp-tools` | The complete Kiali toolset: 11 `http` Tools and 11 Prompts |
| `charts/netobserv-mcp-tools` | The complete NetObserv toolset (3 tools) |
| `charts/tekton-mcp-tools` | The complete Tekton toolset: 6 tools and the `pipeline-troubleshoot` prompt |
| `charts/kubevirt-mcp-tools` | 6 of the 7 KubeVirt tools, its 3 prompts, a new `vm-create` prompt, and `confirmation-rules.toml` with the Windows EULA rule |
| `charts/loki-mcp-tools` | The complete `observability/logs` toolset (4 tools): LokiStack discovery, optionally through OpenShift Routes (`useRoute`), or a fixed `url` |
| `charts/tempo-mcp-tools` | The complete `observability/traces` toolset (5 tools): TempoStack and TempoMonolithic discovery with tenant checks, optionally through Routes, or a fixed `url` |
| `charts/prometheus-mcp-tools` | The complete `observability/metrics` toolset (9 tools): Prometheus/Thanos at `url`, Alertmanager tools when `alertmanagerUrl` is set, `rangeQueryFullResponse` |
| `converted/` | Generated manifests for the convertible `core` and `kcp` tools |
| `conversion-report.md` | Final report of the migration: one row per former in-tree tool or prompt |

The charts and `converted/` were originally generated from the in-tree Go
toolsets by an exporter that paired each tool's real contract with a declared
action; `conversion-report.md` is its final report. The Go toolsets have
since been removed, so the charts are now the source of truth and are
maintained by hand.

`pkg/toolsets/dynamic` tests validate every file here, and
`pkg/mcp/dynamic_test.go` runs the generated core, Tekton and KubeVirt tools,
the KubeVirt and Tekton prompts, the CRD schemas and the EULA confirmation
rule against envtest. `pkg/mcp/dynamic_observability_test.go` renders the
Loki, Tempo and Prometheus charts with helm and runs them against envtest
(LokiStack, TempoStack and Route resources) and a fake backend.

Confirmation rules are server configuration rather than cluster resources:
the KubeVirt chart's `confirmation-rules.toml` goes into the server's
`--config-dir`, or the Windows EULA check is not enforced.
`make dynamic-tools-install` collects the rules of the charts it installs
into `_output/dynamic-tools-config.d/` for `make run-server`.
