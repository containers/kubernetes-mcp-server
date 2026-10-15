# Scaling

The memory caps bound each piece of work.
They do not bound the product.
Up to `max_in_flight` calls can each hold a request body, a decoded backend response, and a tool result at the same time.

A list is often smaller than this.
One backend body stops at `max_backend_response_bytes`, and the YAML plus structured JSON of that body rarely fill `max_result_bytes`.
The floor below is the overlap the caps still allow.

## Pod memory floor

```
floor = baseline + gc × max_in_flight × (amplify × max_backend_response_bytes + max_result_bytes + max_body_bytes)
```

| Symbol | Meaning | Planning value |
|--------|---------|----------------|
| `max_in_flight` | Concurrent `tools/call`, `prompts/get`, and `resources/read` | config, default 64 |
| `max_backend_response_bytes` | One Kubernetes, Kiali, NetObserv, Loki, Tempo, Prometheus, or Alertmanager body | config, default 4 MiB |
| `max_result_bytes` | One tool, prompt, or resource result | config, default 16 MiB |
| `max_body_bytes` | One inbound HTTP body. A stdio frame uses the same number | `http.max_body_bytes`, default 16 MiB |
| `amplify` | Decoded unstructured object versus the raw JSON. Small objects cost more than one big string | 10 |
| `gc` | Heap headroom before Go collects (`GOGC` 100) | 2 |
| `baseline` | Process and client caches | 128 MiB |

`0` disables that cap.
This is not a floor for any term set to `0`.

Defaults:

```
64 × (10 × 4 MiB + 16 MiB + 16 MiB) = 4.5 GiB live
floor ≈ 128 MiB + 2 × 4.5 GiB ≈ 9 GiB
```

A limit around 2 GiB can be exhausted by the result bytes alone.
Around 4 GiB is tight once the decoded objects exist.
About 9 GiB covers the overlap above, including a GC spike.

When `validation_enabled` is on, add one parsed OpenAPI document.
Clients of one cluster share that copy.
A refresh briefly holds a second copy.
Discovery documents stay under `max_backend_response_bytes`.
