# Auth-Headers Provider

The `auth-headers` cluster provider strategy enables multi-tenant Kubernetes MCP server deployments where each user authenticates with their own Kubernetes token via HTTP request headers.

## Overview

This provider:
- **Requires authentication via request headers** (`Authorization` or `kubernetes-authorization`)
- **Extracts cluster connection details** from kubeconfig (server URL, CA certificates)
- **Strips all authentication credentials** from the kubeconfig
- **Creates dynamic Kubernetes clients** per request using the provided bearer tokens

## Use Cases

- **Multi-tenant SaaS deployments** - Single MCP server instance serving multiple users
- **Zero-trust architectures** - No stored credentials, authentication per request
- **OIDC/OAuth integration** - Users authenticate via identity provider, tokens forwarded to Kubernetes
- **Auditing & compliance** - Each request uses the user's actual identity for Kubernetes RBAC

## Configuration

### Basic Setup

```bash
kubernetes-mcp-server \
  --port 8080 \
  --kubeconfig /path/to/kubeconfig \
  --cluster-provider-strategy auth-headers
```

The server will:
1. Read cluster connection details from the kubeconfig
2. Automatically enable `--require-oauth`
3. Reject any requests without valid bearer tokens

### TOML Configuration

```toml
cluster_provider_strategy = "auth-headers"
kubeconfig = "/path/to/kubeconfig"
require_oauth = true
validate_token = true  # Optional: validate tokens against Kubernetes API
```

### With Token Validation

```bash
kubernetes-mcp-server \
  --port 8080 \
  --kubeconfig /path/to/kubeconfig \
  --cluster-provider-strategy auth-headers \
  --validate-token
```

This validates each token using Kubernetes TokenReview API before allowing operations.

## How It Works

### 1. Initialization

When the server starts:
```
Kubeconfig → Extract cluster info (server URL, CA cert) → Create base manager
                                  ↓
                        Strip all auth credentials
                                  ↓
                        Ready to accept requests
```

### 2. Request Processing

For each MCP request:
```
HTTP Request → Extract Authorization header → Create derived Kubernetes client
                              ↓                           ↓
                    "Bearer <token>"           Uses token for authentication
                                                           ↓
                                               Execute Kubernetes operation
```

### 3. Security Model

```
┌──────────────────┐
│   MCP Client     │ (User's application)
│  (Claude, etc)   │
└────────┬─────────┘
         │ Bearer <user-token>
         ↓
┌──────────────────┐
│   MCP Server     │
│  (auth-headers)  │
└────────┬─────────┘
         │ Uses user's token
         ↓
┌──────────────────┐
│  Kubernetes API  │
│     Server       │
└──────────────────┘
         ↓
    RBAC enforced with
    user's actual identity
```

## Client Usage

### Using the Go MCP Client

```go
import "github.com/mark3labs/mcp-go/client/transport"

// Get user's Kubernetes token (from OIDC, service account, etc.)
userToken := getUserKubernetesToken()

client := NewMCPClient(
    transport.WithHTTPHeaders(map[string]string{
        "Authorization": "Bearer " + userToken
    })
)
```

### Using Claude Desktop

```json
{
  "mcpServers": {
    "kubernetes": {
      "url": "https://mcp-server.example.com/sse",
      "headers": {
        "Authorization": "Bearer YOUR_KUBERNETES_TOKEN"
      }
    }
  }
}
```

### Using cURL

```bash
curl -X POST https://mcp-server.example.com/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer eyJhbGci..." \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "pods_list",
      "arguments": {"namespace": "default"}
    },
    "id": 1
  }'
```

## Comparison with Other Providers

| Feature | auth-headers | kubeconfig | in-cluster | disabled |
|---------|--------------|------------|------------|----------|
| **Multi-tenant** | ✅ Yes | ❌ No | ❌ No | ❌ No |
| **Multi-cluster** | ❌ No | ✅ Yes | ❌ No | ❌ No |
| **Per-request auth** | ✅ Yes | ❌ No | ❌ No | ❌ No |
| **Requires headers** | ✅ Required | ❌ Optional | ❌ Optional | ❌ Optional |
| **Stored credentials** | ❌ None | ✅ Kubeconfig | ✅ SA token | ✅ Kubeconfig |
| **Use case** | SaaS/Multi-user | Local dev | In-cluster | Single cluster |

## Security Considerations

### ✅ Advantages

- **No stored credentials** - Server doesn't store any Kubernetes authentication
- **Per-request authentication** - Each request uses fresh, user-specific token
- **RBAC enforcement** - Kubernetes enforces permissions using actual user identity
- **Token expiration** - Short-lived tokens automatically expire
- **Audit trails** - Kubernetes audit logs show actual user, not service account

### ⚠️ Important Notes

1. **Tokens in transit** - Use HTTPS to protect tokens in HTTP headers
2. **Token validation** - Enable `--validate-token` for additional security
3. **Rate limiting** - Consider implementing rate limiting per token/user
4. **Token rotation** - Clients must handle token refresh/expiration
5. **Network security** - Ensure MCP server can reach Kubernetes API

## Example Deployment

### Docker Compose

```yaml
version: '3.8'
services:
  kubernetes-mcp-server:
    image: quay.io/containers/kubernetes_mcp_server:latest
    ports:
      - "8080:8080"
    command:
      - --port=8080
      - --kubeconfig=/kubeconfig/config
      - --cluster-provider-strategy=auth-headers
      - --validate-token
    volumes:
      - ./kubeconfig:/kubeconfig:ro
    environment:
      - LOG_LEVEL=1
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernetes-mcp-server
spec:
  replicas: 3
  selector:
    matchLabels:
      app: kubernetes-mcp-server
  template:
    metadata:
      labels:
        app: kubernetes-mcp-server
    spec:
      containers:
      - name: server
        image: quay.io/containers/kubernetes_mcp_server:latest
        args:
        - --port=8080
        - --kubeconfig=/kubeconfig/config
        - --cluster-provider-strategy=auth-headers
        - --validate-token
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: kubeconfig
          mountPath: /kubeconfig
          readOnly: true
      volumes:
      - name: kubeconfig
        configMap:
          name: cluster-kubeconfig
---
apiVersion: v1
kind: Service
metadata:
  name: kubernetes-mcp-server
spec:
  selector:
    app: kubernetes-mcp-server
  ports:
  - port: 80
    targetPort: 8080
```

## Troubleshooting

### Error: "bearer token required in Authorization header"

**Cause**: Request missing authentication header

**Solution**: Include `Authorization: Bearer <token>` header in all requests

### Error: "auth-headers ClusterProviderStrategy cannot be used in in-cluster deployments"

**Cause**: Trying to use auth-headers provider from within a Kubernetes cluster

**Solution**: Use `in-cluster` or `disabled` strategy for in-cluster deployments, or explicitly set a kubeconfig path

### Error: "token-based authentication required"

**Cause**: `RequireOAuth` is enabled but no token provided

**Solution**: Ensure client sends bearer token in Authorization header

### Warning: "auth-headers ClusterProviderStrategy requires OAuth authentication, enabling RequireOAuth"

**Info**: This is expected - auth-headers provider automatically enables OAuth requirement

## Related Documentation

- [OIDC/OAuth Setup Guide](./KEYCLOAK_OIDC_SETUP.md)
- [Getting Started](./GETTING_STARTED_KUBERNETES.md)
- [Claude Integration](./GETTING_STARTED_CLAUDE_CODE.md)

