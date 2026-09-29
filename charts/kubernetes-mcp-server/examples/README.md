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

## Flux (OCI Helm chart, read-only access)

[`flux/`](flux) deploys the MCP server with [Flux](https://fluxcd.io/) from the OCI chart at `oci://ghcr.io/containers/charts/kubernetes-mcp-server`:

- [`helmrepository.yaml`](flux/helmrepository.yaml): `HelmRepository` with `type: oci`
- [`helmrelease.yaml`](flux/helmrelease.yaml): `HelmRelease` with a read-only configuration

The `HelmRelease` uses a single ServiceAccount bound to the built-in `view` ClusterRole through a `RoleBinding` in each allow-listed namespace (`rbac.extraRoleBindings` with `roleRef.external: true`). There is no `ClusterRoleBinding` to `view`, so namespaces that are not listed stay inaccessible. Kubernetes RBAC has no deny rules, which is why an allow-list is used. `view` does not grant `secrets`, `pods/exec` or `pods/portforward`. `read_only` and `denied_resources` in `config` are a second layer of defense; RBAC is the security boundary.

Edit the namespaces in `extraRoleBindings` (each must exist), then apply:

```bash
kubectl create namespace kubernetes-mcp-server
kubectl apply -f flux/helmrepository.yaml -f flux/helmrelease.yaml
```

Verify the permissions of the release ServiceAccount:

```bash
SA=system:serviceaccount:kubernetes-mcp-server:kubernetes-mcp-server
kubectl auth can-i get pods/log -n app-dev --as=$SA       # yes
kubectl auth can-i get secrets -n app-dev --as=$SA        # no
kubectl auth can-i create pods/exec -n app-dev --as=$SA   # no
kubectl auth can-i get pods -n default --as=$SA           # no
```

The example disables the chart's Ingress. Expose the server only behind an authenticating gateway or a private Service.
