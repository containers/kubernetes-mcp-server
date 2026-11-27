# API discovery and schema inspection

Implemented in the generic `core` toolset.
See the [architecture and rationale](api-discovery-design.md).

Both tools use the selected client's authenticated discovery/OpenAPI endpoints.
They do not read resource instances or CRD objects, resolve catalogue instances into
enums, or add a cross-call schema cache.

## Discover types: `api_resources_list`

| Input | Meaning |
|---|---|
| `kind` | Optional case-insensitive exact kind, across groups |
| `query` | Optional case-insensitive literal substring over kind, plural resource name and API group only; not descriptions, aliases, regex or synonym matching |
| `group` | Optional exact API group; `""` explicitly selects core |
| `apiVersion` | Optional exact group/version, or `v1` for core |
| `continue` | Opaque continuation; repeat the same filters |

Filters combine with AND. `{}` discovers preferred API types, not instances.
Start unfiltered when the user's terminology is uncertain. Results are sorted by
apiVersion/kind/name and paged automatically within **16 KiB of application JSON**;
there is no caller-facing page-size argument.

Each `resources` entry preserves `apiVersion`, `kind`, plural `name` and
`namespaced`, adding a root-description summary: first sentence, at most 100 Unicode
characters including trailing `...` when text is omitted. Descriptions help choose
among returned types; they do not cause search matches. `descriptionUnavailable`
marks missing descriptions. Short-name and singular-name aliases are not returned.

The response includes `total`, `returned`, `remaining` and `complete`. A partial page
has `continue` and a recovery `message`. `warning` reports incomplete discovery or
description access. Missing descriptions do not affect name-search or inventory
coverage. An empty name search is a successful result, not a claim that no API
supports the user's intent; retry unfiltered if terminology is uncertain.

Continuations are bound to filters, denied-resource policy and an inventory/schema
fingerprint. Changed snapshots return an error directing the caller to restart.
An individual entry that cannot fit is an error, not a silently dropped type.

## Inspect a type: `api_resources_explain`

| Input | Meaning |
|---|---|
| `kind` | Required case-insensitive exact kind |
| `apiVersion` | Optional explicit group/version; otherwise resolve the unique kind's preferred version |
| `fieldPath` | Optional root-relative dotted property path, at most 512 bytes / 32 segments |

Group identity is carried by `apiVersion`, not by a second selector grammar.
Automatic kind resolution requires complete discovery. If discovery is incomplete,
explain fails with `discovery_incomplete`: supply an explicit `apiVersion` or retry
after discovery recovers. The error retains discovered, permitted exact matches in
`candidates`; these are choices, not an exhaustive list or an automatic selection.
If none were discovered, the message says so instead of claiming the kind does not
exist. Explicit-version requests do not require complete resource
discovery. Ambiguous kinds return canonical candidates rather than guessing. Explicit
non-preferred served versions are supported. Arrays and map values are traversed
implicitly: `spec.configuration.nodePools.name`, without `[]`.

The response includes canonical identity, `fieldPath`, `schema`, reachable local
reference definitions under `components.schemas`, labelled `ancestors` when narrowed,
`schemaHash`, `complete` and `omissions`. Hashes use the served index hash or a
digest of the document. Local references and `allOf` support field traversal;
alternative schemas are retained, not falsely merged into one branch.

Types, descriptions, required/default/enum, validation keywords, compositions and
Kubernetes extensions including CEL remain intact when they fit. Ancestors retain
validation context, including rules involving siblings. Such rules do not imply that
the sibling schemas are included: inspect their paths or the parent when needed.
OpenAPI does not describe every admission or controller behaviour.

### Full schema first, coarse compaction when necessary

The **24 KiB application-JSON budget includes metadata, ancestors and definitions**.
There is no default depth or field-count cut. Defensive work/depth guards fail
explicitly rather than masquerading as a complete schema. Field-path resolution
shares a cumulative reference/composition visit budget across property and implicit
container steps, so repeated references cannot bypass the work guard.

Oversized property branches are collapsed before retained descriptions are shortened.
There are at most three coarse simplifications, marked in-tree with
`x-mcp-omitted: true`. Each has a readable path, explanation and ready-to-use narrower
`expand` arguments. Unreachable definitions are removed; source schemas are not
modified. Referenced structures at an expanded boundary can expose deeper properties.
Selection is size-based, not domain-aware: `spec`, `status` and metadata have no semantic priority.
Description shortening, when needed, is one coarse operation with a narrower example
path. Oversized indivisible constraints or leaf descriptions fail explicitly.

Illustrative compacted response fragment:

```json
{
  "kind": "Deployment",
  "apiVersion": "apps/v1",
  "fieldPath": "",
  "schema": {
    "type": "object",
    "properties": {
      "spec": {
        "type": "object",
        "description": "Deployment specification.",
        "x-mcp-omitted": true
      }
    }
  },
  "complete": false,
  "omissions": [{
    "path": "spec",
    "message": "Branch details are omitted to fit 24 KiB. Expand if needed.",
    "expand": {"kind": "Deployment", "apiVersion": "apps/v1", "fieldPath": "spec"}
  }]
}
```

Expand only task-relevant omissions. `complete` applies to the selected view and
returned context, not the whole resource after narrowing. A compacted view is not
a complete validation schema.

## Errors, authorization and compatibility

Full and usable compacted responses have MCP `isError: false`; compaction sets
application `complete: false`. Actual failures have `isError: true` and a structured
`error` with code, message and applicable candidate arguments or child fields.
The MCP text block carries the same JSON, including recovery guidance. Typo
suggestions are bounded and never auto-applied.

Missing required arguments, wrong argument types and an explicitly empty `kind`
return `invalid_argument`, not `discovery_failed`. Field-path syntax/size failures
retain `invalid_field_path`; transport and unclassified backend failures retain
`discovery_failed`.

`denied_resources` filters inventory/candidates and blocks explain before fetching
the schema, including case-insensitive kind requests. Read-only annotations do not
replace Kubernetes authorization. The existing authenticated client-go v3 reader
preserves workspace URL prefixes and schema-hash queries. A group/version is fetched
once per call, without cross-call reuse.

The server-wide `max_result_bytes` remains an independent outer bound and counts
text plus structured output; these application budgets do not override it.

Existing exact-kind list calls remain valid and retain the four identity fields.
Clients must tolerate description/coverage fields and follow continuation for exhaustive
results. Inventory now also makes OpenAPI requests for descriptions. CRUD tools still
operate on instances; discovery/explain do not change their contracts.
