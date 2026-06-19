# Structured Tool Output Conventions

Per the [MCP specification](https://modelcontextprotocol.io/specification/2025-11-25/server/tools#structured-content),
tools may return a `structuredContent` field alongside the regular text content
block, allowing MCP clients to consume typed data without parsing prose. This
document defines the repo-wide conventions a new structured emitter should
follow so the same choices don't need to be re-derived in every PR.

## Helper selection

Two helpers live in [`pkg/api/toolsets.go`](../../pkg/api/toolsets.go). Pick by
asking: *is the human-readable text the same string as `json.Marshal(structured)`?*

### `api.NewToolCallResultStructured(structured, err)`

Use when the text representation **is** the JSON serialization of the structured
value. The helper marshals `structured` and reuses the result as the text
content, so the two are guaranteed to stay aligned.

```go
return api.NewToolCallResultStructured(payload, nil), nil
```

Good fit for tools that emit raw typed data with no separate prose framing.

### `api.NewToolCallResultFull(text, structured, err)`

Use when the text representation **differs** from the JSON serialization — for
example, prose framing, a YAML serialization, a table, or any other
human-tailored format. Both Content and StructuredContent are passed
independently and you own keeping them consistent.

```go
return api.NewToolCallResultFull(humanReadable, payload, nil), nil
```

Good fit for tools whose existing text format is worth preserving for backwards
compatibility or LLM readability.

### `api.NewToolCallResult(text, err)`

Text-only. No structured content. Use for tools that have no structured payload
to emit (e.g., a YAML dump where the YAML *is* the user-facing output and there
is no separate typed view).

## Two recipes

### Recipe 1 — Kubernetes resource list tools

Tools that list or fetch Kubernetes resources should route through the existing
output layer. `output.Output.PrintObjStructured` (defined in
[`pkg/output/output.go`](../../pkg/output/output.go)) returns a `*PrintResult`
with both the text rendering (table or YAML) and a structured view extracted
from the underlying object.

The current list-tool callers — e.g. `namespacesList` in
[`pkg/toolsets/core/namespaces.go`](../../pkg/toolsets/core/namespaces.go) —
still use `PrintObj` + `NewToolCallResult` and emit no structured content.
When converting one of them, the pattern is:

```go
func namespacesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
    ret, err := kubernetes.NewCore(params).NamespacesList(
        params, api.ListOptions{AsTable: params.ListOutput.AsTable()},
    )
    if err != nil {
        return api.NewToolCallResult("", fmt.Errorf("failed to list namespaces: %w", err)), nil
    }
    printed, err := params.ListOutput.PrintObjStructured(ret)
    if err != nil {
        return api.NewToolCallResult("", fmt.Errorf("failed to format namespaces: %w", err)), nil
    }
    return api.NewToolCallResultFull(printed.Text, printed.Structured, nil), nil
}
```

For the table output format, `tableToStructured` produces a
`[]map[string]any` keyed by the server-sent column headers; for YAML it returns
the cleaned-up object or list of objects. No tool-specific extractor is
required.

> **Status:** the recipe is the intended pattern but has no in-tree adopter
> yet. The first converted list tool — tracked in #920 (MCP Apps), which
> retrofits several core tools with this pattern — will become the canonical
> reference. If that conversion uncovers a wrinkle (error-wrap style, params
> plumbing, a dedicated helper, etc.), please thread the change back into
> this section so the snippet doesn't quietly diverge from the live code.

### Recipe 2 — Bespoke / non-Kubernetes data

Tools whose payload is neither a Kubernetes object nor a list of them should
declare a small typed Go struct and pass it through `NewToolCallResultFull`
alongside whatever human-readable framing they already produce. The
`configuration_contexts_list` tool
([`pkg/toolsets/config/configuration.go`](../../pkg/toolsets/config/configuration.go))
is the reference example:

```go
type ContextInfo struct {
    Name    string `json:"name"`
    Server  string `json:"server"`
    Default bool   `json:"default"`
}

type ContextsListResult struct {
    DefaultContext string        `json:"defaultContext"`
    Contexts       []ContextInfo `json:"contexts"`
}
```

The struct, its field names, and its `json` tags are part of the tool's public
wire contract — see "Field naming" below.

## Ordering discipline

List-shaped structured emitters must produce a deterministic order. Two
clients consuming the same input shouldn't see entries shuffled. The default
convention is **lexicographic** sort on a stable key (the entry name, the
resource name, etc.):

```go
names := make([]string, 0, len(items))
for name := range items {
    names = append(names, name)
}
sort.Strings(names)
```

Use numeric ordering only where it is intrinsic to the data (e.g., port
numbers, indexes). When mixing numeric-looking strings (`cluster-2`,
`cluster-10`), lexicographic order yields `cluster-10 < cluster-2`; that is
the contract — tests should pin a middle index to catch any drift.

For Kubernetes resource lists, ordering is delegated to the output layer:
Tables preserve the API server's order; YAML preserves the list's `Items`
order. Tool authors are not expected to re-sort.

## Field naming

Top-level keys in the structured payload follow camelCase, matching the MCP
ecosystem's general JSON style. Keys should be descriptive nouns; reserve
abbreviations for cases that are already industry-standard (`url`, `uid`,
`id`).

`configuration_contexts_list` sets the precedent:

```json
{
  "defaultContext": "fake-context",
  "contexts": [
    { "name": "cluster-0", "server": "unknown", "default": false }
  ]
}
```

The container key is the resource's plural name (`contexts`); the marker for
the "default / current / focused" entry is a `default` bool on the entry
itself plus a `defaultContext` top-level scalar pointing at it by name, so
clients can resolve the default without re-scanning the list.

## `outputSchema` stance

The MCP spec lets a tool declare an `outputSchema` (JSON Schema) describing
its `structuredContent` shape. As of this writing the spec marks declaration
as SHOULD-level, not MUST.

This repo takes an **opportunistic** stance:

- **Declare `outputSchema`** when the tool has a stable, hand-authored
  typed struct (Recipe 2 above). The struct already pins the shape; the
  schema is a near-mechanical translation.
- **Skip `outputSchema`** for Kubernetes resource list tools (Recipe 1).
  Their structured shape depends on the queried GVK, the server's column
  metadata, and any future API extensions, so a static schema would either be
  too loose to be useful or too tight to remain accurate.

When in doubt, leave it off and add it later in a follow-up PR — declaring
nothing is preferable to declaring something inaccurate.

A declared `outputSchema` is set on the tool definition and surfaced to clients
through `tools/list`; it is **advertising, not enforcement**. The server does
not validate a handler's `structuredContent` against the declared schema, so the
tool author owns keeping the two aligned — see "Wire-contract discipline" below.

## Wire-contract discipline

Once a tool emits `structuredContent`, its JSON shape is public API and
clients may depend on the field names. To keep the contract visible to future
maintainers:

- Define the structured payload as a named, exported Go type whose `json:`
  tags are the wire keys. `ContextInfo` and `ContextsListResult` in
  `pkg/toolsets/config/configuration.go` are the reference.
- Add a doc comment on the type stating that it is part of the tool's wire
  contract, so reviewers know not to rename `json:` tags casually.
- Cover the structured shape in tests: assert the top-level keys, assert the
  type of nested entries, and pin at least one middle index for list-shaped
  payloads (to catch off-by-one or duplicate-skip bugs that a length check
  alone would miss).

## Empty / nil results

If a tool has no structured payload to emit on a given call — for example, a
list endpoint that returned zero items where a clearer human message is more
useful than an empty array — return text-only via `api.NewToolCallResult` and
leave `StructuredContent` nil. Do **not** pass an empty slice / map / struct;
a typed nil leaking into the `any` interface would create a non-nil interface
value, which is a footgun for downstream nil checks.
