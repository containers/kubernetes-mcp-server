# Dynamic Tools (Tool custom resources)

Status: **Implemented** for the ecosystem toolsets. The `dynamic` toolset is
opt-in (not enabled by default); the former in-tree Kiali, NetObserv, Tekton,
KubeVirt and kcp toolsets have been removed and live on as the Helm charts
under `examples/dynamic-tools/charts`.

Tools and prompts defined declaratively as `Tool` and `Prompt` custom
resources in the cluster, picked up by the server at runtime, instead of
being compiled into the server. The
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

A Tool or Prompt whose name is exposed by another enabled toolset is skipped
and logged at error level, so a resource cannot replace a built-in tool (for
example by declaring `pods_delete` read-only); the other resources are
unaffected. With the owning toolset disabled the name is free (the converted
core manifests rely on this), and enabling it again by reload skips the
resource from then on. Two enabled toolsets exposing the same name is a
server bug and fails the reload, leaving the previous configuration
installed.

Open hardening items (not implemented): restricting the listed Tools by label
selector or allowed names in server configuration, and an allowlist of
`http` hosts.

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
| `response` | `json` (default: decode, fall back to text), `text` (keep the body verbatim) or `binary` (keep the bytes; the tool result carries them as an image/blob content block, `mimeType` labels them when the server does not). |
| `redact` | Remove secrets from a text response with the shared `pkg/redact` rules (bearer tokens, basic-auth flags, PEM blocks, SSH keys, `password=`/`token=`). |

The request is issued through a REST client built from the caller's
`rest.Config`, i.e. the very same transport chain as every built-in tool.

### `http` action

One HTTP request carrying the caller's identity.

| Field | Meaning |
|-------|---------|
| `url` | Absolute base URL. The caller's bearer token is sent as `Authorization: Bearer`. This is what the Kiali and NetObserv toolsets do. |
| `service` | `{namespace, name, port, scheme}`: reached through the API server's service proxy with the caller's `rest.Config`. No TLS or token plumbing, and `services/proxy` RBAC applies. Mutually exclusive with `url`. |
| `method`, `path`, `query`, `queryExpression`, `headers`, `body` | `path` supports `{argument}` placeholders (URL-escaped, required). `query`/`queryExpression`/`body` as above; `{expression: args}` forwards all arguments. |
| `certificateAuthority`, `insecureSkipTLSVerify` | TLS verification for `url`. |
| `response`, `mimeType`, `redact` | As for `kubernetes`. |

### `steps` (composition)

Instead of one action, a Tool may declare a bounded sequence of steps:

```yaml
spec:
  steps:
    - name: existing                 # later expressions see steps.existing
      when: args.action == 'restart' # CEL boolean; false skips the step (result null)
      kubernetes: {verb: get, apiVersion: tekton.dev/v1, kind: PipelineRun, ...}
    - name: restarted
      when: args.action == 'restart'
      kubernetes:
        verb: create
        body: {expression: "{'spec': steps.existing.spec.transformMapEntry(k, v, k == 'status' ? {} : {k: v}), ...}"}
    - name: containers               # a pure expression step: no request
      expression: steps.runs.map(tr, ...).flatten()
    - name: logs
      forEach: steps.containers      # CEL list; runs once per element with `item` bound
      ignoreError: true              # a failed request becomes {"error": message}
      kubernetes: {verb: get, kind: Pod, subresource: log, name: {expression: item.pod}, ...}
  output: ...                        # sees steps.* and response (the last step's result)
```

| Field | Meaning |
|-------|---------|
| `name` | Identifier the later expressions use (`steps.<name>`). |
| `kubernetes` / `http` / `expression` / `fail` / `elicit` | Exactly one. An expression step computes a value without a request; `fail` is a CEL string that, when non-empty, fails the tool with that message (argument and state checks); `elicit` asks the user through MCP elicitation and yields `{action: accept\|decline\|cancel\|unsupported, content}`. |
| `when` | CEL boolean over `args` and `steps`. When false the step is skipped and its result is `null`. |
| `forEach` | CEL list. The action runs once per element with `item` bound; the result is the list of results in order. |
| `ignoreError` | Records a failed request as `{"error": "<message>"}` instead of failing the tool, for gather-style tools. |

Steps run sequentially; there is no looping other than `forEach`, no
recursion and no branching other than `when`. Two limits bound a Tool: at
most 16 steps and at most 64 `forEach` iterations per step. Without an
`output`, the result is the map of all step results. Only the last step can
return bytes: a `response: binary` result of an earlier step, or of any
`forEach` iteration, is reduced to its summary (`{mimeType, size}`).

This is deliberately a small composition model, not a workflow language.
It is exactly enough for the three shapes the in-tree toolsets needed:
fetch-then-act (restart, lifecycle), fan-out (logs per container, guest
info per subresource) and gather (troubleshoot).

