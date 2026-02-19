# MCP Logging

The server supports the MCP logging capability, allowing clients to receive debugging information via structured log messages.

## For Clients

Clients can control log verbosity by sending a `logging/setLevel` request:

```json
{
  "method": "logging/setLevel",
  "params": { "level": "info" }
}
```

**Available log levels** (in order of increasing severity):
- `debug` - Detailed debugging information
- `info` - General informational messages (default)
- `notice` - Normal but significant events
- `warning` - Warning messages
- `error` - Error conditions
- `critical` - Critical conditions
- `alert` - Action must be taken immediately
- `emergency` - System is unusable

## For Developers

### Automatic Kubernetes Error Logging

Kubernetes API errors returned by tool handlers are **automatically logged** to MCP clients.
When a tool handler returns a `ToolCallResult` with a non-nil error that is a Kubernetes API error (`StatusError`), the server categorizes it and sends an appropriate log message.

This means toolset authors **do not need to call any logging functions** for standard K8s error handling.
Simply return the error in the `ToolCallResult` and the server handles the rest:

```go
ret, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
if err != nil {
    return api.NewToolCallResult("", fmt.Errorf("failed to get pod: %w", err)), nil
}
```

The following Kubernetes error types are automatically categorized:

| Error Type | Log Level | Message |
|-----------|-----------|---------|
| Not Found | `info` | Resource not found - it may not exist or may have been deleted |
| Forbidden | `error` | Permission denied - check RBAC permissions for {tool} |
| Unauthorized | `error` | Authentication failed - check cluster credentials |
| Already Exists | `warning` | Resource already exists |
| Invalid | `error` | Invalid resource specification - check resource definition |
| Bad Request | `error` | Invalid request - check parameters |
| Conflict | `error` | Resource conflict - resource may have been modified |
| Timeout | `error` | Request timeout - cluster may be slow or overloaded |
| Server Timeout | `error` | Server timeout - cluster may be slow or overloaded |
| Service Unavailable | `error` | Service unavailable - cluster may be unreachable |
| Too Many Requests | `warning` | Rate limited - too many requests to the cluster |
| Other K8s API errors | `error` | Operation failed - cluster may be unreachable or experiencing issues |

Non-Kubernetes errors (e.g., input validation errors) are **not** logged to MCP clients.

### Manual Logging

For custom messages beyond automatic K8s error handling, use `SendMCPLog` directly:

```go
import "github.com/containers/kubernetes-mcp-server/pkg/mcplog"

mcplog.SendMCPLog(ctx, mcplog.LevelError, "Operation failed - check permissions")
```

## Security

- Authentication failures send generic messages to clients (no security info leaked)
- Sensitive data is automatically redacted before being sent to clients, covering:
  - Generic fields (password, token, secret, api_key, etc.)
  - Authorization headers (Bearer, Basic)
  - Cloud credentials (AWS, GCP, Azure)
  - API tokens (GitHub, GitLab, OpenAI, Anthropic)
  - Cryptographic keys (JWT, SSH, PGP, RSA)
  - Database connection strings (PostgreSQL, MySQL, MongoDB)
- Uses a dedicated named logger (`logger="mcp"`) for complete separation from server logs
- Server logs (klog) remain detailed and unaffected
