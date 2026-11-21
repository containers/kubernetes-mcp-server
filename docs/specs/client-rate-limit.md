# HTTP client rate limit

Status: **Proposed**.

Limit how fast one client can call the HTTP MCP endpoint.
The limit is per process, per client identity, and applies only in HTTP mode.
Stdio does not install this middleware.

The existing per-session limiter stays as it is.
`http.rate_limit_rps` and `http.rate_limit_burst` in [`pkg/mcp/middleware.go`](../../pkg/mcp/middleware.go) key on the MCP session id, skip an empty id, and drop their buckets when the configured rate or burst changes.
Stateless Streamable HTTP leaves the session id empty, and a new session id also starts a fresh bucket.
This spec does not change that limiter.

## Algorithm

Use a token bucket from `golang.org/x/time/rate`, the same type the session limiter uses.
Each bucket has a refill rate (tokens per second) and a burst (maximum tokens).
One request to `/mcp` takes one token.
The bucket refills continuously.
A new bucket starts full.

Memory is a few words per client.

## Client key

HTTP connections and MCP session ids are not the client.
After authorization succeeds, [`AuthorizationMiddleware`](../../pkg/http/authorization.go) stores a key on the request context.
The rate-limit middleware reads that key.
Build the key in this order:

1. **Verified JWT with a non-empty `sub`.** Key is `sub:` + `url.QueryEscape(iss)` + `|` + `url.QueryEscape(sub)`.
   `iss` and `sub` come from the `jwt.Claims` already parsed for validation.
   Both are required so two issuers with the same subject stay distinct.
   Ignore `preferred_username`.
2. **Verified JWT with an empty `sub`, or opaque passthrough.** Key is `tok:` + hex SHA-256 of the raw bearer token.
   Passthrough is `skip_jwt_verification` with an empty `authorization_url`, including OpenShift sha256 tokens.
   Use the hash even when the opaque token happens to be a JWT: claims are not verified on that path.
   The same credential shares a bucket.
   A refreshed token is a new key.
   Store the hash, never the raw token.
3. **No credential.** `require_oauth` is false, so the auth middleware does not set a key.
   The rate-limit middleware uses `ip:` + [`getClientIP`](../../pkg/http/middleware.go).
   That helper uses `X-Forwarded-For` or `X-Real-IP` only when `trust_proxy_headers` is true, and otherwise uses `RemoteAddr`.

A 401 returns from the auth middleware and does not reach the limiter.
Failed authentication is not counted.

One shared service account, or one shared source address, is one client.
The budget is per process.
Several replicas each allow a full budget.

## Configuration

Add two reloadable fields on [`HTTPConfig`](../../pkg/config/config.go), registered with `opt(...).reload()`:

| TOML field | Type | Default | Meaning |
|------------|------|---------|---------|
| `http.client_rate_limit_rps` | float | `16` | Tokens added per second. `0` disables the limiter. |
| `http.client_rate_limit_burst` | integer | `64` | Bucket size. `0` selects this default of 64. |

Reject a negative value:

- `client_rate_limit_rps must not be negative`
- `client_rate_limit_burst must not be negative`

Define the defaults from `DefaultMaxInFlight` (64) in code:

- burst default = `DefaultMaxInFlight`
- rps default = `DefaultMaxInFlight / 4`

Read the live `max_in_flight` option only for the in-flight middleware.
Changing `max_in_flight` does not change these two fields.

The burst default matches the process-wide cap on `tools/call`, `prompts/get`, and `resources/read`.
One client can fan out once up to that cap.
The rate default is a quarter of that cap per second, so one identity cannot hold the whole process after the burst is spent.
`ping` and `tools/list` do not occupy an in-flight slot, and this limiter counts them anyway.

When `client_rate_limit_rps` is greater than 0 and `client_rate_limit_burst` is 0, the middleware uses burst 64.
There is no configured burst of zero.

## Request path

Install the middleware in [`Serve`](../../pkg/http/http.go), after authorization and before [`MaxBodyMiddleware`](../../pkg/http/http.go), so a rejected request does not read the body:

```go
wrappedMux := chain(mux,
    RequestMiddleware(cfgState),
    AuthorizationMiddleware(cfgState, oauthState),
    ClientRateLimitMiddleware(ctx, cfgState),
    MaxBodyMiddleware(cfgState),
)
```

