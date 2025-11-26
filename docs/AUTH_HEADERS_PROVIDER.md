# Auth-Headers Provider

The `auth-headers` cluster provider strategy enables multi-tenant Kubernetes MCP server deployments where each request provides complete cluster connection details and authentication via HTTP headers or MCP tool parameters.

## Overview

This provider:
- **Requires cluster connection details per request** via custom headers (server URL, CA certificate)
- **Requires authentication per request** via bearer token OR client certificates
- **Does not use kubeconfig** - all configuration comes from request headers
- **Creates dynamic Kubernetes clients** per request using the provided credentials

## Use Cases

- **Multi-tenant SaaS deployments** - Single MCP server instance serving multiple users/clusters
- **Zero-trust architectures** - No stored credentials, complete authentication per request
- **Dynamic cluster access** - Connect to different clusters without server configuration
- **Auditing & compliance** - Each request uses the user's actual identity for Kubernetes RBAC
- **Temporary access** - Short-lived credentials without persistent configuration

## Configuration

### Basic Setup

```bash
kubernetes-mcp-server \
  --port 8080 \
  --cluster-provider-strategy auth-headers
```

The server will:
1. Accept requests with cluster connection details in headers
2. Create a Kubernetes client dynamically for each request
3. Reject any requests without required authentication headers

### TOML Configuration

```toml
cluster_provider_strategy = "auth-headers"
# No kubeconfig needed - all details come from request headers
```

### Required Headers

Each request must include the following custom headers:

**Required for all requests:**
- `kubernetes-server` - Kubernetes API server URL (e.g., `https://kubernetes.example.com:6443`)
- `kubernetes-certificate-authority-data` - Base64-encoded CA certificate

**Authentication (choose one):**

Option 1: Bearer Token
- `kubernetes-authorization` - Bearer token (e.g., `Bearer eyJhbGci...`)

Option 2: Client Certificate
- `kubernetes-client-certificate-data` - Base64-encoded client certificate
- `kubernetes-client-key-data` - Base64-encoded client key

**Optional:**
- `kubernetes-insecure-skip-tls-verify` - Set to `true` to skip TLS verification (not recommended for production)

## How It Works

### 1. Initialization

When the server starts:
```
Server starts with auth-headers provider
         ↓
No kubeconfig or credentials loaded
         ↓
Ready to accept requests with headers
```

### 2. Request Processing

For each MCP request:
```
HTTP Request with custom headers
         ↓
Extract kubernetes-server, kubernetes-certificate-authority-data
         ↓
Extract authentication (token OR client cert/key)
         ↓
Create K8sAuthHeaders struct
         ↓
Build rest.Config dynamically
         ↓
Create new Kubernetes client
         ↓
Execute Kubernetes operation
         ↓
Discard client after request
```

### 3. Header Extraction

Headers can be provided in two ways:

**A. HTTP Request Headers** (standard way):
```
POST /mcp HTTP/1.1
kubernetes-server: https://k8s.example.com:6443
kubernetes-certificate-authority-data: LS0tLS1CRUdJ...
kubernetes-authorization: Bearer eyJhbGci...
```

**B. MCP Tool Parameters Meta** (advanced):
```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "pods_list",
    "arguments": {"namespace": "default"},
    "_meta": {
      "kubernetes-server": "https://k8s.example.com:6443",
      "kubernetes-certificate-authority-data": "LS0tLS1CRUdJ...",
      "kubernetes-authorization": "Bearer eyJhbGci..."
    }
  }
}
```

### 4. Security Model

```
┌──────────────────┐
│   MCP Client     │
│  (Claude, etc)   │
└────────┬─────────┘
         │ All cluster info + auth in headers
         ↓
┌──────────────────┐
│   MCP Server     │
│  (auth-headers)  │
│  NO CREDENTIALS  │
│     STORED       │
└────────┬─────────┘
         │ Creates temporary client
         ↓
┌──────────────────┐
│  Kubernetes API  │
│     Server       │
└──────────────────┘
         ↓
    RBAC enforced with
    credentials from headers
```

## Client Usage

### Using the Go MCP Client

```go
import (
    "encoding/base64"
    "github.com/mark3labs/mcp-go/client/transport"
)

// Get cluster connection details
serverURL := "https://k8s.example.com:6443"
caCert := getCAcertificate() // PEM-encoded CA certificate
token := getUserKubernetesToken()

// Encode CA certificate to base64
caCertBase64 := base64.StdEncoding.EncodeToString(caCert)

client := NewMCPClient(
    transport.WithHTTPHeaders(map[string]string{
        "kubernetes-server":                      serverURL,
        "kubernetes-certificate-authority-data":  caCertBase64,
        "kubernetes-authorization":               "Bearer " + token,
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
        "kubernetes-server": "https://k8s.example.com:6443",
        "kubernetes-certificate-authority-data": "LS0tLS1CRUdJTi...",
        "kubernetes-authorization": "Bearer YOUR_KUBERNETES_TOKEN"
      }
    }
  }
}
```

### Using Client Certificates

```json
{
  "mcpServers": {
    "kubernetes": {
      "url": "https://mcp-server.example.com/sse",
      "headers": {
        "kubernetes-server": "https://k8s.example.com:6443",
        "kubernetes-certificate-authority-data": "LS0tLS1CRUdJTi...",
        "kubernetes-client-certificate-data": "LS0tLS1CRUdJTi...",
        "kubernetes-client-key-data": "LS0tLS1CRUdJTi..."
      }
    }
  }
}
```

### Using cURL

```bash
# With bearer token
curl -X POST https://mcp-server.example.com/mcp \
  -H "Content-Type: application/json" \
  -H "kubernetes-server: https://k8s.example.com:6443" \
  -H "kubernetes-certificate-authority-data: LS0tLS1CRUdJTi..." \
  -H "kubernetes-authorization: Bearer eyJhbGci..." \
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

### Getting Required Values

#### 1. Kubernetes Server URL

```bash
kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}'
```

#### 2. CA Certificate (base64)

```bash
kubectl config view --minify --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}'
```

#### 3. Bearer Token

```bash
# From current context
kubectl config view --minify --raw -o jsonpath='{.users[0].user.token}'

# Or get a service account token
kubectl create token <service-account-name> -n <namespace>
```

#### 4. Client Certificate (base64)

```bash
kubectl config view --minify --raw -o jsonpath='{.users[0].user.client-certificate-data}'
```

#### 5. Client Key (base64)

```bash
kubectl config view --minify --raw -o jsonpath='{.users[0].user.client-key-data}'
```
