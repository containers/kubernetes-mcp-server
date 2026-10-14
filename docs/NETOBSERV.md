# NetObserv

The NetObserv tools are no longer compiled into the server. They are delivered
as `Tool` and `Prompt` custom resources by the `netobserv-mcp-tools` Helm chart in
[`examples/dynamic-tools/charts/netobserv-mcp-tools`](../examples/dynamic-tools/charts/netobserv-mcp-tools),
picked up by the `dynamic` toolset at runtime. See
[`docs/specs/dynamic-tools.md`](specs/dynamic-tools.md) for the model.

3 tools for network flows, flow metrics and CSV export backed by the NetObserv console plugin API. The chart needs the plugin URL (`url`) and TLS settings.

```bash
# Install the Tool and Prompt CRDs once per cluster
kubectl apply -f examples/dynamic-tools/crd.yaml

# Install the NetObserv tools (cluster-scoped resources)
helm upgrade --install netobserv-mcp-tools examples/dynamic-tools/charts/netobserv-mcp-tools \
  --set url=https://...   # http-backed charts only

# Enable the dynamic toolset in the server configuration
toolsets = ["core", "config", "dynamic"]
```

The server lists Tool and Prompt resources with its own identity, so its
ServiceAccount needs `get`, `list` and `watch` on `tools` and `prompts` in
the `tools.kubernetes-mcp-server.io` API group. Tool calls run with the
caller's identity, as every built-in tool does.
