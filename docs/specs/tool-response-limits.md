# Tool Response Limits

Status: **Planned**

List and log tools read the Kubernetes API response into memory and then copy it into the MCP tool result. An unbounded `resources_list`, `events_list`, or `pods_log` can exhaust process memory. This spec bounds that path with reloadable config ceilings, one Kubernetes list page per tool call, and a cap on the text returned to the client.

## Current behavior

- [`ResourcesList`](../../pkg/kubernetes/resources.go) forwards `api.ListOptions` with no `Limit` or `Continue`. `api.ListOptions` already embeds `metav1.ListOptions`.
- [`events_list`](../../pkg/toolsets/core/events.go) projects events to maps and drops list metadata, so a continue token cannot be returned.
- [`PodsLog`](../../pkg/kubernetes/pods.go) sets `TailLines` (default 100; a non-positive tail is rewritten to 100) and buffers the body with `Do` / `Raw`. It does not set `LimitBytes`.
- [`NodesLog`](../../pkg/kubernetes/nodes.go) omits `tailLines` when the argument is `<= 0`, which returns the whole log file, and buffers the body with `Do` / `Raw`.
- A tool call selects one cluster in [`pkg/mcp/tools_gosdk.go`](../../pkg/mcp/tools_gosdk.go) via `GetDerivedKubernetes`. There is no fan-out across targets. Limits apply to that single call. Adding a target does not change how much any other target can return.
- The MCP result is a string on `api.ToolCallResult`. The server does not own a streaming sink toward the client.

## Decisions

- Keep `ToolCallResult` as a string. Bound the Kubernetes read and the string built from it.
- Defaults are bounded. An unset config file must not leave list or log calls unbounded.
- The model may request a smaller page or a shorter tail. The server clamps anything above the configured ceiling and reports the effective value.
- One tool call returns one apiserver list page. The continue value is the apiserver's opaque token. The server stores no page cursor.
- `remainingItemCount` is included only when the apiserver sets it, so the model can estimate how many further calls remain.
- A formatted page that exceeds the byte cap does not return a new continue token. That token would skip the rest of the page. The error tells the model to retry with a smaller `limit` and the same `continue` it sent.
- Kubernetes object results (list pages, `resources_get`, events YAML, structured content) are documents. They are returned whole or not at all. A single object that still exceeds the cap is an error with no partial body, same as an oversized Kiali or NetObserv JSON response.
- Log truncation is detected when the body length equals `LimitBytes`. The API server does not flag truncated logs.
- Per-kind page sizes (Pods versus ConfigMaps) are deferred. One global page size is enough for this change.
- `http.max_body_bytes` limits the inbound MCP HTTP request body. It does not limit tool results. See the discussion that introduced this spec; do not fold that knob into these options.
- Every tool result this change caps is text: YAML, tables, JSON, logs, or CSV. `ToolCallResult` has `Content string` and optional JSON `StructuredContent`. It has no image or blob field. Binary payloads use a separate cap, added with the first tool or resource that returns one. They do not count against `max_tool_response_bytes`.

## Config

Add three options on [`Config`](../../pkg/config/config.go), using the same `opt(...).reload().validate(...).desc(...)` pattern as `http.max_body_bytes`. Handlers read `params.Config` on each call, so a reload applies without a restart.

These are defaults. A tool with no override uses them. A set override replaces the default for that tool and that field, including when the override is larger.

| TOML key | Type | Default | Role |
|----------|------|---------|------|
| `max_list_page_size` | int64 | 200 | Kubernetes `Limit` when the model omits `limit`, and the ceiling when it sets one |
| `max_tool_response_bytes` | int64 | 1 MiB (1048576) | Maximum text bytes placed in the tool result |
| `max_log_tail_lines` | int64 | 1000 | Ceiling on log tail. Tool-level default tail stays 100 |

