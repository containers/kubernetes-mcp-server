# Tool RBAC Metadata

Kubernetes MCP Server tools can advertise the Kubernetes RBAC permissions they may require in the `_meta` field returned by `tools/list`. Clients can use this metadata with planned tool arguments to prepare permissions before calling a tool.

The metadata is a planning aid. It does not grant access, replace Kubernetes authorization, or guarantee that a call will succeed. The Kubernetes API server remains authoritative.

Client support for this metadata is optional. Clients that do not dynamically prepare or narrow Kubernetes permissions can ignore it and call tools using their existing credentials. The metadata is intended for clients that want to derive scoped RBAC for a planned call, such as an agent sandbox that creates a short-lived ServiceAccount, Role, or binding before execution.

## Metadata Key

RBAC metadata is published under the `io.kubernetes-mcp-server/rbac` key:

```json
{
  "_meta": {
    "io.kubernetes-mcp-server/rbac": {
      "version": "v1alpha1",
      "none": {}
    }
  }
}
```

The metadata key being absent means that the tool has not declared its RBAC requirements. It must not be interpreted as requiring no RBAC.

## Declaration States

Every declaration has a `version` and exactly one of the following properties:

| Property | Meaning |
|---|---|
| `none` | The tool does not require Kubernetes API authorization. |
| `bounded` | The tool's possible RBAC requirements are described by `requirements`. |
| `unbounded` | The requirements cannot be determined from the tool declaration and call arguments. |

### No RBAC

Tools that do not access the Kubernetes API declare `none` explicitly:

```json
{
  "version": "v1alpha1",
  "none": {}
}
```

An optional `reason` can provide additional context:

```json
{
  "version": "v1alpha1",
  "none": {
    "reason": "Reads the local kubeconfig without accessing the Kubernetes API"
  }
}
```

### Bounded RBAC

A bounded declaration contains one or more requirements:

```json
{
  "version": "v1alpha1",
  "bounded": {
    "requirements": [
      {
        "verbs": ["get"],
        "target": {
          "resource": {
            "resource": "pods"
          }
        }
      }
    ]
  }
}
```

`bounded` describes a finite, conservative upper bound. After resolving arguments and manifests, every Kubernetes API authorization check that the tool may perform must be covered by the declaration. The declaration may include permissions that a particular invocation does not use, for example when behavior depends on an optional argument, cluster state, API discovery, or transport negotiation.

Clients combine all resolved requirements when preparing RBAC. Declaring too few permissions is a metadata error and can cause a tool call to fail with an authorization error. Declaring a slightly broader finite set is permitted and avoids requiring a conditional-expression language in the metadata contract.

Bounded declarations should identify concrete resources after argument or manifest resolution. Broad wildcard permissions such as all API groups, all resources, or all subresources should not be used as a substitute for `unbounded`.

### Unbounded RBAC

Tools whose permissions cannot be determined from their arguments declare `unbounded`:

```json
{
  "version": "v1alpha1",
  "unbounded": {
    "reason": "Permissions depend on resources contained in the referenced Helm chart"
  }
}
```

Clients should apply their own policy to unbounded tools, such as requesting separate approval or rejecting the planned call.

## Requirements

Each requirement contains:

| Field | Required | Description |
|---|---|---|
| `verbs` | Yes | Kubernetes authorization verbs such as `get`, `list`, `create`, `update`, `patch`, or `delete`. |
| `target` | Yes | The resource identity or the source from which it is derived. |
| `namespace` | No | How to determine the namespace for a namespaced target. |
| `resourceName` | No | A literal resource name or an argument containing the resource name. |

Static targets use Kubernetes RBAC API groups and plural resource names directly. Generic tools whose target is selected by call arguments use `apiVersionArgument` and `kindArgument` so clients can resolve the target through Kubernetes discovery.

An argument field names a top-level property in the capability's JSON input object. Argument references are not filesystem paths, JSON Pointers, or dotted expressions.

## Target Types

Three target representations are supported.

### RBAC Resource

Use `resource` when the Kubernetes RBAC API group and plural resource name are known:

```json
{
  "verbs": ["get"],
  "target": {
    "resource": {
      "resource": "pods"
    }
  }
}
```

Subresources are declared separately from the parent resource:

```json
{
  "verbs": ["get"],
  "target": {
    "resource": {
      "resource": "pods",
      "subresource": "log"
    }
  }
}
```

A resource target must declare `resource`. An omitted API group means the core Kubernetes API group.

### Group, Version, and Kind

Use `gvk` when call arguments identify the target by Kubernetes API version and kind. The client resolves the GVK to an RBAC API group and plural resource through Kubernetes discovery. Both argument references are required:

```json
{
  "verbs": ["delete"],
  "target": {
    "gvk": {
      "apiVersionArgument": "apiVersion",
      "kindArgument": "kind"
    }
  }
}
```