### `requires`

A list of `{apiVersion, kind}` the tool depends on. When the server's target
compatibility filters are enabled, the tool is hidden while no target serves
the kind. The KubeVirt chart declares `kubevirt.io/v1` `VirtualMachine` on
every tool, which is what the in-tree toolset did with `HasVirtualMachine`.

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
| `response` | the decoded response (`null` while building the request; the last step's result for composed tools) |
| `steps` | results of the previous steps by name (composed tools) |
| `item` | the current element of a `forEach` step |
| `now` | the current time; `int(now)` is Unix seconds |
| `merge(a, b)` | a map with the entries of `a` overridden by those of `b` |
| `range(n)` | `[0, 1, ..., n-1]`, to walk a `forEach` result next to its input list |
| `yaml(x)`, `json(x)` | render a value as YAML or JSON text (for embedding in prompts) |
| `redact(s)` | remove secrets from a string |
| cel-go extensions | strings, lists, math and two-variable comprehensions (`transformMapEntry` renames or drops keys) |

Strings are returned verbatim (logs, CSV, messages); anything else is
rendered as YAML text and, when it is an object, also returned as
`structuredContent`.

## Prompt resources

Prompts follow the same pattern with a second kind:

```yaml
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Prompt
metadata:
  name: vm-troubleshoot
spec:
  description: ...
  arguments:
    - {name: namespace, required: true}
    - {name: name, required: true}
  steps: [...]                      # the same engine as Tool steps
  messages:
    - role: user
      text: |                       # template: ${expression} over args and steps
        # VM ${args.name}
        ${yaml(steps.vm.status)}
    - role: assistant
      when: has(steps.vm.status)    # optional CEL boolean
      text: I'll analyze the collected data.
```

Templates render strings verbatim, scalars as text and maps or lists as
YAML; `$${` renders a literal `${` for shell-style text that is not an
expression. This is the "server gathers, model reasons" pattern the design skill
prescribes, now as data. The `elicit` step exists for prompts like the
Windows golden image creator, which must ask for EULA consent before it
renders a manifest with `acceptEula: "true"`.

## Confirmation rules with expressions

The Windows EULA check used to be a compiled-in validator because it
inspects the request body. Kube-level confirmation rules now accept an
`expression`: a CEL boolean over `verb`, `kind`, `group`, `version`, `name`,
`namespace` and `object` (the decoded body; an empty map when there is no
body or it is not a JSON object, such as a JSON-patch array, so `has()`
guards evaluate to false instead of erroring; note that `has(a.b.c)` only
tests `c`, so every level is guarded). The KubeVirt chart ships the
EULA rule as [`confirmation-rules.toml`](../../examples/dynamic-tools/charts/kubevirt-mcp-tools/confirmation-rules.toml)
and the in-tree validator is gone: the rule is server configuration that
must be added to `--config-dir` wherever the KubeVirt tools are installed
(`make dynamic-tools-install` collects the rules of the installed charts for
`make run-server`; see [configuration-changes.md](../configuration-changes.md#windows-eula-confirmation)).
An expression that cannot be evaluated fails closed (confirmation is
required and the error is logged), so a rule guards even the requests its
author did not anticipate.

## RBAC metadata

The declaration is enough to derive the server's `_meta` RBAC advertisement
(see [`tool-rbac-metadata.md`](../tool-rbac-metadata.md)) without any code:

- `kubernetes`: **bounded** to `verb` on the resource (declared, or resolved
  through discovery), with namespace and resource name mapped from the
  `argument`/`value` references and a fixed subresource reduced to its first
  segment (`proxy/stats/summary` advertises `nodes/proxy`). Falls back to
  **unbounded** when the kind cannot be resolved at registration time or when
  the subresource, namespace or name comes from a CEL expression.
- `steps`: the **union** of every step's requirements; unbounded if any step
  is. When a step also calls an external `url`, the bounded declaration
  carries a `reason` naming it.
- `http` with `service`: **bounded** to `get`/`create` on `services/proxy`
  for that Service.
- `http` with `url`: **none**, with a reason naming the URL.

## Reload

Tool resources are read from the **default target** when toolsets are applied
(startup, SIGHUP, kubeconfig or cluster-state change). The toolset also
implements the new optional `api.ToolsetWatcher` interface: the server calls
`WatchToolset` for each enabled toolset that implements it, restarts the
watches after every configuration reload (so a toolset enabled by SIGHUP or
a drop-in change is watched too), and the toolset asks for a re-apply when
the set of Tools or their `resourceVersion`s change. The PoC polls
(`dynamic.DefaultPollInterval`, 15s) to mirror the cluster-state watcher; an
informer is the production choice. A list failure (typically a missing
ClusterRole) is logged once when it starts and once when it ends, with the
repeats at verbosity 1.

`ToolsetWatcher` is the one change to the server proper, and it is generic:
any toolset whose surface depends on cluster state can use it.

## What converted

The charts were produced by a one-off exporter that took every registered
in-tree tool and prompt, kept its real contract (name, description, input
schema or arguments, annotations) and paired it with a declared action, or
listed it as skipped with a reason. The exporter was removed together with
the in-tree toolsets it read from; the charts, the plain manifests under
`converted/` and
[`conversion-report.md`](../../examples/dynamic-tools/conversion-report.md)
are maintained by hand since (`pkg/toolsets/dynamic/examples_test.go` keeps
them valid). The figures below are the state at conversion time.

| Toolset | Tools | Prompts | What is left and why |
|---------|-------|---------|----------------------|
| kiali | 11 / 11 | 11 / 11 | nothing |
| netobserv | 3 / 3 | | nothing |
| tekton | 6 / 6 | 1 / 1 | nothing |
| kubevirt | 6 / 7 | 3 / 3 (+ `vm-create`) | `vm_create`, replaced by the `vm-create` prompt and `vm_create_from_template` |
| kcp | 1 / 2 | | `kcp_workspaces_list`, redundant with the provider's `workspace_list` |
| core | 13 / 19 | 0 / 1 | exec (stream), the generic `resources_*` family, `cluster-health-check` (base server) |
| config, helm | 0 / 6 | | not Kubernetes API requests |

55 of 70 tools and prompts. Every third-party toolset is now either fully
converted (Kiali, NetObserv, Tekton) or converted except for one tool whose
replacement is a prompt (KubeVirt). What remains is the base server, Helm,
and a kcp tool the provider already covers. The core conversions are a
demonstration, not a proposal to move core out of the server. The generated
Tekton, KubeVirt and core tools and the KubeVirt and Tekton prompts are
exercised against envtest in `pkg/mcp/dynamic_test.go`, which also installs
the shipped CRD schemas on the API server and verifies the EULA confirmation
rule.

Four generic features closed the gaps found in the previous round, none of
them KubeVirt-specific: `response: binary` (screenshots), `redact` (pod logs
in troubleshoot tools and prompts), `requires` (tool hiding when an API is
absent), and `elicit`/`fail` steps (the EULA consent flow and argument
checks). What a Tool or Prompt still cannot express:

- **Heuristic matching** (`vm_create`): handed to the model as a prompt that
  lists the candidates, or to KubeVirt templates.
- **Capability-dependent behavior**: `pods_run` creates an OpenShift Route
  when the Route API exists; a Tool cannot ask what the cluster serves.
- **Unbounded recursion** (`kcp_workspaces_list`), which the provider does.
- **Pixel-level post-processing**: the screenshot description no longer
  states the image dimensions.

## What stays in the server

Streaming (`pods_exec`, `port-forward`), Helm release storage, the cluster
providers, the generic `resources_*` escape hatch, and the base prompts.
Everything else that the in-tree toolsets do is now expressible as a Tool or
Prompt resource.

## Open questions

- **Scope per target.** Tools are listed from the default target only. In
  multi-cluster mode a Tool defined in cluster A is exposed for cluster B too.
  Options: per-target listing with target-aware filtering (the
  `TargetCompatibilityFilters` mechanism fits), or a `targets` field.
- **MCP resources** as CRDs would follow the same pattern as prompts.
- **Policy as a cluster resource**: the EULA confirmation rule is server
  configuration that has to travel next to the KubeVirt chart; whether rules
  like it should also be deliverable as a cluster resource.
- **Watch mechanism**: informer instead of polling.
- **Limits**: response size (8 MiB today, larger responses are rejected), timeouts. CEL evaluations
  are bounded by a per-call cost limit and `range(n)` by `MaxRange`.
- **Composition** is now in (`steps`), bounded to 16 steps and 64 fan-out
  iterations, sequential, no recursion. The open question is whether that
  is the right ceiling: prompts (`vm-troubleshoot`, `pipeline-troubleshoot`,
  `hco-status`) would need the same engine, and every added construct moves
  the Tool resource closer to a workflow language the server then has to
  own forever.
- **Output formats**: built-in list tools honor `list_output` (table/yaml);
  dynamic tools render YAML or a CEL projection. A `format` option could
  expose the server's table printer.
- **Enum dispatch across resources**: one tool whose argument selects between
  two resources (KubeVirt start/stop on VMs versus pause on VMIs) has to be
  split in two today.
- **Migration**: which in-tree toolsets could move out entirely (kiali and
  netobserv look like pure `http` tools; most of kcp and tekton are
  `kubernetes` tools), and whether the project wants to host their charts.
