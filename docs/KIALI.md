# Kiali

The Kiali tools are delivered
as `Tool` and `Prompt` custom resources by the `kiali-mcp-tools` Helm chart in
[`charts/dynamic-tools/kiali-mcp-tools`](../charts/dynamic-tools/kiali-mcp-tools),
picked up by the `dynamic` toolset at runtime. See
[`docs/specs/dynamic-tools.md`](specs/dynamic-tools.md) for the model.

Tools and prompts for Istio service-mesh observability: mesh status, the traffic graph, metrics, traces, logs and Istio configuration. The chart needs the Kiali API URL (`url`) and, for self-signed certificates, `certificateAuthority` or `insecureSkipTLSVerify`.

```bash
# Install the Tool and Prompt CRDs once per cluster
kubectl apply -f charts/dynamic-tools/crd.yaml

# Install the Kiali tools (cluster-scoped resources)
helm upgrade --install kiali-mcp-tools charts/dynamic-tools/kiali-mcp-tools \
  --set url=https://...   # http-backed charts only

# Enable the dynamic toolset in the server configuration
toolsets = ["core", "config", "dynamic"]
```

The server lists Tool and Prompt resources with its own identity, so its
ServiceAccount needs `get`, `list` and `watch` on `tools`, `prompts` and
`toolsets` (and, to report each resource's status, `update` on `tools/status`,
`prompts/status` and `toolsets/status`) in
the `tools.kubernetes-mcp-server.io` API group; the server Helm chart's
`rbac.dynamicTools: true` renders that ClusterRole. Tool calls run with the
caller's identity, as every built-in tool does.
