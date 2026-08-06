# Authorization

## Overview

The kubernetes-mcp-server supports flexible authorization for HTTP-mode deployments, from fully open (default) to OIDC-secured with backend token exchange. Authorization controls who can reach the MCP tools and, when token exchange is configured, maps the caller's identity to a Kubernetes-scoped credential so that every cluster operation runs as the authenticated user rather than the server's service account.

The implementation aligns with the [MCP Authorization specification](https://modelcontextprotocol.io/specification/draft/basic/authorization) and follows the security guidance in the [MCP Security Best Practices](https://modelcontextprotocol.io/docs/tutorials/security/security_best_practices).

The server supports four progressively stricter authorization modes:

1. **No authorization** (default) - every request reaches the tools using the server's own Kubernetes credentials.
2. **Raw token validation** - the server requires a Bearer JWT and validates it offline (structure, expiration, optional audience).
3. **OIDC provider validation** - on top of offline checks, the token signature is verified against an OIDC provider's [JWKS](https://datatracker.ietf.org/doc/html/rfc7517).
4. **OIDC with token exchange** - after OIDC validation, the server exchanges the caller's token for a cluster-scoped token via [RFC 8693](https://datatracker.ietf.org/doc/html/rfc8693) so Kubernetes API calls run as the end user.

## Requirements

- When `require_oauth` is false (default), no Authorization header is needed and all requests use the server's ambient credentials (in-cluster service account or kubeconfig)
- When `require_oauth` is false but a client provides an Authorization header, the token is passed through to the Kubernetes API without validation (token passthrough)
- When `require_oauth` is true, every request to MCP endpoints must carry an `Authorization: Bearer <token>` header
- Infrastructure endpoints (`/healthz`, `/metrics`) and well-known endpoints are always exempt from authorization
- Offline validation rejects tokens that are malformed, expired, or (when `oauth_audience` is set) missing the expected audience claim
- When an `authorization_url` is configured, the token is also verified online against the OIDC provider's JWKS
- When STS credentials (`sts_client_id`, `sts_audience`) are configured, the server exchanges the incoming token for a cluster-scoped access token before calling the Kubernetes API
- Each MCP tool call gets its own derived Kubernetes client with the exchanged (or passed-through) token - clients are cleaned up automatically when the request ends
- The `Authorization` header is never logged, even at high verbosity levels
- Tokens received through MCP protocol extra headers are propagated via context, supporting both the standard `Authorization` header and the legacy `kubernetes-authorization` header

## Authorization Modes

### 1. No Authorization (Default)

The server operates without any authentication middleware. Kubernetes operations use whatever credentials are available:

- **In-cluster**: the pod's service account
- **Out-of-cluster**: the current context from `~/.kube/config`

```bash
./kubernetes-mcp-server --port 8080
```

### 2. Raw Token Validation

The server requires a Bearer JWT and performs offline checks:

- The token must be a structurally valid JWT (well-formed base64 segments with parseable header and claims) — no cryptographic signature verification is performed since no JWKS is available in this mode
- The `exp` claim must not have passed
- If `oauth_audience` is set, the token's `aud` claim must contain that value

No network call to an identity provider is made. This mode gates access based on token structure and claims, not cryptographic proof of origin. It is useful when the caller already holds a trusted token and the server just needs to enforce expiration and audience.

```bash
./kubernetes-mcp-server --port 8080 --require-oauth
# With audience restriction:
./kubernetes-mcp-server --port 8080 --require-oauth --oauth-audience=mcp-server
```

### 3. OIDC Provider Validation

On top of the offline checks, the server fetches the OIDC provider's JWKS ([OpenID Connect Discovery 1.0](https://openid.net/specs/openid-connect-discovery-1_0.html)) and verifies the token's cryptographic signature online. This ensures that only tokens issued by the trusted provider are accepted.

```bash
./kubernetes-mcp-server --port 8080 --require-oauth \
  --authorization-url https://keycloak.example.com/realms/myrealm
```

### 4. OIDC with Token Exchange

After validating the incoming token, the server exchanges it for a new token scoped to the target Kubernetes cluster. This enables user-level identity propagation: the Kubernetes API server sees the end user's identity, not the MCP server's service account.

The exchange follows the [OAuth 2.0 Token Exchange (RFC 8693)](https://datatracker.ietf.org/doc/html/rfc8693) protocol. The server authenticates to the identity provider using its own client credentials (`sts_client_id` / `sts_client_secret`) and presents the user's token as the `subject_token`.

```toml
require_oauth = true
oauth_audience = "mcp-server"
authorization_url = "https://keycloak.example.com/realms/myrealm"

sts_client_id = "mcp-server"
sts_client_secret = "secret"
sts_audience = "openshift"
sts_scopes = ["mcp:openshift"]
```

## Token Passthrough

When `require_oauth` is false and the server has no OIDC provider configured, the authorization middleware is effectively disabled. However, if a client includes an Authorization header in this mode, the token is passed through directly to the Kubernetes API without any server-side validation. In this mode, the Kubernetes API server is the sole authentication and authorization boundary — the MCP server performs no token verification and relies entirely on the cluster's own RBAC to accept or reject the request. The MCP auth header propagation middleware always extracts the Authorization header from request extras regardless of whether `require_oauth` is enabled. When the Kubernetes client is derived for a tool call, any Bearer token found in the context is used as-is to authenticate against the Kubernetes API.

This is a deliberate exception to the [MCP specification's guidance against token passthrough](https://modelcontextprotocol.io/docs/tutorials/security/security_best_practices#token-passthrough). The MCP specification explicitly warns that token passthrough is an anti-pattern because the MCP server cannot verify the token was issued for it, which breaks audience restriction and opens security risks like lateral token reuse.

The kubernetes-mcp-server supports token passthrough specifically for trusted consumers like OpenShift Lightspeed, where the calling system has already authenticated the user and the token it passes is bound to that specific user's identity. In these scenarios, the MCP host is a trusted backend service (not an untrusted MCP client), the token is already scoped to the user who initiated the request through the host's own interface, and adding a second layer of OAuth validation would be redundant.

Token passthrough should not be used when the MCP server is exposed to untrusted or arbitrary MCP clients. For those scenarios, `require_oauth` should be enabled with proper token validation and, ideally, token exchange.

## Token Validation Flow

When `require_oauth` is enabled, every non-infrastructure request goes through this pipeline:

```
HTTP Request (Authorization: Bearer <token>)
    |
    | 1. Extract Bearer token from header
    |    Missing or non-Bearer header -> 401 (error=missing_token)
    v
Offline Validation
    |
    | 2. Parse JWT (any supported algorithm)
    | 3. Validate exp claim (reject expired)
    | 4. Validate aud claim (if oauth_audience is configured)
    |    Any failure -> 401 (error=invalid_token)
    v
OIDC Validation (if authorization_url is configured)
    |
    | 5. Verify token signature against provider JWKS
    |    Signature mismatch -> 401 (error=invalid_token)
    v
Request proceeds to MCP handler
```

The `401` response includes a `WWW-Authenticate` header:

```
WWW-Authenticate: Bearer realm="Kubernetes MCP Server", audience="mcp-server", error="invalid_token"
```

## Token Exchange

Token exchange maps the user's MCP-scoped token to a Kubernetes-scoped token. This happens transparently on each tool call, before the Kubernetes client is created.

### Global Token Exchange (STS)

The server-level STS configuration exchanges tokens using `golang.org/x/oauth2/externalaccount`. It requires:

- An OIDC provider (`authorization_url`) for the token endpoint
- Client credentials (`sts_client_id`, `sts_client_secret`)
- A target audience (`sts_audience`)

### Per-Target Token Exchange

For multi-cluster deployments (e.g. ACM), each target cluster can have its own token exchange configuration with a pluggable strategy:

- **[RFC 8693](https://datatracker.ietf.org/doc/html/rfc8693)** (`rfc8693`): Standard OAuth 2.0 Token Exchange with `requested_token_type` set to access token
- **Keycloak V1** (`keycloak-v1`): Extends RFC 8693 with the [`subject_issuer` parameter](https://www.keycloak.org/docs/latest/securing_apps/index.html#external-token-to-internal-token-exchange) for cross-realm token exchange

Per-target configuration supports:

- Independent token endpoints per cluster
- Independent client credentials per cluster
- Same-realm exchange (subject token type: `access_token`) vs. cross-realm exchange (subject token type: `jwt`, with `subject_issuer`)
- Custom CA certificates for TLS verification
- Client authentication style: request body parameters or HTTP Basic header

### Token Exchange Flow

```
MCP Tool Call
    |
    | 1. Extract Authorization header from context
    v
Per-Target Exchange (if provider implements TokenExchangeProvider)
    |
    | 2a. Look up target-specific config
    | 2b. Use registered strategy (keycloak-v1 or rfc8693)
    | 2c. Exchange token at target's token endpoint
    v
OR Global STS Exchange (fallback)
    |
    | 2a. Build STS from server-level config
    | 2b. Exchange via externalaccount.NewTokenSource
    v
Derived Kubernetes Client
    |
    | 3. Create new rest.Config with exchanged Bearer token
    | 4. All Kubernetes API calls use this client
    | 5. Client is cleaned up when request context ends
    v
Kubernetes API Server
```

## Well-Known Endpoints

When `authorization_url` is configured, the server acts as a reverse proxy for OAuth discovery endpoints:

- `/.well-known/oauth-authorization-server` ([RFC 8414](https://datatracker.ietf.org/doc/html/rfc8414))
- `/.well-known/oauth-protected-resource` ([RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728))
- `/.well-known/openid-configuration` ([OpenID Connect Discovery 1.0](https://openid.net/specs/openid-connect-discovery-1_0.html))

The server fetches the upstream metadata from the OIDC provider and can modify the response:

- If `disable_dynamic_client_registration` is true: removes `registration_endpoint` ([RFC 7591](https://datatracker.ietf.org/doc/html/rfc7591)) and sets `require_request_uri_registration` to false
- If `oauth_scopes` is set: overrides the `scopes_supported` array

CORS headers (`Access-Control-Allow-Origin: *`) are added to all well-known responses so browser-based MCP clients (e.g. MCP Inspector) can discover the OAuth configuration.

For security, client request headers are not propagated to the upstream provider, response headers are filtered to a safe allowlist, and responses are limited to 1 MB.

## MCP Client OAuth Discovery

When an MCP client (VSCode, MCP Inspector, Claude Desktop, etc.) connects to the server over HTTP and the server has `require_oauth` enabled, the client must discover and complete an OAuth flow before it can call tools. This flow follows the [MCP Authorization specification](https://modelcontextprotocol.io/specification/draft/basic/authorization#authorization-flow-steps). The server participates by returning appropriate HTTP responses and proxying OAuth metadata.

### Discovery Flow

```
MCP Client                          MCP Server                     Authorization Server
    |                                    |                                |
    | 1. MCP request (no token)          |                                |
    |----------------------------------->|                                |
    |                                    |                                |
    | 2. HTTP 401 Unauthorized           |                                |
    |    WWW-Authenticate: Bearer ...    |                                |
    |<-----------------------------------|                                |
    |                                    |                                |
    | 3. GET /.well-known/               |                                |
    |    oauth-protected-resource        |                                |
    |----------------------------------->|                                |
    |                                    | 4. Proxy to authorization_url  |
    |                                    |------------------------------->|
    |                                    |<-------------------------------|
    | 5. Protected resource metadata     |                                |
    |    (authorization_servers, etc.)   |                                |
    |<-----------------------------------|                                |
    |                                    |                                |
    | 6. GET /.well-known/               |                                |
    |    oauth-authorization-server      |                                |
    |----------------------------------->|                                |
    |                                    | 7. Proxy (may modify response) |
    |                                    |------------------------------->|
    |                                    |<-------------------------------|
    | 8. Authorization server metadata   |                                |
    |    (endpoints, scopes, DCR, etc.)  |                                |
    |<-----------------------------------|                                |
    |                                    |                                |
    | 9. Client registration + OAuth     |                                |
    |    authorization code flow         |                                |
    |<-------------------------------------------------------------->    |
    |                                    |                                |
    | 10. MCP request with access token  |                                |
    |----------------------------------->|                                |
    |                                    |                                |
```

1. The MCP client sends its first request without a token
2. The server returns `401 Unauthorized` with a `WWW-Authenticate: Bearer` header indicating the realm and (optionally) audience
3. The client fetches `/.well-known/oauth-protected-resource` to discover which authorization server to use
4. The server proxies this request to the upstream OIDC provider (`authorization_url`)
5. The client receives the protected resource metadata, which includes the authorization server location
6. The client fetches `/.well-known/oauth-authorization-server` (or `/.well-known/openid-configuration`) to discover the authorization server's endpoints
7. The server proxies this request, optionally modifying the response (removing DCR endpoint, overriding scopes)
8. The client receives the authorization server metadata with token endpoints, authorization endpoints, supported scopes, and client registration information
9. The client registers (or uses pre-registered credentials) and completes the OAuth authorization code flow with the user
10. The client includes the obtained access token in all subsequent MCP requests

### Client Registration

The [MCP specification defines a priority order](https://modelcontextprotocol.io/specification/draft/basic/authorization#client-registration-approaches) for how clients obtain credentials:

1. **Pre-registered client**: the client has a hardcoded or user-provided client ID for this server
2. **[Client ID Metadata Documents](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-00)**: the client uses an HTTPS URL as its `client_id` (when the authorization server supports `client_id_metadata_document_supported`)
3. **[Dynamic Client Registration (DCR)](https://datatracker.ietf.org/doc/html/rfc7591)**: the client registers dynamically via the `registration_endpoint` advertised in the authorization server metadata
4. **Manual entry fallback**: if none of the above work, the client prompts the user for a client ID

### Disabling Dynamic Client Registration

Some authorization servers (e.g. Keycloak) expose a `registration_endpoint` in their metadata but don't actually support open registration, or the administrator wants to control which clients connect. When `disable_dynamic_client_registration` is set to `true`, the server removes the `registration_endpoint` from the proxied authorization server metadata.

This forces MCP clients that would otherwise attempt DCR (such as VSCode) to fall back to their next option - typically prompting the user for a client ID via a manual entry form. The user then enters the pre-registered client ID (e.g. `mcp-client`) configured in the authorization server.

### Scope Override

The `oauth_scopes` configuration allows the server to override the `scopes_supported` array in the proxied authorization server metadata. This is useful when the authorization server advertises many scopes but the MCP server only needs a specific subset (e.g. `["openid", "mcp-server"]`). MCP clients use `scopes_supported` to determine which scopes to request during the authorization flow, so overriding this controls what the client asks for.

## Per-Request Kubernetes Clients

When an Authorization header is present, the server creates a dedicated Kubernetes client for each tool call:

- A new `rest.Config` is built with the (possibly exchanged) Bearer token
- The server-side TLS configuration (CA bundle, server name) is copied from the base config
- Client-side auth from the base kubeconfig (certificates, service account tokens) is stripped
- The derived client is automatically closed when the request context ends
- If the Authorization header is absent and `require_oauth` is false, the server falls back to its base credentials

## TLS

The HTTP server supports TLS for production deployments:

- Configured via `tls_cert` and `tls_key` (both required together)
- Noisy TLS handshake errors from health check probes (TCP-only connects) are filtered from logs
- Custom CA certificates (`certificate_authority`) can be provided for verifying the OIDC provider's TLS certificate

## Configuration

### TOML Reference

| TOML Field | CLI Flag | Default | Description |
|------------|----------|---------|-------------|
| `require_oauth` | `--require-oauth` | `false` | Enable OAuth authorization |
| `oauth_audience` | `--oauth-audience` | `""` | Expected audience in JWT `aud` claim |
| `authorization_url` | `--authorization-url` | `""` | OIDC provider URL for online validation and token exchange |
| `server_url` | `--server-url` | `""` | Server URL for protected resource metadata |
| `certificate_authority` | `--certificate-authority` | `""` | CA certificate file for OIDC provider TLS verification |
| `disable_dynamic_client_registration` | - | `false` | Remove `registration_endpoint` from well-known responses |
| `oauth_scopes` | - | `[]` | Override `scopes_supported` in well-known responses |
| `sts_client_id` | - | `""` | OAuth client ID for backend token exchange |
| `sts_client_secret` | - | `""` | OAuth client secret for backend token exchange |
| `sts_audience` | - | `""` | Target audience for exchanged tokens |
| `sts_scopes` | - | `[]` | Scopes to request during token exchange |
| `tls_cert` | `--tls-cert` | `""` | TLS certificate file path |
| `tls_key` | `--tls-key` | `""` | TLS private key file path |

### Per-Target Token Exchange (Provider Config)

For multi-cluster providers (e.g. ACM), token exchange is configured per cluster:

```toml
[cluster_provider_configs.acm]
token_exchange_strategy = "keycloak-v1"

[cluster_provider_configs.acm.clusters."local-cluster"]
token_url = "https://keycloak.example.com/realms/hub/protocol/openid-connect/token"
client_id = "mcp-sts"
client_secret = "<your-client-secret>"
audience = "mcp-server"
subject_token_type = "urn:ietf:params:oauth:token-type:access_token"
ca_file = "/etc/certs/ca.crt"

[cluster_provider_configs.acm.clusters."managed-cluster"]
token_url = "https://keycloak.example.com/realms/managed/protocol/openid-connect/token"
client_id = "mcp-server"
client_secret = "<your-managed-client-secret>"
subject_issuer = "hub-realm"
audience = "mcp-server"
subject_token_type = "urn:ietf:params:oauth:token-type:jwt"
ca_file = "/etc/certs/ca.crt"
auth_style = "header"
```

| Field | Default | Description |
|-------|---------|-------------|
| `token_url` | - | Token endpoint for this target's realm |
| `client_id` | - | OAuth client ID for exchange |
| `client_secret` | - | OAuth client secret for exchange |
| `audience` | - | Target audience for exchanged token |
| `subject_token_type` | - | Token type URN (`access_token` for same-realm, `jwt` for cross-realm) |
| `subject_issuer` | `""` | IDP alias for cross-realm exchange (Keycloak V1 only) |
| `scopes` | `[]` | Scopes to request during exchange |
| `ca_file` | `""` | CA certificate for the token endpoint TLS |
| `auth_style` | `"params"` | Client auth method: `"params"` (body) or `"header"` (HTTP Basic) |

## Non-goals

- Session management or cookie-based authentication
- User registration or account management
- Fine-grained per-tool authorization (access control is handled by Kubernetes RBAC)
- Token refresh or caching (each tool call exchanges independently)

## References

### MCP Specification

- [MCP Authorization](https://modelcontextprotocol.io/specification/draft/basic/authorization) - OAuth authorization flow for MCP clients and servers
- [MCP Security Best Practices](https://modelcontextprotocol.io/docs/tutorials/security/security_best_practices) - Security considerations including token passthrough guidance

### OAuth and OIDC Standards

- [RFC 8693 - OAuth 2.0 Token Exchange](https://datatracker.ietf.org/doc/html/rfc8693) - Token exchange protocol used for STS and per-target exchange
- [RFC 9728 - OAuth 2.0 Protected Resource Metadata](https://datatracker.ietf.org/doc/html/rfc9728) - Discovery of authorization server from protected resource (`.well-known/oauth-protected-resource`)
- [RFC 8414 - OAuth 2.0 Authorization Server Metadata](https://datatracker.ietf.org/doc/html/rfc8414) - Authorization server discovery (`.well-known/oauth-authorization-server`)
- [RFC 7591 - OAuth 2.0 Dynamic Client Registration](https://datatracker.ietf.org/doc/html/rfc7591) - Dynamic client registration protocol
- [RFC 6750 - OAuth 2.0 Bearer Token Usage](https://datatracker.ietf.org/doc/html/rfc6750) - Bearer token format and WWW-Authenticate header
- [OAuth Client ID Metadata Documents](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-00) - URL-based client registration alternative to DCR
- [OpenID Connect Discovery 1.0](https://openid.net/specs/openid-connect-discovery-1_0.html) - OIDC provider metadata and JWKS discovery

### Keycloak

- [Keycloak Token Exchange](https://www.keycloak.org/docs/latest/securing_apps/index.html#_token-exchange) - Keycloak's token exchange implementation including cross-realm `subject_issuer` parameter

### Project Documentation

- [Keycloak OIDC Setup](../KEYCLOAK_OIDC_SETUP.md) - Local development environment with Kind cluster and Keycloak
- [Configuration Reference](../configuration.md) - Complete TOML configuration reference

## Related Code

| Component | Location |
|-----------|----------|
| Authorization middleware | `pkg/http/authorization.go` |
| JWT claims parsing and validation | `pkg/http/authorization.go` |
| Well-known endpoint proxy | `pkg/http/wellknown.go` |
| HTTP server setup and middleware chain | `pkg/http/http.go` |
| MCP auth header propagation | `pkg/mcp/middleware.go` |
| Per-request Kubernetes client derivation | `pkg/kubernetes/manager.go` |
| Token exchange orchestration | `pkg/kubernetes/token_exchange.go` |
| Token-exchanging provider wrapper | `pkg/kubernetes/provider_token_exchange.go` |
| STS implementation | `pkg/kubernetes/sts.go` |
| Token exchange registry (RFC 8693, Keycloak V1) | `pkg/tokenexchange/` |
| Per-target exchange config | `pkg/tokenexchange/config.go` |
| StaticConfig auth fields | `pkg/config/config.go` |
| Auth-related interfaces | `pkg/api/config.go` |
| CLI flag definitions and validation | `pkg/kubernetes-mcp-server/cmd/root.go` |
| Keycloak local dev setup | `docs/KEYCLOAK_OIDC_SETUP.md` |

<details>
<summary>Design Decisions</summary>

### Why offline validation before OIDC?

Offline checks (expiry, audience) are cheap and catch obvious problems without a network round-trip. This reduces load on the identity provider and provides faster feedback for clearly invalid tokens. OIDC verification only runs if offline checks pass.

### Why token exchange instead of token passthrough?

The [MCP specification explicitly forbids token passthrough](https://modelcontextprotocol.io/docs/tutorials/security/security_best_practices#token-passthrough) as an anti-pattern: passing a token through without verifying it was issued for the MCP server breaks audience restriction, makes audit trails unreliable, and enables lateral token reuse across services. Token exchange ([RFC 8693](https://datatracker.ietf.org/doc/html/rfc8693)) solves this by producing a new token with the correct audience and scopes for the target cluster. The MCP server validates the incoming token (audience: `mcp-server`), then exchanges it for a cluster-scoped token (audience: `openshift`), maintaining proper audience separation at every hop.

### Why support token passthrough despite the MCP spec warning?

Token passthrough is supported as a deliberate exception for trusted backend consumers like OpenShift Lightspeed. In these deployments, the MCP server is not exposed to arbitrary MCP clients - it sits behind a trusted backend that has already authenticated the user. The token passed is already specific to the user acting through the host's interface. Requiring the host to re-obtain a token via the full MCP OAuth flow would add complexity without security benefit, since the trust boundary is between the host and its user, not between the host and the MCP server. This mode should only be used when the MCP server is deployed behind a trusted service, never when exposed to untrusted clients.

### Why a derived Kubernetes client per request?

Each tool call may carry a different user identity. Creating a fresh `rest.Config` with the caller's token ensures that Kubernetes RBAC is evaluated against the correct identity. The client is garbage-collected when the request context ends, preventing credential leakage between requests.

### Why strip client-side auth from derived configs?

The base kubeconfig may contain service account tokens, client certificates, or exec-based credential plugins. When creating a derived config with a user's Bearer token, carrying over those credentials could cause the wrong identity to be used. The derived config copies only server-side TLS settings (CA, server name) and discards all client auth.

### Why proxy well-known endpoints instead of serving static metadata?

The OIDC provider's metadata (JWKS URIs, supported grants, endpoints) can change. Proxying ensures the MCP server always reflects the upstream state without manual synchronization. The proxy also allows the server to selectively modify the metadata (disable DCR, override scopes) without maintaining a full copy.

### Why disable dynamic client registration for VSCode?

VSCode's MCP client only supports [Dynamic Client Registration (RFC 7591)](https://datatracker.ietf.org/doc/html/rfc7591) for OAuth. When the authorization server exposes a `registration_endpoint`, VSCode uses it - but if the server doesn't support DCR, the flow fails silently. Removing `registration_endpoint` from the metadata triggers VSCode's [fallback behavior](https://modelcontextprotocol.io/specification/draft/basic/authorization#client-registration-approaches): prompting the user to manually enter a client ID. This workaround is controlled by `disable_dynamic_client_registration`. See also [microsoft/vscode#257415](https://github.com/microsoft/vscode/issues/257415).

### Why two token exchange strategies?

[RFC 8693](https://datatracker.ietf.org/doc/html/rfc8693) is the standard but doesn't cover Keycloak's cross-realm exchange, which requires the [`subject_issuer` parameter](https://www.keycloak.org/docs/latest/securing_apps/index.html#external-token-to-internal-token-exchange) to identify the foreign realm's identity provider alias. The Keycloak V1 strategy adds this parameter. Both strategies share the same base implementation and registry.

### Why per-target exchange config for multi-cluster?

In ACM deployments, each managed cluster may live in a different Keycloak realm with its own token endpoint, client credentials, and trust relationships. A single global STS config cannot accommodate this. Per-target configuration allows same-realm exchange for the hub's local-cluster and cross-realm exchange (with `subject_issuer`) for remote managed clusters.

### Why support both `Authorization` and `kubernetes-authorization` headers?

The standard `Authorization` header is the OAuth-compliant way to pass tokens. The `kubernetes-authorization` header exists for backward compatibility with earlier MCP client implementations that used a custom header name. The server checks the standard header first and falls back to the custom one.

</details>
