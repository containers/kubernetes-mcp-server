# Dynamic tools (proof of concept)

Tools defined as `Tool` custom resources in the cluster and picked up by the
`dynamic` toolset at runtime. Design notes and open questions live in
[`docs/specs/dynamic-tools.md`](../../docs/specs/dynamic-tools.md).

```bash
# 1. Install the CRD
kubectl apply -f crd.yaml

# 2. Install some tools
kubectl apply -f tools/

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
| `tools/deployments-summary.yaml` | A list with a CEL projection so the model sees five fields per object |
| `tools/deployment-scale.yaml` | A subresource patch whose body is built from the arguments with CEL |
| `tools/pod-evict.yaml` | A subresource create (`pods/eviction`) |
| `charts/kiali-mcp-tools/` | The Kiali toolset as an independently versioned Helm chart of `http` tools |

The Kiali chart is the point of the exercise: the in-tree Kiali toolset is a
set of HTTP POSTs to `/api/mcp/<tool>` carrying the caller's bearer token, so
every one of its tools can be expressed as a `Tool` resource and shipped by
the Kiali project on its own release cadence.

```bash
helm template kiali ./charts/kiali-mcp-tools --set kiali.url=https://kiali.example.com/kiali
```