Create `Serve`'s cancellable context before this chain.
The reaper and the HTTP server's `BaseContext` both use that context, so signal shutdown and caller cancellation stop the reaper.

On each request:

1. If `r.URL.Path` is not `/mcp`, call the next handler.
   `/healthz`, `/metrics`, `/stats`, and `/.well-known/` are not counted.
   Any method on `/mcp` counts as one token, including GET and DELETE.
2. Read `client_rate_limit_rps` and `client_rate_limit_burst` from the live config.
   Resolve burst 0 to 64.
3. If the rate is 0, do not take a token.
   When an entry for this key already exists, refresh its `lastSeen` and leave its tokens unchanged.
   Do not create an entry while the limiter is off.
4. Otherwise take or create the bucket under the map lock, then reserve outside that lock.

The HTTP metrics middleware already wraps this chain, so a 429 is recorded by the existing request counter.
Add no new metric.

### Rejection

Reserve one token.
If the reservation delay is greater than zero, cancel the reservation so the token is not consumed, and respond with HTTP 429.
Set `Retry-After` to the delay rounded up to at least 1 second, then write the status with `http.Error` and the body `rate limit exceeded`.
Set the header before `http.Error`, which writes the status line.

If the reservation is not allowed because the burst cannot cover one token, respond with the same 429 and `Retry-After: 1`.

Log the rejection at V(1) with the request method, path, and key.
The key is an issuer and subject, a token hash, or an IP.
Do not log the `Authorization` header or the raw token.

### Reload

Keep the bucket across a config change.
Store the rate and burst last applied to that `*rate.Limiter`.
On the next request that uses the entry, if either value differs, call `SetLimit` and `SetBurst` before reserving.

A limiter is created full only for a key that has no entry.
`SetBurst` stores the new burst and leaves the current token count in place.
The reservation then advances time, which caps tokens at the new burst.
Credit above the new burst is dropped.
The request is allowed when the capped count still covers one token.
An empty bucket stays empty and refills at the new rate.

## Bounding the map

The map is `map[string]*entry`, guarded by a `sync.Mutex`.
Each entry holds the `*rate.Limiter`, the rate and burst last applied, and `lastSeen`.
Refresh `lastSeen` when the request consults an existing entry, including when the limiter is disabled.
Do not scan the map on every request.

Stop growth in two ways:

- **Idle reaping.** A goroutine wakes every 5 minutes and deletes entries whose `lastSeen` is older than 10 minutes.
  It exits when the `Serve` context is cancelled.
- **Hard cap of 8192 keys.** On a miss at the cap, sweep idle entries once while holding the lock.
  If a slot opened, insert the new key.
  If the map is still full, do not insert.
  Respond with HTTP 429, body `rate limit exceeded`, and `Retry-After: 600`.
  Log at V(1) that the tracker is full.

A config change does not remove entries.

## Documentation

When implementing, document the two fields for operators:

- [`docs/configuration.md`](../configuration.md), in the HTTP server security section: the table, the note that this limit is per client and per process, and an example under `[http]`.
  State that a shared service account or a shared client address shares one bucket, and that each replica has its own buckets.
- [`docs/configuration-changes.md`](../configuration-changes.md): both fields are reloadable.
- [`docs/specs/config.md`](config.md): add rows next to `http.rate_limit_rps` and `http.rate_limit_burst`.

## Tests

Follow [`docs/dev/testing.md`](../dev/testing.md).
Use a testify suite in `package http`.
Cover observable HTTP behavior:

- A request under the burst returns 200 from a downstream handler.
- The next request past the burst returns 429 with `Retry-After` and does not consume a token (a later request still waits out the same debt).
- Two opaque tokens have independent buckets.
- With `require_oauth` false, the source address is the key.
  `X-Forwarded-For` changes the key only when `trust_proxy_headers` is true.
- `client_rate_limit_rps = 0` allows requests that would otherwise be over budget.
- Turning the limit back on applies the new rate and burst to the existing bucket.
  A client that already emptied the bucket stays empty.
  It does not receive a full new burst.
- Lowering the burst caps remaining credit at the new burst.
  The client can spend that many tokens and no more.
- `/healthz` is not counted.
- A negative rps or burst fails config validation.

Test the 8192 cap and idle deletion in `package http` with a short idle and a small cap on the tracker.
Waiting for the production 10 minute idle is not required.
Cancel the reaper context in every test that starts it.
