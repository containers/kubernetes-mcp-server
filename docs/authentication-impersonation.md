# Trusted Proxy Impersonation

Use `cluster_auth_mode = "impersonation"` when an authentication proxy identifies MCP callers and the server connects to Kubernetes with shared backend credentials. Kubernetes authorizes each request against the impersonated user's RBAC bindings.

```text
MCP client → authentication proxy → MCP server → Kubernetes API
             verifies caller        backend credentials
             sets user/groups       + impersonated user/groups
```

## Server configuration

```toml
port = "8080"
cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = ["127.0.0.1/32", "::1/128"]
kubeconfig = "/etc/kubernetes-mcp-server/kubeconfig"
```

The example trusts a proxy on the same host or in the same pod. For a separate proxy, configure its immediate peer CIDRs. Forwarded address headers do not affect this check. `trust_proxy_headers` does not grant permission to supply an impersonation identity.

This mode supports HTTP and kubeconfig-backed clusters only. In-cluster, KCP, disabled, and custom providers are rejected. When running inside a pod, set an explicit `kubeconfig` path to select the kubeconfig provider; there is no automatic fallback to the pod's service account.

MCP preserves backend authentication from kubeconfig. The caller's bearer token is used for MCP authentication and session binding, not for Kubernetes authentication. The server never falls back to the backend identity when an impersonation request is missing identity.

Provider discovery and background watchers use the backend identity: grant it API discovery access. Tool and prompt requests still require a caller identity and use impersonation.

Stateful and stateless HTTP are supported. Changing into or out of impersonation mode requires a restart so existing sessions cannot retain incompatible identity bindings. The trusted proxy list is reloadable; updates affect subsequent requests. Token exchange and stdio are incompatible with this mode.

Kiali and NetObserv are unsupported in impersonation mode. Their separate HTTP APIs do not enforce the Kubernetes impersonation identity, so client construction fails before any external request. This also applies to an impersonated client retained across an authentication-mode reload. Backend credentials are never forwarded to those APIs through this mode; use only toolsets that perform operations through the derived Kubernetes client.

## Proxy contract

The proxy must perform all of these steps on **every request**, including requests with an existing MCP session:

1. Authenticate the caller and check that access to this MCP endpoint is allowed.
2. Remove all caller-supplied `Impersonate-*` headers.
3. Set exactly one `Impersonate-User` header to the authorized Kubernetes username. Supply optional `Impersonate-Group` headers from an explicit group allowlist, using a separate header for each group; comma-separated values are not split. Identity values must be nonempty, unpadded, and free of control characters.
4. Forward the caller's nonempty `Authorization: Bearer ...` header. Do not substitute a bearer token shared by all users.

`Impersonate-Uid` and `Impersonate-Extra-*` are not supported and are rejected. Supply explicit narrow groups when a Kubernetes access proxy would otherwise apply broader default groups.

The proxy is an authorization boundary: a trusted peer can assert any supported user and group permitted by the backend. Restrict MCP network access to that proxy, use TLS across host boundaries, and avoid trusting CIDRs containing ordinary workloads or clients. A shared ingress address is insufficient unless that ingress itself authenticates callers and replaces the identity headers.

For additional token verification inside MCP, configure:

```toml
require_oauth = true
authorization_url = "https://idp.example.com/realms/users"
oauth_audience = "kubernetes-mcp-server"
```

Local verification does not replace the trusted proxy contract or derive Kubernetes identity from token claims. With `require_oauth = false`, authentication is delegated to the proxy. `skip_jwt_verification = true` is rejected in impersonation mode.

## Kubernetes permissions

The backend identity needs permission to impersonate the intended users and groups. Restrict those grants with `resourceNames` where practical. For example, the following ClusterRole permits only two users and one group; bind it to the backend identity separately:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mcp-impersonator
rules:
  - apiGroups: [""]
    resources: ["users"]
    verbs: ["impersonate"]
    resourceNames: ["user-a", "user-b"]
  - apiGroups: [""]
    resources: ["groups"]
    verbs: ["impersonate"]
    resourceNames: ["developers"]
```

Grant workload permissions to these users with ordinary RoleBindings or ClusterRoleBindings. The impersonation grant itself does not grant workload permissions. A Kubernetes access proxy between MCP and the API server must also permit and preserve the impersonated identity.

Verify two users concurrently against the same MCP endpoint: each can access its permitted resources, cross-user operations are denied, and audit records contain the correct identity. Test logs, exec, Helm, and backend credential renewal as well as ordinary resource operations.
