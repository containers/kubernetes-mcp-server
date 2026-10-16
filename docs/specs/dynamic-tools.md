# Dynamic Tools (Tool custom resources)

Status: **Implemented**. The `dynamic` toolset is opt-in (not enabled by
default); the ecosystem integrations (Kiali, NetObserv, Tekton, KubeVirt,
Loki, Prometheus, Tempo) are delivered as the Helm charts under
`charts/dynamic-tools`.

Tools and prompts defined declaratively as `Tool` and `Prompt` custom
resources in the cluster, grouped into families by `ToolSet` resources, and
picked up by the server at runtime instead of being compiled into it. The
goal is a small, rock-solid base server whose ecosystem integrations (Kiali,
NetObserv, Tekton, KubeVirt, ...) are delivered and versioned independently,
the way CRDs and operators extend Kubernetes itself.

The CRD, the charts and hand-written example Tools live in
[`charts/dynamic-tools`](../../charts/dynamic-tools). This document
says what a Tool can express; [`tools-for-models.md`](tools-for-models.md)
says what a Tool should express for the model that will use it, and which
engine features (`ToolSet`, output budget, shared argument vocabulary,
`variants`, `examples`, lints) make that the easy path.

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
- The server lists Tool, Prompt and ToolSet resources with **its own**
  identity (the default target's kubeconfig / in-cluster service account),
  never with the caller's token, through the provider's
  `GetServerKubernetes`; this is what makes the dynamic toolset work under
  `require_oauth`, where no caller exists when toolsets are applied.
  Operators decide what that identity may read: `get`, `list` and `watch`
  on `tools`, `prompts` and `toolsets` in `tools.kubernetes-mcp-server.io`,
  plus `update` on `tools/status`, `prompts/status` and `toolsets/status`
  for the server to report what it did with each resource (optional).
- Tool **calls** run with the **caller's** identity, except that a family
  may declare `identity: server` for its `http` actions, which every tool
  of the family then reports. A Tool cannot escalate:
  a `kubernetes` action is subject to the caller's RBAC, `denied_resources`,
  the validators and the confirmation rules exactly like a built-in tool,
  because it goes through the same `rest.Config` and round tripper. An `http`
  action to an in-cluster Service goes through the API server's service
  proxy, so it needs `services/proxy` RBAC on that Service.
- `read_only` and `disable_destructive` apply through the declared
  annotations. The Kubernetes verbs a Tool performs are static in its
  definition, so the server checks them: a `readOnly` Tool that creates,
  updates, patches or deletes, or a deleting Tool without `destructive:
  true`, is rejected at registration (and by the CRD). HTTP methods are not
  checked, since a read may be a POST (Kiali), so a Tool that lies about an
  HTTP action is an operator trust problem, which is one reason the resource
  is cluster-scoped. Prompts carry no annotations: in `read_only` mode a
  Prompt whose steps mutate through the Kubernetes API is withheld.

A Tool or Prompt whose name is exposed by another toolset — enabled or
not — is skipped and logged at error level, so a resource cannot replace a
built-in tool (for example by declaring `pods_delete` read-only), not even
while the toolset that owns the name is disabled; the other resources are
unaffected. Two enabled toolsets exposing the same name is a
server bug and fails the reload, leaving the previous configuration
installed — for two dynamic families too: to the server a family is a
toolset like any other, while within one family the collision is resolved
per resource (`NameTaken`).

`allowed_http_hosts` bounds where http actions may send credentials: when
set, a Tool with a fixed `url` outside it is withheld at registration (its
status says so), and every request — a `urlExpression` result, or any
`url` — is checked again at call time. `service` actions are not affected:
they carry no user token and are bounded by `services/proxy` RBAC.

Open hardening item (not implemented): restricting the listed Tools by label
selector or allowed names in server configuration.

## Resource

```yaml
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: deployments-summary          # the MCP tool name is this with '-' -> '_'
spec:
  toolset: apps                      # optional: the ToolSet (family) this tool belongs to
  name: deployments_summary          # optional: an MCP tool name other than the derived one
  title: "Deployments: Summary"      # optional annotation
  description: ...                   # written for the model, required
  readOnly: true                     # MCP annotations; drive read_only /
  destructive: false                 #   disable_destructive filtering
  idempotent: true
  inputSchema: {...}                 # JSON Schema object of the arguments
  kubernetes: {...}                  # exactly one of kubernetes / http
  http: {...}
  output: <CEL>                      # optional projection of the response
  outputFull: <CEL>                  # optional projection behind detail: full
  limit: {default: 50, max: 500}     # optional output budget of a list-shaped result
  meta: {...}                        # optional, passed through as the tool's _meta
  timeout: 30s                       # bounds one call, every step included (default 60s)
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
| `queryExpression` | CEL over `args` producing a map of query parameters, merged under `query`. Parameters that turn a request into a stream or change what a write does (`watch`, `follow`, `dryRun`, `force`, `propagationPolicy`, `gracePeriodSeconds`, `fieldManager`, `timeoutSeconds`, ...) are refused unless the same key is a declared `query` entry: `{expression: args}` forwards what the model sends, and the model must not pick them. |
| `body` | `{argument: manifest}` (YAML/JSON string or object argument), `{value: {...}}` (fixed JSON) or `{expression: <CEL over args>}`. Defaults to `{}` for `create`/`update`/`patch`, which is what subresource endpoints such as KubeVirt's `pause` expect. `patch` sends a merge patch. |
| `response` | `json` (default: decode when the server's Content-Type says JSON, or is absent as the Kubernetes API's is, and the body is one document; anything else stays text, so a log whose lines parse as JSON is not reinterpreted; an explicit `json` forces decoding), `text` (keep the body verbatim) or `binary` (keep the bytes; the tool result carries them as an MCP image content block whatever the media type, labelled by `mimeType` when declared, else by the server's Content-Type, else `application/octet-stream`). |
| `redact` | Remove secrets from a text response with the shared `pkg/redact` rules (bearer tokens, basic-auth flags, PEM blocks, SSH keys, `password=`/`token=`). |

The request is issued through a REST client built from the caller's
`rest.Config`, i.e. the very same transport chain as every built-in tool.

### `http` action

One HTTP request carrying the caller's identity.

| Field | Meaning |
|-------|---------|
| `url` | Absolute base URL. The caller's bearer token is sent as `Authorization: Bearer` (in token-less modes, stdio or a server without `require_oauth`, that is the kubeconfig's or service account's token the derived client holds), or the server's own credentials when the ToolSet declares `identity: server`. This is what the Kiali and NetObserv toolsets do. |
| `urlExpression` | CEL string computing the base URL at run time from `steps` and `item`: a LokiStack's Route host, a TempoStack's gateway Service. It must not reference `args` directly, and should be derived from objects earlier steps fetched (the check catches the naive `args.url`; it cannot see through an expression step, so the definition is what keeps the model from choosing where the caller's token goes); the result must be an absolute http(s) URL. The TLS settings below still apply. Mutually exclusive with `url` and `service`. |
| `service` | `{namespace, name, port, scheme}`: reached through the API server's service proxy with the caller's `rest.Config`. No TLS or token plumbing, and `services/proxy` RBAC applies; the API server authenticates the caller and strips the `Authorization` header before proxying, so the backend sees no user token (a backend that authorizes per user, such as a LokiStack gateway, needs `url` or `urlExpression`). Mutually exclusive with `url`. |
| `method`, `path`, `query`, `queryExpression`, `headers`, `body` | `path` is a reference: a plain string supports `{argument}` placeholders (URL-escaped, required), an `{argument}`/`{expression}` reference is appended verbatim. Whatever it resolves to must start with a single `/` and carry no `..` segment, query or fragment, and the request is refused if the parsed URL's scheme or host differs from the base: a path can never move the caller's token to another origin or walk out of a service proxy prefix. `headers` values are references too (an empty value omits the header: a tenant header only when a tenant is given); `Authorization`, `Proxy-Authorization`, `Host`, `Cookie` and `Impersonate-*` are refused, since in service mode they would replace the caller's identity. A list value in `queryExpression` becomes a repeated parameter (`match[]`, Alertmanager's `filter`). `query`/`queryExpression`/`body` as above; `{expression: args}` forwards all arguments. |
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
| `kubernetes` / `http` / `expression` / `fail` / `elicit` | Exactly one. An expression step computes a value without a request; `fail` is a CEL string that, when non-empty, fails the tool with that message (argument and state checks; a non-string result such as a bare boolean is an error, so `args.n > 3` must be written `args.n > 3 ? 'message' : ''`); `elicit` asks the user through MCP elicitation and yields `{action: accept\|decline\|cancel\|unsupported, content}` (`content` is absent when the client does not support elicitation, so guard it with `has()`). |
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

### `variants`

One tool with modes, when the modes differ in what they do rather than in
one argument. Instead of a `kubernetes`, `http` or `steps`, the Tool declares:

```yaml
spec:
  variants:
    argument: action                 # the enum argument the engine adds
    description: What to do with the VirtualMachine.
    default: start                   # optional: the option when the argument is omitted; without it the argument is required
    options:
      start:
        description: run it (runStrategy Always); a no-op if it already runs
        steps: [...]
        output: "{'message': ..., 'vm': steps.final}"   # optional: overrides spec.output
      pause:
        description: suspend the running instance in place
        kubernetes: {verb: update, apiVersion: subresources.kubevirt.io/v1, resource: virtualmachineinstances, subresource: pause, ...}
```

Each option carries exactly one of `kubernetes`, `http` or `steps`, and may
carry its own `output`; everything else (`inputSchema`, `requires`,
`timeout`, `limit`, `outputFull`, `confirmation`) is the Tool's and is shared.
The engine adds `action` to the input schema as an `enum` (required unless
`default` names an option) whose description lists every option with its
description, so the model sees one tool and chooses a mode the way it would
choose any other argument. A call
whose `action` is not an option is refused with the list of options. Each
option is compiled and validated as a Tool would be, and the RBAC metadata
of the Tool is the union of its options'. A Tool declares at most 16 options;
at most one of `kubernetes`, `http`, `steps` and `variants` may be set.

### `examples`

```yaml
spec:
  examples:
    - summary: CPU of a deployment over the last hour
      arguments: {query: 'sum(rate(container_cpu_usage_seconds_total{pod=~"api-.*"}[5m]))', duration: 1h, step: 1m}
```

Each example is rendered at the end of the description as `Example:
<summary>: <arguments as JSON>`, so every tool on the server shows its
examples the same way. Registration checks each example against the input
schema: every argument it names is declared (or one the engine adds:
`limit`, `continue`, `detail`, the variants argument), every required
argument is given, and enum arguments carry one of their values. An example
the tool would refuse is a validation error.

### `requires`

A list of `{apiVersion, kind}` the tool depends on. When the server's target
compatibility filters are enabled, the tool is hidden while no target serves
the kind. Whether or not they are, a call checks the target it runs against
(the model may name another cluster than the one the listing saw) and is
refused with "needs the `<apiVersion> <Kind>` API, which this cluster does
not serve" before any step runs. The KubeVirt chart declares `kubevirt.io/v1`
`VirtualMachine` on every tool, which is what the in-tree toolset did with
`HasVirtualMachine`; a ToolSet's `requires` is inherited by its tools.

### `output`

A [CEL](https://cel.dev) expression evaluated with `response` (the decoded
response) and `args` (the tool arguments). Its result replaces the response
in the tool result. CEL is already in the dependency graph (CRD validation
rules use it) and is the language operators already write in Kubernetes.
Expressions are compiled when the toolset is applied, so a broken Tool is
logged and skipped rather than failing on first call; so is a resource
whose shape does not fit the definition (a number where a reference is
expected), which the CRD's open reference fields cannot catch. Neither
takes the other resources down.

Every expression (output, body, query, references) sees the same environment:

| Name | Meaning |
|------|---------|
| `args` | the tool arguments, `map<string, dyn>` |
| `response` | the decoded response (`null` while building the request; the last step's result for composed tools) |
| `steps` | results of the previous steps by name (composed tools) |
| `item` | the current element of a `forEach` step |
| `now` | the current time; `int(now)` is Unix seconds |
| `cluster.serves(apiVersion, kind)` | whether the target of the call serves the kind (the RESTMapper lookup a `kubernetes` action performs); for `when` conditions on optional APIs such as OpenShift Routes |
| `merge(a, b)` | a map with the entries of `a` overridden by those of `b` |
| `range(n)` | `[0, 1, ..., n-1]`, to walk a `forEach` result next to its input list |
| `sum(list)` | the sum of a list of numbers as a double, since CEL comprehensions cannot fold (series statistics) |
| `time(s)` | a time argument as Unix seconds, in the forms every tool accepts: RFC 3339, Unix seconds, `NOW`, `NOW-1h` (the shared vocabulary of `start`/`end`); a bad value is a clear error naming the forms |
| `seconds(s)` | a duration argument (`30s`, `15m`, `1h30m`, `2d`, `1w`) in seconds (the vocabulary of `duration`) |
| `cel.bind(name, value, expr)` | a local binding: name a sub-expression once instead of repeating it (CEL has no `let`) |
| `yaml(x)`, `json(x)` | render a value as YAML or JSON text (for embedding in prompts) |
| `redact(s)` | remove secrets from a string |
| cel-go extensions | strings, lists, math and two-variable comprehensions (`transformMapEntry` renames or drops keys) |

Errors are written for the model: a tool failure names the tool and the
step, a CEL runtime error carries the CEL message ("no such key: items")
without the expression source, which goes to the server log at verbosity 1
where the author looks, and a type mismatch in `when`, `forEach` or `fail`
names the type as an author would ("got map", not a Go type name).

Strings are returned verbatim (logs, CSV, messages); anything else is
rendered as YAML text and, when it is an object, also returned as
`structuredContent`. Numbers: a whole number is an integer wherever it came
from (a JSON response, a tool argument, a CEL result), so `response.count
* 2` and `string(steps.range.start)` behave; only fractional numbers are
doubles, and mixing the two in arithmetic needs an explicit `int()` or
`double()`, as CEL requires.

## Output budget

The engine, not the author, owns result size (the rationale is in
[`tools-for-models.md`](tools-for-models.md)):

```yaml
spec:
  output: ...                        # the concise projection (the default)
  outputFull: ...                    # optional: the projection behind detail: full
  limit:                             # for list-shaped results
    default: 50                      # items returned when the model gives no limit
    max: 500                         # the most it may ask for
    argument: limit                  # the argument's name
    field: metrics                   # the key holding the list when the result is a map (optional when it has one list)
    narrowWith: [match]              # what the trailer tells the model to narrow with
```

- **Default projection.** A `kubernetes` `list` without an `output` returns
  the essentials of every item (`kind`, `name`, `namespace`, `phase`,
  `ready` or the first `True` condition), never the raw objects. Every
  Kubernetes response loses `managedFields` and the
  `last-applied-configuration` annotation, as kubectl does.
- **`limit`.** Declaring it adds the `limit` argument (with the engine's
  description, default and maximum) to the input schema. A single
  `kubernetes` `list` action is paginated by the API server: the engine
  sends `limit` and `continue`, adds the `continue` argument, and when a
  page is not the last the result ends with `# showing N items; more are
  available: call again with continue="..."`. Any other list-shaped result
  (a steps tool, an `http` response, a map's `field`) is cut by the engine
  to the limit and ends with `# showing N of M items; narrow with
  labelSelector, or raise limit (at most 500)`; a map result also carries
  `truncated: {returned, total, narrowWith}` in its structured content.
  Without `narrowWith`, the declared `labelSelector`/`fieldSelector`
  arguments are named.
- **`detail`.** Declaring `outputFull` adds `detail: concise | full`
  (default `concise`); `full` runs the `outputFull` projection instead.
- **Standard argument descriptions.** A declared `namespace`, `name`,
  `labelSelector`, `fieldSelector`, `start`, `end` or `duration` argument
  without a description gets the description every tool on the server
  uses; an author's own description is kept.

The byte caps are a separate, lower layer: the engine's 8 MiB response cap,
and the server-level byte caps planned in
[kubernetes-mcp-server#1471](https://github.com/containers/kubernetes-mcp-server/pull/1471),
which truncate with a notice. The budget keeps results from reaching them with
something the model can act on; the pagination it adds is the per-tool
answer to [kubernetes-mcp-server#1473](https://github.com/containers/kubernetes-mcp-server/issues/1473).

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

## ToolSet resources

A `ToolSet` is the family a chart's Tools and Prompts belong to: the unit
operators enable, the prefix its tools are named with, the instructions the
model gets about the family as a whole, and the requirements and identity
every tool of the family shares (the rationale is in
[`tools-for-models.md`](tools-for-models.md)).

```yaml
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: ToolSet
metadata:
  name: prometheus
spec:
  description: Prometheus and Alertmanager queries      # for operators and the toolsets listing
  prefix: prom                                           # every tool of the family is named prom_*
  instructions: |                                        # contributed to the MCP server instructions
    Find the exact metric name with prom_metrics before writing a query; ...
  requires:                                              # inherited by every tool (a tool may add its own)
    - {apiVersion: monitoring.coreos.com/v1, kind: Prometheus}
  identity: caller                                       # caller (default) | server
```

- A Tool or Prompt joins a family with `spec.toolset: <name>`. One whose
  family does not exist, or whose name lacks the family's prefix, is skipped
  and logged: tools are named `<prefix>_...`, prompts `<prefix>-...` (MCP
  prompt names are dashed).
- **Enablement**: `toolsets = ["core", "dynamic/prometheus", "dynamic/loki"]`
  enables families; plain `dynamic` enables every family and the Tools that
  belong to none. A toolset and one of its own families cannot both be
  enabled. `dynamic/<name>` is accepted without the cluster being reachable,
  so configuration validates offline; a family whose ToolSet does not exist
  (yet) is simply empty.
- **Instructions** of every enabled family are appended to the MCP server
  instructions, each under a `## <name>` heading. They are recomputed
  whenever the toolsets are applied (startup, a configuration reload, a
  ToolSet resource that appears or changes) and sent to every session that
  initializes from then on; a session that is already open keeps the text
  it was given.
- **`requires`** of the family is prepended to each Tool's own.
- **`identity: server`** makes the family's `http` actions send the
  server's own credentials (the target's kubeconfig or in-cluster service
  account token, obtained through the provider for the call's target)
  instead of the caller's: a declaration the RBAC metadata reports
  ("authenticated with the server's own credentials (ToolSet identity:
  server)"). The charts expose it as the `identity` value. `kubernetes`
  actions always run as the caller.
- The watcher re-applies the toolsets when ToolSet resources change, as it
  does for Tools and Prompts.

## Status

Every Tool, Prompt and ToolSet has a `status` subresource the server writes
when it applies the toolsets: the MCP name the resource is exposed as
(`status.name`) and one `Ready` condition saying what it did with the
resource and why, so `kubectl get tools` answers "why is my tool not there"
without the server's log.

```
$ kubectl get tools
NAME             READY   REASON           TOOL
loki-labels      True    Registered       loki_labels
mislabeled       False   MissingPrefix    labels
orphan           False   UnknownToolSet
```

Reasons: `Registered`; `Invalid` (validation or CEL compilation, the
message is the error); `Malformed` (the shape does not fit the definition);
`ShadowsBuiltIn`; `NameTaken` (another resource exposes the name first);
`UnknownToolSet`; `MissingPrefix`; `Withheld` (policy: a mutating Prompt
under `read_only`, or an http Tool whose url host is not in
`allowed_http_hosts`). `observedGeneration` is the generation the condition
describes. The condition is written only when it changes, through the
server's own identity, and is best effort: without `update` on the status
subresources the tools work regardless and the server says so once. A
resource of a family that is not enabled is not written to: only the
toolset that owns a resource decides about it.

## Lints

Beyond validation, registration checks a Tool against the guidance of
[`tools-for-models.md`](tools-for-models.md) and reports what it does not
follow in the message of its `Ready` condition (`exposed as tool x; lint:
...`) and in the log at verbosity 1. None of it hides the tool: an operator
sees the advice next to the resource, and the shipped charts are tested to
carry none. The lints are:

- a description shorter than a sentence;
- an argument without a description (the standard names `namespace`,
  `name`, `labelSelector`, `fieldSelector`, `start`, `end` and `duration`
  get the engine's);
- a string argument whose description spells out quoted values ("one of
  'a', 'b'") without declaring `enum`;
- a `list` verb with neither `output` nor `limit`;
- no `examples`.

`kubernetes-mcp-server check <file|directory|->` runs the same validation and
lints offline, on manifests or on a rendered chart (`helm template ... |
kubernetes-mcp-server check --strict -`), and prints one `PASS:`, `LINT:` or
`FAIL:` line per resource; `--strict` fails on lints.

Two things are refused rather than reported: a `queryExpression` or
`body.expression` that is exactly `args` (the model's arguments forwarded
wholesale), and a tool name without its ToolSet's prefix. A ToolSet that
exposes more than twelve tools is logged at registration.

## Confirmation rules with expressions

The Windows EULA check used to be a compiled-in validator because it
inspects the request body. Kube-level confirmation rules now accept an
`expression`: a CEL boolean over `verb`, `kind`, `group`, `version`, `name`,
`namespace` and `object` (the decoded body; an empty map when there is no
body or it is not a JSON object, such as a JSON-patch array, so `has()`
guards evaluate to false instead of erroring; note that `has(a.b.c)` only
tests `c`, so every level is guarded). The KubeVirt chart ships the
EULA rule as [`confirmation-rules.toml`](../../charts/dynamic-tools/kubevirt-mcp-tools/confirmation-rules.toml)
and the in-tree validator is gone: the rule is server configuration that
must be added to `--config-dir` wherever the KubeVirt tools are installed
(`make dynamic-tools-install` collects the rules of the installed charts for
`make run-server`; see [configuration-changes.md](../configuration-changes.md#windows-eula-confirmation)).
An expression that cannot be evaluated fails closed (confirmation is
required and the error is logged), so a rule guards even the requests its
author did not anticipate.

TODO: the EULA rule travels with the KubeVirt chart but is server
configuration, so an upgraded deployment that enables the KubeVirt family
without adding the rule loses the gate silently. Surfacing that at startup
(needs a heuristic: the KubeVirt family is enabled and no confirmation
rule names a `tekton.dev` PipelineRun) would turn the silence into a log
line; deliberately not built, because the heuristic is the kind that
rots.

## RBAC metadata

The declaration is enough to derive the server's `_meta` RBAC advertisement
(see [`tool-rbac-metadata.md`](../tool-rbac-metadata.md)) without any code:

- `kubernetes`: **bounded** to `verb` on the resource (declared, or resolved
  through discovery), with namespace and resource name mapped from the
  `argument`/`value` references and a fixed subresource reduced to its first
  segment (`proxy/stats/summary` advertises `nodes/proxy`). A namespace from
  an expression, or an argument with `all: true` (an omitted argument lists
  everything), advertises all namespaces; a name from an expression
  advertises any name. Falls back to **unbounded** only when the kind cannot
  be resolved at registration time or when the subresource comes from an
  argument, an expression or a `{placeholder}`.
- `steps`: the **union** of every step's requirements; unbounded if any step
  is. When a step also calls an external `url`, the bounded declaration
  carries a `reason` naming it.
- `http` with `service`: **bounded** to `services/proxy` for that Service,
  with the verb the API server derives from the HTTP `method` (`get` for
  GET/HEAD, `create` for POST, `update` for PUT, `patch` for PATCH, `delete`
  for DELETE).
- `http` with `url`: **none**, with a reason naming the URL; with
  `urlExpression`, the reason names the expression instead.

## Reload

Tool, Prompt and ToolSet resources are read from the **default target** when toolsets are applied
(startup, SIGHUP, kubeconfig or cluster-state change). The toolset also
implements the new optional `api.ToolsetWatcher` interface: the server calls
`WatchToolset` for each enabled toolset that implements it, restarts the
watches after every configuration reload (so a toolset enabled by SIGHUP or
a drop-in change is watched too), and the toolset asks for a re-apply when
the set of Tools or their `resourceVersion`s change. The watcher polls
(`dynamic.DefaultPollInterval`, 15s), mirroring the cluster-state watcher:
no watch machinery to maintain, and tool definitions are not
latency-sensitive. An informer would be an optimization if scale ever
demands one. A list failure (typically a missing
ClusterRole) is logged once when it starts and once when it ends, with the
repeats at verbosity 1.

`ToolsetWatcher` is the one change to the server proper, and it is generic:
any toolset whose surface depends on cluster state can use it.

## Coverage

Every ecosystem integration is served from
`charts/dynamic-tools`: Kiali, NetObserv, Tekton, KubeVirt (all
of it but `vm_create`, whose replacement is the `vm-create` prompt and
`vm_create_from_template`), Loki, Prometheus and Tempo. The `kcp`
`kcp_workspace_describe` tool ships as the plain manifest
`charts/dynamic-tools/tools/kcp-workspace-describe.yaml`;
`kcp_workspaces_list` is not converted because the provider's
`workspace_list` already lists workspaces. What remains in the server is
listed [below](#what-stays-in-the-server).

The integrations drove the engine's feature set: `steps` and `forEach`
(fetch-then-act and fan-out), `response: binary` (screenshots), `redact`
(pod logs in troubleshoot tools and prompts), `requires` (hiding tools
when an API is absent), `elicit` and `fail` steps (consent flows and
argument checks), `urlExpression` (a backend found in the cluster: the
LokiStack's Route, the TempoStack's gateway Service), `path` and
`headers` as references (a tenant path segment and header only when a
tenant is given), repeated query parameters (`match[]`, Alertmanager's
`filter`), `sum` (series statistics), `meta` (the `show_timeseries` UI
hook) and `cluster.serves` (capability-dependent steps: `pods_run`
creates its Route only when the target serves the Route API).

What a Tool or Prompt still cannot express:

- **Heuristic matching** (`vm_create`): handed to the model as a prompt that
  lists the candidates, or to KubeVirt templates.
- **Unbounded recursion** (`kcp_workspaces_list`), which the provider does.
- **Pixel-level post-processing**: the screenshot description does not
  state the image dimensions.
- **Query-language parsing**: the Prometheus guardrails (reject a query
  without label matchers, a blanket regex, or a metric above a cardinality
  limit) and the metric-existence check parse PromQL; a Tool can only
  pattern match. They stay unenforced in the chart.
- **Pure computation**: the `otelcol` tools validate collector
  configuration against JSON schemas embedded in the binary and never
  talk to a backend.

## What stays in the server

Streaming (`pods_exec`, `port-forward`), Helm release storage, the cluster
providers, the generic `resources_*` escape hatch, the base prompts, and the
`observability/otelcol` schema tools. Everything else that the in-tree
toolsets did is now expressible as a Tool or Prompt resource.

## Open questions

- **Chart ownership.** The charts under `charts/dynamic-tools/` make the
  tool definitions data, but this repository is still their release
  vehicle: a Kiali release that changes its API waits on a pull request
  here. The endgame is each project shipping its own chart out of its own
  repository. What has to be true on this side before a chart can move:
  the CRD contract is versioned (a `v1alpha1` change must not break a
  chart, and a breaking bump has a migration path for existing
  resources), the project's CI runs `kubernetes-mcp-server check` on its
  rendered templates, and the house-style lints hold in their charts as
  they do in these. Whether and when a project takes its chart home is
  that project's decision; until one does, this repository hosts them.
- **Scope per target.** Tools are listed from the default target only. In
  multi-cluster mode a Tool defined in cluster A is exposed for cluster B too.
  Options: per-target listing with target-aware filtering (the
  `TargetCompatibilityFilters` mechanism fits), or a `targets` field.
- **MCP resources** as CRDs would follow the same pattern as prompts.
- **Policy as a cluster resource**: the EULA confirmation rule is server
  configuration that has to travel next to the KubeVirt chart; whether rules
  like it should also be deliverable as a cluster resource.
- **Limits**: response size (8 MiB for `http` and `kubernetes` actions
  alike; larger responses are rejected), a per-call `timeout` (default 60s),
  a per-call CEL cost limit, and `range(n)` bounded by `MaxRange`. Whether
  these are the right ceilings is open.
- **Composition** is in (`steps`), bounded to 16 steps and 64 fan-out
  iterations, sequential, no recursion, and Prompts use the same engine
  (`vm-troubleshoot`, `tekton-pipeline-troubleshoot`, `vm-hco-status`). The
  open question is whether that is the right ceiling: every added construct
  moves the Tool resource closer to a workflow language the server then has
  to own forever.
- **Output formats**: built-in list tools honor `list_output` (table/yaml);
  dynamic tools render YAML or a CEL projection. A `format` option could
  expose the server's table printer.