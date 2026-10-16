# Tools for Models

Status: **Implemented.** Open items are listed at the end. Design guidance
for everyone who writes a `Tool`,
`Prompt` or chart for the `dynamic` toolset, and the requirements the engine
implements to make the guidance the easy path. It was written from the
point of view of the consumer of these tools: a language model that receives
`tools/list`, picks a tool from its description alone, fills in arguments
from a schema, and has to make sense of whatever comes back, under a token
budget, often dozens of times per task.

[`dynamic-tools.md`](dynamic-tools.md) says what a Tool *can* express. This
document says what a Tool *should* express, and which of that the engine
enforces or supplies rather than leaving to each author.

## The one-paragraph version

A Tool is a bounded, named question the model can ask of a cluster. The
model is good at deciding which question to ask and at reading the answer;
it is bad at guessing conventions, at recovering from unexplained
truncation, and at telling eleven similar tools apart. So: name tools by
family, describe them by *when*, keep the argument vocabulary the same
across the server, return compact results with stable identifiers, make
every refusal and every truncation say what to do next, and compose on the
server only when the intermediate data would be useless to the model.
Everything below follows from that paragraph.

## House style

Every tool on the server, built-in or dynamic, looks the same to the
model. This is the style the shipped tools follow and the lints check;
a new tool copies it from any existing one.

### Names

- **Tool**: `<prefix>_<what>`. The prefix is the family (a ToolSet's
  `prefix`; for the built-in tools the resource: `pods`, `nodes`,
  `resources`, `helm`). `<what>` is a noun for a read (`prom_labels`,
  `tempo_tags`, `loki_instances`, `pods_list`) and a verb for an action
  (`tekton_start`, `vm_clone`, `pods_delete`). No `get_`, `list_`,
  `_details` or `_by_id`: a noun reads, a plural noun lists, and the
  arguments say which one.
- **Argument**: camelCase, the Kubernetes spelling when there is one
  (`labelSelector`, `apiVersion`), one word when one word does
  (`namespace`, `name`, `label`, `tag`, `query`). Never snake_case, never
  a unit or a type in the name (`lookbackSeconds`, `name_regex`).
- **Title**: `<Family>: <Thing>`, the family as a product name
  (`Prometheus: Query`, `Tekton: Logs`, `Pods: List`, `Context: List`).
- **Variants argument**: `action` when the options are verbs (start,
  stop, restart), `kind` when they are resource kinds (pipeline, task),
  otherwise the noun the options are values of (`info`).

### Argument vocabulary

| Name | Meaning |
|------|---------|
| `namespace`, `name` | The Kubernetes object the tool is about. On a list, `name` is optional and selects one object; `namespace` omitted means every namespace. |
| `start`, `end`, `duration`, `time` | A time range, a lookback, an instant. The engine documents and parses them: RFC 3339, Unix seconds, `NOW`, `NOW-1h`; `30s`, `15m`, `2h`, `1d`, `1w`. |
| `limit`, `continue` | The output budget of a list. `tail` is the budget of a log. |
| `labelSelector`, `fieldSelector` | The Kubernetes selectors. |
| `query` | A query in the backend's language (PromQL, LogQL, TraceQL). `selector` is a series selector, `filter` a backend filter expression, `match` a regular expression. |
| `<family>Namespace`, `<family>Name`, `tenant` | The backend instance when a cluster can host several (`lokiNamespace`, `tempoName`), as listed by `<prefix>_instances`. |
| `<target>` (`context`, `cluster`) | Added by the server in multi-target mode: where to run, as listed by `<target>_list`. |
| `detail` | `concise` (default) or `full`, when the tool declares `outputFull`. |
| `traceId`, and ids in general | camelCase with `Id`. |

### Descriptions

Three to five sentences in this order: the question the tool answers;
when to prefer a sibling; the shape of the result; the defaults. Written
plainly for a capable colleague: no capitals for emphasis, no numbered
procedures, no "IMPORTANT". Reference material a query language needs (a
TraceQL cheat sheet, a filter syntax, the dimensions a backend can group
by) goes once in the ToolSet's `instructions` or in an `enum`, never
repeated in every argument of every tool. Every argument has a
description, every fixed set is an `enum`, every default is declared, and
every tool has one or two `examples`, the first being the common call.

## Guidance for authors

### Shape the surface

- **One family, one prefix, one switch.** Tools belong to a `ToolSet` (see
  below). Every tool name carries the ToolSet's prefix (`loki_`, `tempo_`,
  `kiali_`). A model recognizes a family by its prefix and an operator
  enables or disables it as a unit.
- **Fewer tools with modes beat many tools with one shape each.** If two
  tools differ by one argument, they are one tool with an optional
  argument: `loki_labels` lists label names, or the values of the `label`
  given. More than about a dozen tools in one family and the model has to
  tell near-identical names apart in `tools/list`; split the family into
  two ToolSets or merge tools. Twelve is a convention the shipped charts
  are tested against, not a measured threshold.
