# Tekton

The Tekton tools are no longer compiled into the server. They are delivered
as `Tool` and `Prompt` custom resources by the `tekton-mcp-tools` Helm chart in
[`examples/dynamic-tools/charts/tekton-mcp-tools`](../examples/dynamic-tools/charts/tekton-mcp-tools),
picked up by the `dynamic` toolset at runtime. See
[`docs/specs/dynamic-tools.md`](specs/dynamic-tools.md) for the model.

6 tools (start, restart, cancel, logs for Pipelines and Tasks) and the `pipeline-troubleshoot` prompt.

```bash
# Install the Tool and Prompt CRDs once per cluster
kubectl apply -f examples/dynamic-tools/crd.yaml

# Install the Tekton tools (cluster-scoped resources)
helm upgrade --install tekton-mcp-tools examples/dynamic-tools/charts/tekton-mcp-tools \
  --set url=https://...   # http-backed charts only

# Enable the dynamic toolset in the server configuration
toolsets = ["core", "config", "dynamic"]
```

The server lists Tool and Prompt resources with its own identity, so its
ServiceAccount needs `get`, `list` and `watch` on `tools` and `prompts` in
the `tools.kubernetes-mcp-server.io` API group. Tool calls run with the
caller's identity, as every built-in tool does.
