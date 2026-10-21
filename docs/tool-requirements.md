# Tool requirements

Some tools require additional cluster APIs, resource instances, or permissions
before they become available. Enabling a toolset does not necessarily make all
of its tools available.

This page is not an exhaustive list. Add tool-specific availability requirements
here as they are documented.

## Availability requirements

| Toolset | Tool | Requirement | When enforced |
|---------|------|-------------|---------------|
| `observability/metrics` | `show_timeseries` | At least one `perses.dev/v1alpha2` Perses instance in a configured cluster | Always, regardless of compatibility filtering |

## observability/metrics

### show_timeseries

The tool is registered only when at least one `perses.dev/v1alpha2` Perses
resource instance is found in a configured cluster. Installing the CRD alone
is not sufficient.

This check applies regardless of
`experimental_enable_target_compatibility_tool_filters`.
See the [configuration reference](configuration.md) for that setting.

#### Permissions

Detection lists Perses resources across namespaces. The credentials used for
detection need the `list` verb on the `perses` resource in the `perses.dev`
API group, granted across all namespaces.

If no configured cluster returns an instance, including when all listing
attempts fail, the tool stays hidden. No Perses-specific permissions are
granted by default.

For Helm deployments using the chart's ServiceAccount, configure:

```yaml
rbac:
  extraClusterRoles:
    - name: perses-reader
      rules:
        - apiGroups: ["perses.dev"]
          resources: ["perses"]
          verbs: ["list"]
  extraClusterRoleBindings:
    - name: perses-reader
      roleRef:
        name: perses-reader
```
