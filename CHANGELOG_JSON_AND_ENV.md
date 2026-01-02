# JSON Output Format & Environment Variable Configuration

## Summary

Added JSON output format support and environment variable configuration to make the server compatible with agentic frameworks and container deployments.

## Changes

### 1. JSON Output Format

**Added:** New `json` output format alongside existing `yaml` and `table` formats.

**Files Modified:**
- `pkg/output/output.go` - Added JSON marshaling with proper indentation
- `pkg/output/output_test.go` - Added unit tests
- `pkg/mcp/*_test.go` - Added integration tests for pods, namespaces, and resources

**Usage:**
```bash
# Command line
kubernetes-mcp-server --list-output=json

# Environment variable
export MCP_LIST_OUTPUT=json
kubernetes-mcp-server

# Docker
docker run -e MCP_LIST_OUTPUT=json ...
```

### 2. Environment Variable Configuration

**Added:** All configuration options now support environment variables with `MCP_` prefix.

**Files Modified:**
- `pkg/kubernetes-mcp-server/cmd/root.go` - Added `loadEnvironmentVariables()` function
- `pkg/kubernetes-mcp-server/cmd/root_test.go` - Added environment variable tests

**Supported Variables:**
- `MCP_PORT` - Server port
- `MCP_LIST_OUTPUT` - Output format (yaml/json/table)
- `MCP_LOG_LEVEL` - Logging level
- `MCP_KUBECONFIG` - Kubeconfig path
- `MCP_READ_ONLY` - Read-only mode (true/false)
- `MCP_DISABLE_DESTRUCTIVE` - Disable destructive operations
- `MCP_STATELESS` - Stateless mode
- `MCP_TOOLSETS` - Comma-separated toolsets
- `MCP_DISABLE_MULTI_CLUSTER` - Disable multi-cluster
- Plus OAuth and other configuration options

**Configuration Precedence:**
1. Command-line flags (highest)
2. Environment variables
3. Configuration file
4. Default values (lowest)

## Container Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernetes-mcp-server
spec:
  template:
    spec:
      containers:
      - name: mcp-server
        image: quay.io/containers/kubernetes_mcp_server:latest
        env:
        - name: MCP_PORT
          value: "8080"
        - name: MCP_LIST_OUTPUT
          value: "json"
        - name: MCP_STATELESS
          value: "true"
```

## Documentation Updates

- `README.md` - Updated `--list-output` description and added Environment Variables section

## Testing

All tests pass with no breaking changes:
```bash
go test ./pkg/output/... -v
go test ./pkg/mcp/... -v -run "AsJson"
go test ./pkg/kubernetes-mcp-server/cmd/... -v -run TestEnvironmentVariables
```

## How this change helps

1. **JSON Format:** Compatible with agentic frameworks that prefer JSON over YAML
2. **Environment Variables:** Standard configuration method for containers
3. **No Breaking Changes:** All existing functionality preserved
4. **Automatic Support:** All tools work with JSON without code changes

