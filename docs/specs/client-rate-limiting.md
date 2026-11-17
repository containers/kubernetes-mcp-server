# Client rate limiting

Status: **Implemented** for the request rate ([#1472](https://github.com/containers/kubernetes-mcp-server/issues/1472)); the per-client share of `max_in_flight` is a follow-up.

Two reloadable options under `[http]` limit how many MCP requests one client can make, however many sessions it opens.
`0` disables the limit.
They are fields on [`HTTPConfig`](../../pkg/config/config.go), registered with `opt(...).reload()`, and documented with the other HTTP options in [`docs/configuration.md`](../configuration.md).

| TOML field | Default | What it limits |
|------------|---------|----------------|
| `http.client_rate_limit_rps` | `0` | MCP requests per second per client |
| `http.client_rate_limit_burst` | `0` (effective `10`) | Requests one client can make at once above that rate |

## Problem

`http.rate_limit_rps` ([#1051](https://github.com/containers/kubernetes-mcp-server/pull/1051)) keys a token bucket by MCP session ID.
A client that opens a new session gets a new, full bucket.
In `stateless` mode the go-sdk gives every request a temporary session with an empty ID, and the per-session limiter passes empty IDs through, so it never applies.
`stateless` is the mode suggested for containers and load balancing.
`max_in_flight` ([memory bounds](memory-bounds.md)) is one pool for the whole process, so it does not stop one client from using all of it.

## Where it runs

[`ClientRateLimitMiddleware`](../../pkg/http/ratelimit.go) is an HTTP middleware, chained right after [`AuthorizationMiddleware`](../../pkg/http/authorization.go) in [`Serve`](../../pkg/http/http.go).
It runs there, not as MCP middleware, because the MCP layer only sees request headers and `TokenInfo`: neither the verified token nor the client address reaches it.
It does not apply to stdio.
The per-session limiter is unchanged; both can be enabled.

Only `POST` to the MCP endpoint counts.
`GET` and `DELETE` on the MCP endpoint, health, metrics, stats, and well-known endpoints are not limited.
Requests rejected by `AuthorizationMiddleware` never reach it.

## Client key

1. When `AuthorizationMiddleware` verified a bearer token, the token's `sub` claim (`sub:<subject>`).
   `AuthorizationMiddleware` stores it in the request context only after offline and OIDC validation succeed.
2. Otherwise the client IP address from `getClientIP` (`ip:<address>`), which uses `X-Forwarded-For` / `X-Real-IP` only when `trust_proxy_headers` is set.

A token forwarded without local validation (`skip_jwt_verification` with no `authorization_url`) is not trusted as an identity, so those requests use the IP key.
Clients behind one proxy or NAT without a verified token share one limit.
`getClientIP` takes the first `X-Forwarded-For` entry, so with `trust_proxy_headers` the proxy must set that header itself rather than append to one sent by the client; otherwise a client can pick a new key on every request.

## Limit

One `golang.org/x/time/rate` token bucket per key, at `client_rate_limit_rps` with `client_rate_limit_burst` (`DefaultRateLimitBurst` when `0`).
The values are read from the live config on each request.
When they change, every bucket is dropped so all clients start with the new size.
A refused request does not use a token.

## What the client sees

HTTP `429 Too Many Requests` with `Retry-After` set to the seconds until the next token, rounded up, at least `1`.
The MCP handler is not called.

## Retention

Buckets are kept in memory, one per key seen.
At most once a minute, buckets idle longer than 10 minutes, or than the time a full bucket takes to refill if that is longer (capped at 24 hours), are removed.
Removing one never gives a client more tokens than it would have had.

## Follow-up

- A per-client share of `max_in_flight`, so one client cannot hold every slot.
- A metric for refused requests.

## Tests

Cover:

- burst, refusal, and refill
- a refused request that does not use a token
- separate buckets per key, and subject keys separate from IP keys
- new settings resetting buckets
- removal of idle buckets, and a slow-refilling bucket kept until it has refilled
- disabled by default, default burst
- only `POST` to the MCP endpoint counted
- one subject from two addresses as one client, two subjects behind one address as two
- `X-Forwarded-For` ignored without `trust_proxy_headers`, used with it
- enabling and disabling through a config reload
- `429` with `Retry-After`
- a verified token storing its subject; a passthrough token storing none
- the real server in `stateless` mode, with `http.rate_limit_rps` also set, refusing the request over the client limit
