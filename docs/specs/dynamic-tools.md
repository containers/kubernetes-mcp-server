# Dynamic Tools (Tool custom resources)

Status: **Proof of concept** (`dynamic` toolset, opt-in, not enabled by default)

Tools defined declaratively as `Tool` custom resources in the cluster, picked
up by the server at runtime, instead of being compiled into the server. The
goal is a small, rock-solid base server whose ecosystem integrations (Kiali,
NetObserv, Tekton, KubeVirt, ...) are delivered and versioned independently,
the way CRDs and operators extend Kubernetes itself.

Examples, the CRD and a Helm chart that ports part of the Kiali toolset live
in [`examples/dynamic-tools`](../../examples/dynamic-tools).

## Motivation

Every built-in tool is, at the bottom, one of two things:

1. a single request against the Kubernetes API with the caller's identity,
   shaped for the model (the `core`, `kubevirt`, `tekton` and `kcp` toolsets);
2. an HTTP request to a project's own API carrying the caller's bearer token
   (the `kiali` and `netobserv` toolsets, which POST the tool arguments to
   `/api/mcp/<tool>` on the project's server).

What the server adds around them is the hard part and is shared: per-request
derived clients, token exchange, multi-cluster targeting, the access-control
round tripper, validation, confirmation rules, RBAC metadata and hot reload.
Today the only way to get a tool behind that layer is to merge it into this
repository, which couples the server's release cadence to every ecosystem
project and makes maintainers responsible for tools they did not ask for.

A `Tool` resource captures (1) and (2) as data. The server keeps the policy
layer; the ecosystem project keeps its tool definitions.

## Trust model

**A Tool resource is prompt text injected into every agent that connects.**
Whoever can create one can steer every model that uses the server, and can
point `http` tools at arbitrary URLs with the caller's bearer token. This is
the same trust level as installing an admission webhook or a CRD, so:

- `Tool` is **cluster-scoped**. Creating one requires cluster-level RBAC on
  `tools.tools.kubernetes-mcp-server.io`; namespace admins cannot add tools.
- The server lists Tool resources with **its own** identity (the default
  target's kubeconfig / in-cluster service account), never with the caller's
  token. Operators decide what that identity may read.
- Tool **calls** run with the **caller's** identity. A Tool cannot escalate:
  a `kubernetes` action is subject to the caller's RBAC, `denied_resources`,
  the validators and the confirmation rules exactly like a built-in tool,
  because it goes through the same `rest.Config` and round tripper. An `http`
  action to an in-cluster Service goes through the API server's service
  proxy, so it needs `services/proxy` RBAC on that Service.
- `read_only` and `disable_destructive` apply through the declared
  annotations, which the server cannot verify. A Tool that lies about being
  read-only is an operator trust problem, not a server one, and it is one
  reason the resource is cluster-scoped.

Open hardening items (not implemented): restricting the listed Tools by label
selector or allowed names in server configuration, an allowlist of `http`
hosts, and refusing a Tool whose name shadows a built-in tool (today a
duplicate name fails the whole reload, which is safe but unfriendly).

## Resource

```yaml
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: deployments-summary          # tool name defaults to this with '-' -> '_'
spec:
  name: deployments_summary          # optional explicit MCP tool name
  title: "Deployments: Summary"      # optional annotation
  description: ...                   # written for the model, required
  readOnly: true                     # MCP annotations; drive read_only /
  destructive: false                 #   disable_destructive filtering
  idempotent: true
  inputSchema: {...}                 # JSON Schema object of the arguments
  kubernetes: {...}                  # exactly one of kubernetes / http
  http: {...}
  output: <CEL>                      # optional projection of the response
```

### References

Every place a request takes a value accepts a reference:

```yaml
name: {argument: name}            # a tool argument
name: {expression: "args.a + '-' + args.b"}   # CEL over args
name: fixed-value                 # a fixed value; shorthand for {value: ...}
name: {value: "prefix-{name}"}    # fixed values may embed {argument} placeholders
```

Resolution order is argument, then expression, then value; the first
non-empty result wins. Namespace references add `all: true` as the fallback
after those (otherwise the caller's default namespace is used).

### `kubernetes` action

One request against the Kubernetes API of the target cluster.

| Field | Meaning |
|-------|---------|
| `verb` | `get`, `list`, `create`, `update`, `patch`, `delete`. Maps to the HTTP method and is what the RBAC metadata advertises. |
| `apiVersion` + `kind` | Resolved to a resource through the target's RESTMapper, so CRDs work and the model never builds a path. |
| `apiVersion` + `resource` (+ `namespaced`) | Alternative to `kind`: the plural, bypassing discovery. Needed for aggregated APIs that only serve subresources, such as `subresources.kubevirt.io`. |
| `subresource` | A reference. May have several segments (`proxy/stats/summary`) or be argument-derived (`{argument: action}` for KubeVirt's `virtualmachines/{start,stop,restart}`). |
| `namespace` | Reference plus `all: true`. Ignored for cluster-scoped resources. |
| `name` | Reference. Required for every verb except `list` and `create`. |
| `query` | Map of query parameter to reference (`labelSelector`, `fieldSelector`, `tailLines`, ...). Empty values are omitted. |
| `queryExpression` | CEL over `args` producing a map of query parameters, merged under `query`. |
| `body` | `{argument: manifest}` (YAML/JSON string or object argument), `{value: {...}}` (fixed JSON) or `{expression: <CEL over args>}`. Defaults to `{}` for `create`/`update`/`patch`, which is what subresource endpoints such as KubeVirt's `pause` expect. `patch` sends a merge patch. |

The request is issued through a REST client built from the caller's
`rest.Config`, i.e. the very same transport chain as every built-in tool.
A JSON response is decoded; anything else (e.g. `pods/log`) is returned as
text, verbatim.

### `http` action

One HTTP request carrying the caller's identity.

| Field | Meaning |
|-------|---------|
| `url` | Absolute base URL. The caller's bearer token is sent as `Authorization: Bearer`. This is what the Kiali and NetObserv toolsets do. |
| `service` | `{namespace, name, port, scheme}`: reached through the API server's service proxy with the caller's `rest.Config`. No TLS or token plumbing, and `services/proxy` RBAC applies. Mutually exclusive with `url`. |
| `method`, `path`, `query`, `queryExpression`, `headers`, `body` | `path` supports `{argument}` placeholders (URL-escaped, required). `query`/`queryExpression`/`body` as above; `{expression: args}` forwards all arguments. |
| `certificateAuthority`, `insecureSkipTLSVerify` | TLS verification for `url`. |

### `output`

A [CEL](https://cel.dev) expression evaluated with `response` (the decoded
response) and `args` (the tool arguments). Its result replaces the response
in the tool result. CEL is already in the dependency graph (CRD validation
rules use it) and is the language operators already write in Kubernetes.
Expressions are compiled when the toolset is applied, so a broken Tool is
logged and skipped rather than failing on first call.

Every expression (output, body, query, references) sees the same environment:

| Name | Meaning |
|------|---------|
| `args` | the tool arguments, `map<string, dyn>` |
| `response` | the decoded response (`null` while building the request) |
| `now` | the current time; `int(now)` is Unix seconds |
| `merge(a, b)` | a map with the entries of `a` overridden by those of `b` |
| cel-go extensions | strings, lists, math and two-variable comprehensions (`transformMapEntry` renames or drops keys) |

Strings are returned verbatim (logs, CSV, messages); anything else is
rendered as YAML text and, when it is an object, also returned as
`structuredContent`.

## RBAC metadata

The declaration is enough to derive the server's `_meta` RBAC advertisement
(see [`tool-rbac-metadata.md`](../tool-rbac-metadata.md)) without any code:

- `kubernetes`: **bounded** to `verb` on the resource (declared, or resolved
  through discovery), with namespace and resource name mapped from the
  `argument`/`value` references and a fixed subresource reduced to its first
  segment (`proxy/stats/summary` advertises `nodes/proxy`). Falls back to
  **unbounded** when the kind cannot be resolved at registration time or when
  the subresource, namespace or name comes from a CEL expression.
- `http` with `service`: **bounded** to `get`/`create` on `services/proxy`
  for that Service.
- `http` with `url`: **none**, with a reason naming the URL.

## Reload

Tool resources are read from the **default target** when toolsets are applied
(startup, SIGHUP, kubeconfig or cluster-state change). The toolset also
implements the new optional `api.ToolsetWatcher` interface: the server calls
`WatchToolset` for each enabled toolset that implements it and the toolset
asks for a re-apply when the set of Tools or their `resourceVersion`s change.
The PoC polls (`dynamic.PollInterval`, 15s) to mirror the cluster-state
watcher; an informer is the production choice.

`ToolsetWatcher` is the one change to the server proper, and it is generic:
any toolset whose surface depends on cluster state can use it.

## What converts, measured

[`examples/dynamic-tools/hack/export`](../../examples/dynamic-tools/hack/export)
takes every registered in-tree tool, keeps its real contract (name,
description, input schema, annotations) and pairs it with a declared action,
or lists it as skipped with a reason. The output is a Helm chart per
ecosystem toolset plus plain manifests for the rest, and
[`conversion-report.md`](../../examples/dynamic-tools/conversion-report.md).

| Toolset | Converted | What is left and why |
|---------|-----------|----------------------|
| kiali | 11 / 11 | nothing: every tool is one POST to `/api/mcp/<tool>` |
| netobserv | 3 / 3 | nothing: GET with a CEL-computed time window |
| tekton | 2 / 6 | restart (get + create), logs (several requests + pod log streams) |
| kubevirt | 4 / 7 (+1 split out) | create (resolves instancetypes/preferences), screenshot (image), troubleshoot (workflow) |
| kcp | 1 / 2 | recursive workspace discovery |
| core | 12 / 19 | exec (stream), run (two creates), the generic `resources_*` family |
| config, helm | 0 / 6 | not Kubernetes API requests |

33 of 54 tools, with the two toolsets that motivated this (Kiali, NetObserv)
at 100 %. The core conversions are a demonstration, not a proposal to move
core out of the server. The generated core manifests are exercised against
envtest in `pkg/mcp/dynamic_test.go`.

## What stays in the server

Streaming (`pods_exec`, `port-forward`), image results (`vm_console_screenshot`),
multi-request workflows (`vm-troubleshoot`, `pipelinerun_troubleshoot`), Helm
release storage, and the generic `resources_*` escape hatch. Everything that
is "one request, shaped" is a candidate for a Tool resource.

## Open questions

- **Scope per target.** Tools are listed from the default target only. In
  multi-cluster mode a Tool defined in cluster A is exposed for cluster B too.
  Options: per-target listing with target-aware filtering (the
  `TargetCompatibilityFilters` mechanism fits), or a `targets` field.
- **Name collisions** with built-in tools: reject, prefix, or let the Tool win?
- **Prompts and resources** as CRDs follow the same pattern (`spec.messages`
  with CEL/argument substitution, mirroring the TOML prompt support).
- **Watch mechanism**: informer instead of polling; watching toolsets enabled
  by a later configuration reload (today only toolsets enabled at startup are
  watched).
- **Limits**: CEL cost limits, response size (8 MiB cap today), timeouts.
- **Composition**: should a Tool be allowed to chain two requests (fetch then
  act)? The current answer is no; that is what prompts and the model are for.
  It is the single largest reason a tool is not convertible (Tekton restart,
  KubeVirt `info_type: all`, pods_run).
- **Output formats**: built-in list tools honor `list_output` (table/yaml);
  dynamic tools render YAML or a CEL projection. A `format` option could
  expose the server's table printer.
- **Enum dispatch across resources**: one tool whose argument selects between
  two resources (KubeVirt start/stop on VMs versus pause on VMIs) has to be
  split in two today.
- **Migration**: which in-tree toolsets could move out entirely (kiali and
  netobserv look like pure `http` tools; most of kcp and tekton are
  `kubernetes` tools), and whether the project wants to host their charts.