- **A Tool must offer more than `resources_get`.** A single request the
  generic family already expresses is not a Tool, it is maintenance. A
  Tool earns its place with what the generic family structurally cannot
  do: composing steps, speaking a backend's query language, discovering
  the backend in the cluster, or shaping the answer. A purpose-named GET
  is `resources_get` with a shorter name.
- **Compose on the server only when the intermediate data is useless to the
  model.** Resolving a `LokiStack` to a gateway URL belongs in steps.
  Deciding which Pod to look at does not: the model reads lists better than
  any CEL filter does, and it will want to explain its choice. Steps exist
  to remove round trips the model would learn nothing from.
- **Do not forward the model's arguments wholesale.** `queryExpression:
  args` and `body: {expression: args}` make the tool's contract the backend's
  query language. Declare the arguments the tool means.

### Describe for the model

A description is the only thing the model sees when choosing. It answers,
in this order: what question the tool answers; when to prefer it over its
siblings; what the result looks like; one example call. Written for a
capable colleague who has never seen this backend, not as a reference
manual and not as a list of commands in capital letters.

- **Workflow guidance goes on the ToolSet, said once.** "Discover metric
  names before querying" is an instruction for the family, contributed to
  the server's instructions, not a paragraph repeated in nine descriptions.
- **Every argument has a description; every fixed set is an enum; every
  default is declared.** The engine refuses a Tool that does not.
- **Examples are data.** `examples` on the Tool are rendered into the
  description in one consistent form and double as eval inputs. A tool that
  ships with examples is a tool whose author has met a model.

### Return what can be acted on

- **Project by default.** A `list` without an `output` returns the engine's
  default projection (names, namespaces, kinds, status summary), never raw
  objects with `managedFields`. Raw objects are an explicit choice (`detail:
  full`).
- **Stable identifiers.** Names and namespaces, never UIDs or resource
  versions unless the tool is about them.
- **Truncation explains itself.** A result that was cut says how much there
  was and which argument narrows it. A dead end is a bug; a next step is a
  result.
- **Errors name the argument to change.** "both `start` and `end` must be
  given together", "tenant `prod` does not exist for instance `simplest`
  (known: dev)". Never the engine's internals, never a CEL source line.
- **Keep one vocabulary.** Time arguments are `start`, `end`, `duration`
  and accept the same formats on every tool; lists take `limit` and
  `continue`; detail is `detail: concise|full`. The engine supplies the
  descriptions and the parsing, so the model learns the convention once.

## What the engine supplies

Each item below is a requirement on `pkg/toolsets/dynamic`, with the shape
it takes in the resources. They are additive to the engine in
[`dynamic-tools.md`](dynamic-tools.md).

### `ToolSet` resource

The unit of enablement, naming and instructions.

```yaml
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: ToolSet
metadata:
  name: prometheus
spec:
  description: Prometheus and Alertmanager queries      # for operators and tools/list
  prefix: prom                                           # every tool is named prom_*
  instructions: |                                        # contributed to the MCP server instructions
    Discover metric names with prom_metrics before writing a PromQL query;
    names differ between clusters. Prefer prom_alerts when the question is
    about something being wrong right now.
  requires:                                              # inherited by every tool (may add its own)
    - {apiVersion: monitoring.coreos.com/v1, kind: Prometheus}
  identity: caller                                       # caller (default) | server; what http actions send
```

- A `Tool` or `Prompt` names its family with `spec.toolset: prometheus`.
  A Tool without a ToolSet is allowed but is listed under the server's
  generic `dynamic` toolset and gets no prefix check, no instructions and no
  switch of its own.
- Operators enable families with the existing toolset list:
  `toolsets = ["core", "config", "dynamic/prometheus", "dynamic/loki"]`;
  plain `dynamic` enables every family. The server's `toolsets/list`
  shows each ToolSet with its description.
- `instructions` of every enabled ToolSet are appended to the MCP server
  instructions under a heading with the ToolSet's name.
- `identity` is the only place a chart can ask for the server's own
  credentials on an `http` action. It
  is declared on the family, not the tool, and the RBAC metadata of every
  tool in the family reports it.

### Output budget (implemented)

The engine, not the author, owns result size. On the `Tool`:

```yaml
spec:
  output: ...                        # as today: a CEL projection (concise)
  outputFull: ...                    # optional: the projection behind detail: full
  limit:                             # for list-shaped results
    default: 50                      # items returned when the model gives no limit
    max: 500
    argument: limit                  # the argument name (default: limit)
