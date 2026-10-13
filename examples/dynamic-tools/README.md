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
| `crd.yaml` | The `Tool` CRD with its schema and CEL validation rules |
| `tools/` | Hand-written examples: a CEL projection, a subresource patch, a subresource create |
| `charts/kiali-mcp-tools` | The complete Kiali toolset (11 tools) as `http` Tools |
| `charts/netobserv-mcp-tools` | The complete NetObserv toolset (3 tools) |
| `charts/tekton-mcp-tools` | The single-request half of the Tekton toolset |
| `charts/kubevirt-mcp-tools` | The single-request half of the KubeVirt toolset |
| `converted/` | Generated manifests for the convertible `core` and `kcp` tools |
| `conversion-report.md` | One row per in-tree tool: converted or why not |
| `hack/export` | The generator that produces `charts/*/templates`, `converted/` and the report |

The charts and `converted/` are **generated**: the exporter takes each
in-tree tool's real contract from the registered Go definition and pairs it
with a declared action, then validates the result with the toolset's own
compiler. Regenerate after changing a toolset or the mapping:

```bash
go run ./examples/dynamic-tools/hack/export
```

`pkg/toolsets/dynamic` tests validate every file here, and
`pkg/mcp/dynamic_test.go` runs the generated core manifests against envtest.
