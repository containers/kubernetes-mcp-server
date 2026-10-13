# KubeVirt

The KubeVirt tools are no longer compiled into the server. They are delivered
as `Tool` and `Prompt` custom resources by the `kubevirt-mcp-tools` Helm chart in
[`examples/dynamic-tools/charts/kubevirt-mcp-tools`](../examples/dynamic-tools/charts/kubevirt-mcp-tools),
picked up by the `dynamic` toolset at runtime. See
[`docs/specs/dynamic-tools.md`](specs/dynamic-tools.md) for the model.

6 tools (lifecycle, clone, template instantiation, guest info, troubleshoot, console screenshot) and 4 prompts (`vm-troubleshoot`, `hco-status`, `windows-golden-image`, `vm-create`). The chart also ships `confirmation-rules.toml` with the Microsoft EULA rule for Windows golden-image PipelineRuns; add it to the server's `--config-dir`.

```bash
# Install the Tool and Prompt CRDs once per cluster
kubectl apply -f examples/dynamic-tools/crd.yaml

# Install the KubeVirt tools (cluster-scoped resources)
helm upgrade --install kubevirt-mcp-tools examples/dynamic-tools/charts/kubevirt-mcp-tools \
  --set url=https://...   # http-backed charts only

# Enable the dynamic toolset in the server configuration
toolsets = ["core", "config", "dynamic"]
```

The server lists Tool and Prompt resources with its own identity, so its
ServiceAccount needs `get`, `list` and `watch` on `tools` and `prompts` in
the `tools.kubernetes-mcp-server.io` API group. Tool calls run with the
caller's identity, as every built-in tool does.