```

- A `list` verb without `output` uses the default projection: for every
  item `kind`, `namespace`, `name`, and a status summary (`phase`, `ready`,
  the first `True` condition) when present; `managedFields` and
  `last-applied` annotations are never returned.
- When `limit` is declared, the engine adds `limit` (integer, with the
  engine's description) to the input schema, applies it, and when the
  result was cut appends a trailer to the text result and a
  `truncated: {returned, total, narrowWith: [labelSelector, fieldSelector]}`
  entry to the structured result. For Kubernetes `list` verbs the engine
  also adds `continue` and passes the API server's continue token through.
- When `outputFull` is declared, the engine adds `detail` (`concise` |
  `full`, default `concise`) to the input schema.
- The 8 MiB cap stays as the hard limit; the budget is what keeps a tool
  from reaching it.

### Shared argument vocabulary (implemented)

Reserved argument names the engine recognizes when a Tool declares them. The
author writes `{type: string}`; the engine fills in the description and
provides the parsing in CEL, so every tool on the server accepts the same
formats.

| Argument | Type | Engine description (abridged) | CEL |
|----------|------|-------------------------------|-----|
| `start`, `end` | string | RFC 3339, Unix seconds, `NOW`, or `NOW±duration` (`NOW-1h`) | `time(args.start)` → Unix seconds |
| `duration` | string | Lookback from now: `30s`, `15m`, `2h`, `1d`, `1w` | `seconds(args.duration)` |
| `limit`, `continue` | integer, string | supplied by the output budget | |
| `detail` | enum | supplied by `outputFull` | |
| `namespace`, `name`, `labelSelector`, `fieldSelector` | string | the Kubernetes meanings | |

`time()` and `seconds()` are plain CEL functions and may be used on any
argument; the table only says which names get the standard description for
free. A Tool that declares one of these names with its own description keeps
its own.

### Variants (implemented)

One tool with modes, without a `when` on every step:

```yaml
spec:
  variants:
    argument: action                 # the enum argument that selects the mode
    options:
      start: {description: run it, steps: [...], output: ...}
      stop: {description: halt it, steps: [...], output: ...}
      pause: {description: suspend the instance in place, kubernetes: {...}}
```

The engine adds `action` as a required enum argument whose description
lists the options, validates each option as it would a Tool, lets an option
carry its own `output`, and reports the union of their RBAC. This replaces
the two-tools-for-one-question shape, and the `when: args.action == ...`
on every step that the single-tool shape used to need.

### Examples (implemented)

```yaml
spec:
  examples:
    - summary: Last hour of CPU for a deployment
      arguments: {query: 'rate(container_cpu_usage_seconds_total{pod=~"api-.*"}[5m])', duration: 1h, step: 1m}
```

Rendered at the end of the description as `Example: <summary>: <arguments as
JSON>`, one per example, and checked at registration against the input
schema (declared names, required arguments, enum values). Exporting them as
the seed of an eval task per tool is not built; an eval task needs setup and
assertions an example does not carry.

### Lints (implemented)

Registration refuses a tool name that does not carry its ToolSet's prefix,
and `queryExpression: args` or `body: {expression: args}` on a `kubernetes`
action (pass-through of the model's arguments). It reports, in the Ready
condition's message and the log, without hiding the tool:

- a description shorter than one sentence, an argument without a
  description or not spelled in camelCase, or a string argument whose
  description spells out quoted values without declaring `enum`;
- a `list` verb with neither `output` nor `limit` (the default projection
  is applied, but the author is told);
- a tool without `examples`.

More than twelve tools in one ToolSet is logged at registration. Authors
run the same checks offline with `kubernetes-mcp-server check` (on files, a
directory, or a rendered chart on standard input; `--strict` fails on
lints). The shipped charts and the built-in tools are tested to carry no
lint at all
(`api.LintTool` is the shared rule set; the built-in tools declare
`examples` on `api.Tool` and the server renders them the same way).

### Errors (implemented)

A tool failure returned to the model names the tool, the step when there is
one, and a message written for the model. CEL evaluation errors are reported
as "step <name>: expression failed: <CEL message>" without the expression
source; the source goes to the server log at verbosity 1. A `requires` the
call's target does not serve, and a `timeout` (default 60s) that expires,
are reported in the same terms.

## Open items

Threads to pick up later, none of which this document contorts around:

- **Output budget for the built-in list tools.** `pods_list`,
  `resources_list`, `events_list` and `namespaces_list` return every
  object; the dynamic engine's `limit`, `continue` and default projection
  have no counterpart in the Go tools. Not built here on purpose: PR #1471
  (byte caps) and #1473 (pagination) are the layers this sits on, and a
  third implementation would collide. When they land, the `Limit` shape of
  the dynamic engine (`default`, `max`, the trailer, `truncated` in the
  structured result) is the contract to reuse.
- **Eval export of `examples`.** An eval task needs setup and assertions an
  example does not carry; the examples are checked against the schema and
  rendered, nothing more.
- **Instructions already given.** A session keeps the instructions it
  initialized with; a ToolSet created afterwards reaches the next session.
  The MCP protocol has no notification for changed instructions.

## Non-goals

- A generic "run any request" tool: the point of a Tool is a bounded, named
  question.
- Loops or recursion in steps: when a task needs a loop, the model should
  drive it.
- `identity: server` on a single tool: it is a family-level decision the
  RBAC metadata must report.
- Hiding tools behind a meta-tool to reduce the list: ToolSet enablement and
  the twelve-tool guideline keep the list small instead.
