# Trusted Proxy Impersonation

Set `cluster_auth_mode = "impersonation"` when an authentication proxy identifies callers and MCP uses shared kubeconfig credentials. Kubernetes authorizes each tool request against the caller's RBAC bindings.

```toml
port = "8080"
cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = ["127.0.0.1/32", "::1/128"]
kubeconfig = "/etc/kubernetes-mcp-server/kubeconfig"
```

The CIDRs identify the proxy's immediate network peer; forwarded address headers and `trust_proxy_headers` do not grant trust. Restrict network access to the authenticating proxy, use TLS across host boundaries, and avoid CIDRs shared with ordinary workloads.

Stateful and stateless HTTP with kubeconfig-backed clusters are supported. Pods must mount an explicit kubeconfig. Stdio, in-cluster and other providers, token exchange, Kiali, and NetObserv are unsupported. Changing impersonation mode or trusted proxy CIDRs requires a restart.

## Proxy contract

On **every request**, including existing MCP sessions, the proxy must:

1. Authenticate the caller and authorize access to this MCP endpoint.
2. Remove all caller-supplied `Impersonate-*` headers.
3. Set exactly one `Impersonate-User` and optional separate `Impersonate-Group` headers from an authorized group allowlist. Comma-separated groups are not split. Values must be nonempty, unpadded, and free of control characters.
4. Forward the caller's nonempty `Authorization: Bearer ...` header.

`Impersonate-Uid` and `Impersonate-Extra-*` are rejected. A trusted proxy can assert any user or group permitted by the backend's impersonation grants. The proxy must not substitute a bearer token shared by all callers.

MCP requires current identity on every tool request, binds sessions to users, and derives separate Kubernetes clients and caches. Kubernetes requests retain backend authentication; caller bearer tokens are never forwarded to Kubernetes. Missing identity never falls back to backend identity.

Authentication is delegated to the proxy by default. Optional MCP token verification uses the existing `require_oauth`, `authorization_url`, and `oauth_audience` settings; `skip_jwt_verification` is rejected. Token verification does not derive the Kubernetes user or replace the proxy contract.

## Kubernetes permissions

The backend identity needs `impersonate` permission on the intended `users` and `groups`; restrict grants with `resourceNames` where practical. Give those users workload permissions through ordinary RoleBindings or ClusterRoleBindings. An API proxy between MCP and Kubernetes must preserve impersonation headers.

Background capability discovery uses backend identity, which needs discovery access. Tool and prompt requests use caller identity. Kiali and NetObserv reject impersonation because their external APIs cannot enforce Kubernetes caller identity.
