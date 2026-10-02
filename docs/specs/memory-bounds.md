# Memory bounds

Status: **In progress** — `max_in_flight` is implemented.

Three reloadable top-level options cap each backend response, each tool result, and how many calls run at once. `0` disables that limit. They are TOML fields on [`Config`](../../pkg/config/config.go), registered with `opt(...).reload()`. Document them together in [`docs/configuration.md`](../configuration.md).

| TOML field | Default | What it limits |
|------------|---------|----------------|
| `max_backend_response_bytes` | `4194304` | Bytes read from one Kubernetes API, Kiali, or NetObserv response, and combined stdout+stderr retained by one `pods_exec` |
| `mcp_max_result_bytes` | `16777216` | Bytes of one tool, prompt, or resource result |
| `max_in_flight` | `64` | `tools/call`, `prompts/get`, and `resources/read` running in this process |

## Backend response

Read `max_backend_response_bytes` from the live config on each call. Allow exactly that many bytes. The next byte closes the body and returns a typed error. `0` leaves the read uncapped.

In [`newKubernetesFromLive`](../../pkg/kubernetes/kubernetes.go), wrap `resp.Body` on the existing round-tripper stack so client-go stops in `io.ReadAll` and does not decode. Skip `101` responses so exec and attach upgrades are left intact. Skip `/openapi/` responses.

Apply the same read limit in [`Kiali.ExecuteRequest`](../../pkg/kiali/kiali.go) and [`NetObserv.executeGetAbsolute`](../../pkg/netobserv/netobserv.go). Remove the existing hardcoded limits.

In [`PodsExec`](../../pkg/kubernetes/pods.go), share one counter across stdout and stderr. At the cap, cancel the exec context, keep the bytes already stored, and append a truncation notice. Any other stream error still drops the buffers.

## Result size

For a [`ToolCallResult`](../../pkg/api/toolsets.go), the measured size is `len(Content)` plus the JSON size of `StructuredContent` when that field is set. Prompt text and resource text or blob are the whole result for those handlers.

Set a remaining-byte budget on the handler context. Aggregators charge it as they append and stop when `Content` would exceed the cap:

- [`collectContainerLogs`](../../pkg/toolsets/tekton/taskrun.go) and [`getPipelineRunLogs`](../../pkg/toolsets/tekton/pipelinerun.go)
- [`fetchPipelineRunLogsForPrompt`](../../pkg/toolsets/tekton/pipeline_troubleshoot.go)
- virt-launcher log and event assembly in [`pkg/toolsets/kubevirt/vm/troubleshoot/tool.go`](../../pkg/toolsets/kubevirt/vm/troubleshoot/tool.go) and [`pkg/toolsets/kubevirt/vm_troubleshoot.go`](../../pkg/toolsets/kubevirt/vm_troubleshoot.go)

Return what was kept plus a truncation notice. The tool adapter then rejects the result when `len(Content)` plus the JSON size of `StructuredContent` is still over the cap.

## In-flight calls

Add a middleware beside [`rateLimitingMiddleware`](../../pkg/mcp/middleware.go). It acquires a process-wide slot before `tools/call`, `prompts/get`, and `resources/read`, and releases it when the handler returns. Read the limit on each acquire. Reject with a JSON-RPC error when a slot is not available.

## What the client sees

`max_backend_response_bytes` is one backend response. `mcp_max_result_bytes` measures the result sent to the client. For tools that is `Content` plus the JSON form of `StructuredContent`, including when those two fields are the same data twice. (The limit exists to protect the server, which must hold both fields in memory regardless of what the client uses or cares about.)

The client can see less than the backend cap because an over-cap list, get, log, Kiali call, or NetObserv call becomes an error, because exec returns one stream plus a notice, and because `limit` and `tail` still apply. The measured result can be larger than the backend cap because YAML can be larger than the JSON that was counted, because text and structured content are added together, and because several under-cap reads can be combined until the result cap. MCP JSON framing adds more on the wire. In-flight calls multiply memory by the number of slots.

## Tests

Cover an under-cap body, an exact-cap body, a chunked over-cap body, a skipped upgrade, a disabled cap, the same over-cap error for Kiali and NetObserv, exec truncation, an append that stops at the result budget, a tool result rejected because `Content` plus `StructuredContent` exceeds the cap, and global slot exhaustion.
