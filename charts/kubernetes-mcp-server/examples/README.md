# Helm chart examples

## OpenShift with NetObserv

[`values-openshift-netobserv.yaml`](values-openshift-netobserv.yaml) deploys the MCP server with:

- `toolsets`: `core` and `netobserv`
- `cluster_auth_mode: kubeconfig` (pod ServiceAccount for Kubernetes API and NetObserv plugin)
- Pod ServiceAccount auth (`require_oauth` stays off)
- RBAC: `view` for core tools and NetObserv plugin API access

Full toolset and authentication details: [NetObserv integration](../../../docs/NETOBSERV.md).

From the **chart directory** (`charts/kubernetes-mcp-server`):

```bash
cd charts/kubernetes-mcp-server

helm upgrade -i kubernetes-mcp-server . \
  -n kubernetes-mcp-server --create-namespace \
  -f examples/values-openshift-netobserv.yaml \
  --set ingress.host=kubernetes-mcp-server.apps.<cluster-domain>
```