Per-tool overrides live in one map, keyed by the tool name the server registers (`resources_list`, `kiali_get_logs`). The field names match the globals. This map is not `tool_overrides` (that map replaces descriptions) and not `toolset_configs` (those blocks are connection settings, core has none, and Kiali's parser requires `url`).

```toml
max_tool_response_bytes = 1048576
max_list_page_size = 200
max_log_tail_lines = 1000

[response_limits.resources_list]
max_tool_response_bytes = 262144
max_list_page_size = 50

[response_limits.pods_log]
max_tool_response_bytes = 2097152
max_log_tail_lines = 500

[response_limits.netobserv_list_flows]
max_tool_response_bytes = 4194304

[response_limits.netobserv_get_flow_metrics]
max_tool_response_bytes = 4194304

[response_limits.netobserv_export_flows]
max_tool_response_bytes = 2097152

[response_limits.kiali_get_mesh_status]
max_tool_response_bytes = 524288
```

Rules:

- An omitted field inherits the global. A present value is that tool's effective limit and may be above or below the global.
- `0` and negative values fail validation, on the globals and on every override. Absence is what inherits.
- An unknown tool name fails the load. The name is looked up in the toolset registry ([`toolsets.Toolsets`](../../pkg/toolsets/toolsets.go)), which `init` fills before [`Config.Validate`](../../pkg/config/validate.go) runs. That check does not call [`isToolApplicable`](../../pkg/mcp/mcp.go). Target-compatibility filters, `toolsets`, `enabled_tools`, `disabled_tools`, `read_only`, and `disable_destructive` decide exposure later, in `collectApplicableTools`, and again when cluster discovery changes. A `response_limits` entry for a tool those filters hide is unused, not a load error. A downstream rename (Kiali's toolset name override) changes the registered name, so the key must match that binary.
- Any of the three fields may be set on any registered tool. A field the tool's read path does not consult has no effect and is not a load error. Unknown keys inside the table still fail the load, because the table is a fixed struct. There is no per-tool allowlist and no config declaration for a tool to add.
- The byte cap is applied in the MCP adapter ([`pkg/mcp/tools_gosdk.go`](../../pkg/mcp/tools_gosdk.go)), which already has `tool.Tool.Name` in the handler closure. Every current and future tool gets that cap with no further declaration. Page size is read in the shared list helper, and tail lines in the shared log helpers, using that same name. A new tool that lists or logs through those helpers picks up an override automatically. A new read path that does not use them calls the same effective-limit lookup with its tool name; it does not add a config field.
- Shared readers take the calling tool's effective values. Kiali `ExecuteRequest`, NetObserv `ExecuteGet`, and Tekton `collectContainerLogs` do not grow a second key. Restoring Kiali's 512 KiB means one table per `kiali_*` tool. Restoring NetObserv's 4 MiB JSON means `netobserv_list_flows` and `netobserv_get_flow_metrics`. The export tool is its own table.

The model-facing ceiling for a call is that tool's effective value. The server still clamps a requested `limit` or tail to it.

## Types and call path

One new `Option` holds every per-tool override. Tool names are map keys, not fields on `Config`.

```go
type ResponseLimits struct {
	MaxToolResponseBytes *int64 `toml:"max_tool_response_bytes,omitempty"`
	MaxListPageSize      *int64 `toml:"max_list_page_size,omitempty"`
	MaxLogTailLines      *int64 `toml:"max_log_tail_lines,omitempty"`
}

// On Config, beside the three Option[int64] globals:
ResponseLimits Option[map[string]ResponseLimits]
```

Install it with `opt("response_limits", map[string]ResponseLimits(nil)).reload().validate(validateResponseLimits).desc(...)`. Pointers distinguish an omitted field (inherit) from a present value. `rejectContainerUnknown` already rejects unknown fields inside `map[string]Struct`, the same way as `tool_overrides`.

```go
func validateResponseLimits(m map[string]ResponseLimits) error
```

Walk `toolsets.Toolsets()` and `GetTools` with a stub `FilteringProvider` whose `IsTargetCompatibilityToolFiltersEnabled` returns false, so discovery does not query the cluster. A key missing from that name set fails. A set pointer `<= 0` fails. A nil pointer is skipped. Do not call `isToolApplicable`.

```go
type EffectiveResponseLimits struct {
	MaxToolResponseBytes int64
	MaxListPageSize      int64
	MaxLogTailLines      int64
}

func (c *Config) EffectiveResponseLimits(tool string) EffectiveResponseLimits
```

Start from the three globals. Overwrite a field only when that tool's entry has a non-nil pointer.

`ToolHandlerParams` gains `ToolName string`. The MCP adapter sets it from `tool.Tool.Name`, which the handler closure already has. After the handler returns, the adapter loads `EffectiveResponseLimits(tool.Tool.Name)` and, if `Content` is still longer than `MaxToolResponseBytes`, returns an error and drops `Content` and `StructuredContent`.

Callers pass numbers into existing reads. They do not add config fields:

```go
func (c *Core) PodsLog(ctx context.Context, namespace, name, container string, previous bool, tail, limitBytes int64) (string, error)

func (k *Kiali) ExecuteRequest(ctx context.Context, endpoint string, arguments map[string]any, maxBytes int64) (string, error)
```

List handlers, `NodesLog`, NetObserv, and `collectContainerLogs` take the same effective tail or byte value from `params.Config.EffectiveResponseLimits(params.ToolName)`. A new tool that returns text is capped in the adapter. A new list or log path calls that method with its tool name.

## Documentation

[`docs/configuration.md`](../configuration.md) documents the three globals and `response_limits` as one map. The key is a registered tool name. The page does not list a block per tool.

`DocumentedOptions()` walks `Option` fields, so `response_limits` is one path. Its description must say the key is a registered tool name and the value has the three fields, with an omitted field inheriting the global. A generator that renders `DocumentedOptions()` must not expand map keys into per-tool rows. `make update-readme-tools` documents tool parameters (`limit`, `continue`, `tail`). It does not emit `response_limits.<tool>` rows.

## Lists

Add optional `limit` (integer, minimum 1) and `continue` (string) to the tools that return a list to the model:

- `resources_list` ([`pkg/toolsets/core/resources.go`](../../pkg/toolsets/core/resources.go))
- `events_list` ([`pkg/toolsets/core/events.go`](../../pkg/toolsets/core/events.go))
- `pods_list`, `pods_list_in_namespace`, `namespaces_list`, `projects_list` (same `ResourcesList` path)

Schema maximums stay unset. The ceiling is config and can change on reload, so the handler clamps.

Clamp `limit` into `[1, effective max_list_page_size]`. An omitted `limit` becomes that effective value. Pass `Limit` and `Continue` through on `api.ListOptions`.

`EventsList` returns `metadata.continue` and `remainingItemCount` (when set) along with the projected maps. Today it returns only `[]map[string]any`.

Append this trailer for table and YAML output when the call was clamped or another page exists:

```text
# limit=<effective> continue=<token> remainingItemCount=<n>
```

Omit `continue` when the apiserver sends none. Omit `remainingItemCount` when the apiserver leaves it unset.

Tool descriptions state that the next call resends the same selectors plus this `continue`, and that `limit` cannot exceed the server ceiling.

A list page is one document, in table output and in YAML. Structured content is the same document in another encoding. Slicing it on a byte boundary, or dropping trailing items, produces a result that parses as a shorter complete page. The apiserver continue token cannot name the dropped suffix, so the omitted objects are unreachable from that result.

When the formatted text or the structured content exceeds the tool's effective `max_tool_response_bytes`:

1. Return an error. Omit the page body and structured content. Do not attach a new continue token. The error tells the model to retry with a smaller `limit` and the same `continue` it sent on this call.
2. When `limit` is already 1 and that single object still exceeds the cap, return the same kind of error. Name the object when its name is known, and state that it exceeds `max_tool_response_bytes`. There is no partial YAML, table row, or structured object.

`resources_get` is the limit-1 case with no `continue`: the same error, no partial object.

Internal `ResourcesList` callers, including health check, pass a `Limit` so they do not materialize an unbounded list.

A parameter that fetches multiple pages inside one call is out of scope. It would multiply the memory this change is bounding.

## Logs

`PodsLog`:

- Tail default stays 100. A non-positive tail stays 100.
- Clamp a requested tail to the tool's effective `max_log_tail_lines`.
- Set `PodLogOptions.LimitBytes` to the tool's effective `max_tool_response_bytes`.
- When the body length equals that cap, append a truncation warning.

`NodesLog`:

- A missing or non-positive `tailLines` uses the default of 100, under the same ceiling. It does not return the entire file.
- The kubelet log proxy has no `limitBytes` equivalent. When `Raw()` exceeds the tool's effective `max_tool_response_bytes`, truncate the string and append the same warning.

## Backstop

The MCP adapter in [`pkg/mcp/tools_gosdk.go`](../../pkg/mcp/tools_gosdk.go) does not slice `Content`. A cut string is how a list or CR would leak out as a broken document.

Document handlers (`resources_list`, `resources_get`, `events_list`, and the other list tools) already return an error when the result does not fit. Stream handlers (logs, NetObserv CSV, Tekton) truncate to the cap before they return, and the string they return fits.

If `Content` is still longer than the tool's effective `max_tool_response_bytes`, the adapter returns an error and drops `Content` and `StructuredContent`. That is the safety net for a handler that skipped its own check.

## One byte cap for every toolset

Several toolsets already stop an unbounded upstream read. The constants differ; the job does not. None of them are configured by the operator, so there is no setting to deprecate. Replacing the constants with `max_tool_response_bytes` is a behavior change and belongs in the release notes.

| Call site | Constant today | When the body is larger |
|-----------|----------------|-------------------------|
| Kiali `ExecuteRequest` ([`pkg/kiali/kiali.go`](../../pkg/kiali/kiali.go)) | 512 KiB | Error, body discarded. The comment says "truncated"; the code returns an error. |
| Tekton `collectContainerLogs` ([`pkg/toolsets/tekton/taskrun.go`](../../pkg/toolsets/tekton/taskrun.go)) | 1 MiB per container | Silent truncate, then concatenate every step and sidecar. |
| NetObserv JSON `ExecuteGet` ([`pkg/netobserv/netobserv.go`](../../pkg/netobserv/netobserv.go)) | 4 MiB | Error, body discarded. |
| NetObserv CSV export ([`pkg/toolsets/netobserv/tools/defaults.go`](../../pkg/toolsets/netobserv/tools/defaults.go)) | 2 MiB | Truncate and append a marker. |

With the default of 1 MiB, Kiali allows a larger body than today (512 KiB → 1 MiB). NetObserv JSON (4 MiB → 1 MiB) and CSV export (2 MiB → 1 MiB) allow a smaller body. Tekton's per-container ceiling matches today at the default. The concatenated Tekton result is newly capped at 1 MiB; today N containers can return N MiB.

Two shapes stay, because the payload does:

- **Documents** error and discard when the result exceeds the tool's effective `max_tool_response_bytes`. This includes Kiali, NetObserv JSON, Kubernetes list pages, `resources_get`, and events YAML. A cut JSON or YAML body is not a usable tool result, and a shortened list would look like a complete page. The Kiali and NetObserv read limit becomes that effective value instead of the private constant.
- **Text streams** (pod logs, node logs, NetObserv CSV) still truncate and append a warning. A prefix of a log or a CSV export is still the data the model asked for. Tekton keeps a per-container read stop, because one call fans out across steps and sidecars and the MCP-edge check runs only after those reads. That stop uses the tool's effective `max_tool_response_bytes`, the truncate is reported, and collection stops once the aggregated text reaches the same value.

The shipped defaults are the globals only. Implementation adds [`docs/examples/zz-response-limits-previous.toml`](../examples/zz-response-limits-previous.toml) with the contents below and does not load it. A consumer who wants the previous Kiali and NetObserv constants copies that file into `--config-dir`. [`docs/configuration-changes.md`](../configuration-changes.md) carries the same explanation.

```toml
# Previous Kiali and NetObserv response-size constants.
# Not loaded automatically. Copy into the directory passed to --config-dir.
# Drop-ins merge in lexical order; the zz- prefix sorts after other files
# that set the same keys. A downstream rename of the kiali or netobserv
# toolset must rename these keys to the names that binary registers.

[response_limits.kiali_get_logs]
max_tool_response_bytes = 524288

[response_limits.kiali_get_mesh_status]
max_tool_response_bytes = 524288

[response_limits.kiali_get_mesh_traffic_graph]
max_tool_response_bytes = 524288

[response_limits.kiali_get_metrics]
max_tool_response_bytes = 524288

[response_limits.kiali_get_pod_performance]
max_tool_response_bytes = 524288

[response_limits.kiali_get_resource_details]
max_tool_response_bytes = 524288

[response_limits.kiali_get_trace_details]
max_tool_response_bytes = 524288

[response_limits.kiali_list_mesh_clusters]
max_tool_response_bytes = 524288

[response_limits.kiali_list_traces]
max_tool_response_bytes = 524288

[response_limits.kiali_manage_istio_config]
max_tool_response_bytes = 524288

[response_limits.kiali_manage_istio_config_read]
max_tool_response_bytes = 524288

[response_limits.netobserv_list_flows]
max_tool_response_bytes = 4194304

[response_limits.netobserv_get_flow_metrics]
max_tool_response_bytes = 4194304

[response_limits.netobserv_export_flows]
max_tool_response_bytes = 2097152
```

Tekton's per-container cap was already 1 MiB, so the file leaves `tekton_taskrun_logs` and `tekton_pipelinerun_logs` unset. The file does not lift the new bounds on core list and log tools, and it does not restore Tekton's old behavior of concatenating every container with no aggregate cap: the per-container stop and the aggregate cap are the same effective value.

Kiali's own tail maximum (the tool description cites 200 lines) is an upstream Kiali constraint. It stays.

Health-check prompt text (150-character messages, 20 events) is presentation after the list. It is not a response-size policy. The unbounded `Events().List` behind it is covered by the internal-caller `Limit` rule above.

OIDC discovery in [`pkg/http/wellknown.go`](../../pkg/http/wellknown.go) caps a metadata document at 1 MiB. That is not a tool result.

## Binary results

No tool in this repository returns a non-text body. KubeVirt cloud-init handling base64-decodes secret text into the troubleshoot report. MCP resource handlers can return `ResourceContent.Blob` ([`pkg/api/toolsets.go`](../../pkg/api/toolsets.go)); the `image/png` case exists only in [`pkg/mcp/resources_gosdk_test.go`](../../pkg/mcp/resources_gosdk_test.go). No shipped resource uses it.

A console screenshot is a binary document. A cut PNG does not display, so the policy is whole or error, as with Kubernetes objects. The ceiling is a different number: a useful screenshot is often larger than the text default, and raising `max_tool_response_bytes` to fit one would loosen the list and log guard. Count the raw image bytes, before any base64 expansion into a text block.

When the first binary tool or resource lands, add a separate reloadable option for that raw size. Do not put the image in `Content` to reuse the text cap. Until that option exists, this spec does not add it.

## Tests

- Omitted `limit` uses `max_list_page_size`.
- `limit` above the ceiling is clamped, and the trailer reports the effective value.
- `continue` is sent to the apiserver and returned in the trailer.
- An oversized page returns an error, no page body, and no new continue token.
- A single oversized object, including `resources_get`, returns an error and no partial document.
- Log tail above `max_log_tail_lines` is clamped.
- A pod log body that hits `LimitBytes` includes the truncation warning.
- `nodes_log` with tail `0` returns at most the default tail, and a body over the byte cap is truncated.
- Kiali and NetObserv JSON reads use the calling tool's effective `max_tool_response_bytes` and still return an error when the body is larger, with no partial document.
- A `response_limits` entry replaces only the fields it sets. A tool name that is not in the registered catalog, or a value `<= 0`, fails the load. A known field on a tool whose read path does not consult it loads and has no effect. An override for a tool that target-compatibility filtering, toolset selection, or the read-only and destructive gates currently hide still loads. An override larger than the global is accepted.
- NetObserv CSV export truncates at `max_tool_response_bytes` and keeps its truncation marker.
- A Tekton task with several containers stops reading each container at `max_tool_response_bytes`, reports truncation, and stops the aggregate at the same cap.

After the schema edits, refresh generated tool docs with `make update-readme-tools`. That generator documents tool parameters only. It does not emit `response_limits.<tool>` rows.
