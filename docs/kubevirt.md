# KubeVirt

The KubeVirt tools are delivered
as `Tool` and `Prompt` custom resources by the `kubevirt-mcp-tools` Helm chart in
[`charts/dynamic-tools/kubevirt-mcp-tools`](../charts/dynamic-tools/kubevirt-mcp-tools),
picked up by the `dynamic` toolset at runtime. See
[`docs/specs/dynamic-tools.md`](specs/dynamic-tools.md) for the model.

Tools for the VirtualMachine lifecycle, cloning, template instantiation, guest info, troubleshooting and console screenshots, and the `vm-troubleshoot`, `vm-hco-status`, `vm-windows-golden-image` and `vm-create` prompts. The chart also ships `confirmation-rules.toml` with the Microsoft EULA rule for Windows golden-image PipelineRuns; add it to the server's `--config-dir` (the server does not enforce the EULA check without it; `make dynamic-tools-install DYNAMIC_TOOLS_CHARTS=kubevirt-mcp-tools` collects it for `make run-server`).

```bash
# Install the Tool and Prompt CRDs once per cluster
kubectl apply -f charts/dynamic-tools/crd.yaml

# Install the KubeVirt tools (cluster-scoped resources)
helm upgrade --install kubevirt-mcp-tools charts/dynamic-tools/kubevirt-mcp-tools \
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
