# Tekton

The Tekton tools are delivered
as `Tool` and `Prompt` custom resources by the `tekton-mcp-tools` Helm chart in
[`charts/dynamic-tools/tekton-mcp-tools`](../charts/dynamic-tools/tekton-mcp-tools),
picked up by the `dynamic` toolset at runtime. See
[`docs/specs/dynamic-tools.md`](specs/dynamic-tools.md) for the model.

Tools to start, restart and cancel Pipelines and Tasks and to read their logs, each taking a `kind` argument, and the `tekton-pipeline-troubleshoot` prompt.

```bash
# Install the Tool and Prompt CRDs once per cluster
kubectl apply -f charts/dynamic-tools/crd.yaml

# Install the Tekton tools (cluster-scoped resources)
helm upgrade --install tekton-mcp-tools charts/dynamic-tools/tekton-mcp-tools \
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