For a fixed subresource of a GVK-derived resource, use `subresource`:

```json
{
  "verbs": ["get", "update"],
  "target": {
    "gvk": {
      "apiVersionArgument": "apiVersion",
      "kindArgument": "kind",
      "subresource": "scale"
    }
  }
}
```

For example, a `resources_scale` call with `apiVersion: "apps/v1"` and `kind: "Deployment"` resolves through discovery to the RBAC resource `deployments/scale` in the `apps` API group. The tool always gets the scale and may update it, so its conservative upper bound includes both verbs even when a particular call omits the optional `scale` argument.

```yaml
apiGroups: ["apps"]
resources: ["deployments/scale"]
verbs: ["get", "update"]
```

### Manifest

Use `manifest` when resource identities are contained in a YAML or JSON manifest argument:

```json
{
  "verbs": ["patch"],
  "target": {
    "manifest": {
      "argument": "resource"
    }
  }
}
```

The client is responsible for parsing the manifest and determining its resource identities, names, and namespaces. This includes handling formats such as multiple YAML documents or Kubernetes List objects when supported by the client.

## Namespaces

Namespace selection is independent of how the resource identity is determined. A requirement with a fixed resource can still obtain its namespace from an argument.

### Namespace from an Argument

```json
{
  "namespace": {
    "argument": "namespace"
  }
}
```

When a namespace argument is omitted, Kubernetes MCP Server normally uses a default namespace from the selected target's kubeconfig. That namespace is target-dependent and is not included in RBAC metadata. Publishing every target's kubeconfig-derived default would complicate the contract and could expose configuration that most clients do not need.

An RBAC-managing client cannot resolve an omitted namespace from the declaration alone. It may instead:

- Add an explicit namespace to the planned capability call.
- Grant access across all namespaces with an appropriate ClusterRoleBinding.
- Create a ClusterRole and bind it with RoleBindings in a client-selected set of namespaces.
- Fail closed and require the caller to choose a namespace.

Clients must not assume that an omitted namespace argument means the target resource is cluster-scoped. Resource scope is determined independently through Kubernetes discovery.

### Fixed Namespace

A literal value identifies a fixed namespace:

```json
{
  "namespace": {
    "name": "sandbox"
  }
}
```

### All Namespaces

A requirement that always accesses all namespaces uses:

```json
{
  "namespace": {
    "all": true
  }
}
```

For `resource` and `gvk` targets, clients determine whether the resolved resource is namespaced or cluster-scoped through Kubernetes discovery. Cluster-scoped targets omit `namespace`.

For `manifest` targets, namespace selection is part of manifest interpretation and the requirement normally omits `namespace`.

## Resource Names

The optional `resourceName` field identifies the object targeted by a call:

```json
{
  "resourceName": {
    "argument": "name"
  }
}
```

It may also contain a literal name:

```json
{
  "resourceName": {
    "name": "cluster-settings"
  }
}
```

Clients may use this value to produce Kubernetes `resourceNames` restrictions where Kubernetes supports them. In particular, `resourceNames` cannot restrict `create`, `list`, `watch`, or `deletecollection`. Clients are responsible for splitting or widening PolicyRules as necessary.

## Complete Examples

### Fixed Resource with Argument-Derived Placement

```json
{
  "version": "v1alpha1",
  "bounded": {
    "requirements": [
      {
        "verbs": ["get"],
        "target": {
          "resource": {
            "resource": "pods"
          }
        },
        "namespace": {
          "argument": "namespace"
        },
        "resourceName": {
          "argument": "name"
        }
      }
    ]
  }
}
```

### Generic Resource Tool

```json
{
  "version": "v1alpha1",
  "bounded": {
    "requirements": [
      {
        "verbs": ["delete"],
        "target": {
          "gvk": {
            "apiVersionArgument": "apiVersion",
            "kindArgument": "kind"
          }
        },
        "namespace": {
          "argument": "namespace"
        },
        "resourceName": {
          "argument": "name"
        }
      }
    ]
  }
}
```

### Manifest Tool

```json
{
  "version": "v1alpha1",
  "bounded": {
    "requirements": [
      {
        "verbs": ["patch"],
        "target": {
          "manifest": {
            "argument": "resource"
          }
        }
      }
    ]
  }
}
```

## Consumer Expectations

Consumers should:

- Ignore this metadata if they do not manage Kubernetes RBAC for tool calls.
- Treat an absent declaration as unknown, not as `none`.
- Resolve argument references against the planned tool call's JSON input.
- Use Kubernetes discovery to resolve GVKs and resource scope.
- Apply Kubernetes semantics when translating resource names into PolicyRules.
- Choose an explicit grant strategy or fail closed when a namespace argument is omitted.
- Apply independent policy limits before granting any resolved permissions.
