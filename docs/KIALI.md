# Kiali

The Kiali tools are no longer compiled into the server. They are delivered
as `Tool` and `Prompt` custom resources by the `kiali-mcp-tools` Helm chart in
[`examples/dynamic-tools/charts/kiali-mcp-tools`](../examples/dynamic-tools/charts/kiali-mcp-tools),
picked up by the `dynamic` toolset at runtime. See
[`docs/specs/dynamic-tools.md`](specs/dynamic-tools.md) for the model.

11 tools and 11 prompts for Istio service-mesh observability (mesh status, traffic graph, metrics, traces, logs, Istio configuration). The chart needs the Kiali API URL (`url`) and, for self-signed certificates, `certificateAuthority` or `insecureSkipTLSVerify`.

```bash
# Install the Tool and Prompt CRDs once per cluster
kubectl apply -f examples/dynamic-tools/crd.yaml

# Install the Kiali tools (cluster-scoped resources)
helm upgrade --install kiali-mcp-tools examples/dynamic-tools/charts/kiali-mcp-tools \
  --set url=https://...   # http-backed charts only

# Enable the dynamic toolset in the server configuration
toolsets = ["core", "config", "dynamic"]
```

The server lists Tool and Prompt resources with its own identity, so its
ServiceAccount needs `get`, `list` and `watch` on `tools` and `prompts` in
the `tools.kubernetes-mcp-server.io` API group. Tool calls run with the
caller's identity, as every built-in tool does.
