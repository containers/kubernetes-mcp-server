# Memory bounds

Status: **Implemented**.

Three reloadable top-level options cap each backend response, each tool result, and how many calls run at once. `0` disables that limit. They are TOML fields on [`Config`](../../pkg/config/config.go), registered with `opt(...).reload()`. Document them together in [`docs/configuration.md`](../configuration.md).

| TOML field | Default | What it limits |
|------------|---------|----------------|
| `max_backend_response_bytes` | `4194304` | Bytes read from one Kubernetes API, Kiali, or NetObserv response, and combined stdout+stderr retained by one `pods_exec` |
| `mcp_max_result_bytes` | `16777216` | Bytes of one tool, prompt, or resource result |
| `max_in_flight` | `64` | `tools/call`, `prompts/get`, and `resources/read` running in this process |

## Backend response

Read `max_backend_response_bytes` from the live config on each call. Allow exactly that many bytes. The next byte closes the body and returns a typed error. `0` leaves the read uncapped.

In [`newKubernetesFromLive`](../../pkg/kubernetes/kubernetes.go), wrap `resp.Body` on the existing round-tripper stack so client-go stops in `io.ReadAll` and does not decode. Skip `101` responses so exec and attach upgrades are left intact. Skip `/openapi/` responses. That document is the apiserver's aggregated spec and has to be parsed whole. Clients of one manager share one parsed model ([`openAPISchemaCache`](../../pkg/kubernetes/schema_validator.go)). Concurrent validations wait on one fetch. A failed fetch is not kept.

Apply the same read limit in [`Kiali.ExecuteRequest`](../../pkg/kiali/kiali.go) and [`NetObserv.executeGetAbsolute`](../../pkg/netobserv/netobserv.go). Remove the existing hardcoded limits. Kiali and NetObserv JSON calls return the over-cap error. NetObserv flow export keeps the bytes already read and appends a truncation notice at that same cap.

In [`PodsExec`](../../pkg/kubernetes/pods.go), share one counter across stdout and stderr. At the cap, cancel the exec context, keep the bytes already stored, and append a truncation notice. The exec library may report that cancel as `context.Canceled` or as some other stream error; either way the capped bytes are kept. A stream error before the cap still drops the buffers.

## Result size

For a [`ToolCallResult`](../../pkg/api/toolsets.go), the measured size is `len(Content)` plus the JSON size of `StructuredContent` when that field is set. When `ContentBlocks` is set, those block bytes replace `Content`, including image data. A tool error is sent as its error text alone, so that text is the measured size. Prompt text and resource text or blob are the whole result for those handlers.

Handlers read the limit and write retained text through [`BuilderWithBudget`](../../pkg/api/resultbudget.go). A write that does not fit is cut, and a truncation notice is appended. The notice may extend the result past the cap by its own length. The adapter allows that excess and rejects a result that is still larger.

## Retention

The purpose of these limits is to keep this process from retaining unbounded or very large amounts of memory. The three options are the controls for that. They are not a list of the only paths that matter.

`max_backend_response_bytes` bounds one backend read. Code that keeps more than that read, or keeps a growing structure derived from several reads, retains those bytes through `BuilderWithBudget` and does not start another fetch after `Truncated()` is set. `NewBuilderWithBudget(0)` is uncapped and is not used to assemble a result. A nested collector takes a [`Scratch`](../../pkg/api/resultbudget.go) capped at the bytes the parent can still accept. The post-hoc result check rejects one blob that is still too big. It does not bound memory a handler has already accumulated.

Stopping the next fetch leaves the prefix already retained and the truncation notice. The call is not failed after those bytes have been kept.

## In-flight calls

Add a middleware beside [`rateLimitingMiddleware`](../../pkg/mcp/middleware.go). It acquires a process-wide slot before `tools/call`, `prompts/get`, and `resources/read`, and releases it when the handler returns. Read the limit on each acquire. Reject with a JSON-RPC error when a slot is not available.

## What the client sees

`max_backend_response_bytes` is one backend response. `mcp_max_result_bytes` measures the result sent to the client. For tools that is `Content` plus the JSON form of `StructuredContent`, or the `ContentBlocks` bytes when those are set, including when text and structured content are the same data twice. (The limit exists to protect the server, which must hold both fields in memory regardless of what the client uses or cares about.)

The client can see less than the backend cap because an over-cap list, get, log, Kiali call, or NetObserv JSON call becomes an error, because NetObserv flow export and exec return one prefix plus a notice, and because `limit` and `tail` still apply. The measured result can be larger than the backend cap because YAML can be larger than the JSON that was counted, because text and structured content are added together, and because several under-cap reads can be combined until the result cap. A collector stops the next fetch once that cap is reached. MCP JSON framing adds more on the wire. In-flight calls multiply memory by the number of slots.

## Tests

Cover an under-cap body, an exact-cap body, a chunked over-cap body, a skipped upgrade, a disabled cap, the same over-cap error for Kiali and NetObserv JSON, a NetObserv flow export that keeps a prefix, exec truncation, an append that stops at the result budget, a nested scratch that stops a second fetch, pipeline event listing that does not request the next target once the builder is full, health-check events that retain 20 lines, a tool result rejected because `Content` plus `StructuredContent` exceeds the cap, and global slot exhaustion.
