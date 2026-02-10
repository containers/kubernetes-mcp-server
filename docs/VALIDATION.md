# Pre-Execution Validation

The kubernetes-mcp-server includes a validation layer that catches errors before they reach the Kubernetes API. This prevents AI hallucinations (like typos in resource names) and permission issues from causing confusing failures.

## Why Validation?

When an AI assistant makes a Kubernetes API call with errors, the raw Kubernetes error messages can be cryptic:

```
the server doesn't have a resource type "Deploymnt"
```

With validation enabled, you get clearer feedback:

```
Resource apps/v1/Deploymnt does not exist in the cluster
```

The validation layer catches three types of issues:

1. **Resource Validation** - Catches typos like "Deploymnt" instead of "Deployment"
2. **Schema Validation** - Catches invalid fields like "spec.replcias" instead of "spec.replicas"
3. **RBAC Validation** - Pre-checks permissions before attempting operations

## Configuration

Validation is **disabled by default**. All validators (resource, schema, RBAC) run together when enabled.

### Environment Variables

```bash
# Enable all validation
MCP_VALIDATION_ENABLED=true
```

### TOML Configuration

Add a `[validation]` section to your config file:

```toml
[validation]
# Enable all validation (default: false)
enabled = true
```

### Configuration Reference

| TOML Field | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `enabled` | `MCP_VALIDATION_ENABLED` | `false` | Enable/disable all validators |

Environment variables take precedence over TOML config.

**Note:** The schema validator caches the OpenAPI schema for 15 minutes internally.

## How It Works

### Validation Flow

Validation happens at the HTTP RoundTripper level, intercepting all Kubernetes API calls:

```
MCP Tool Call → Kubernetes Client → HTTP RoundTripper → Kubernetes API
                                          ↓
                                   Access Control (deny list)
                                          ↓
                                   Resource Validator
                                   "Does this GVK exist?"
                                          ↓
                                   Schema Validator
                                   "Are the fields valid?"
                                          ↓
                                   RBAC Validator
                                   "Does the user have permission?"
                                          ↓
                                   Forward to K8s API
```

This HTTP-layer approach ensures **all** Kubernetes API calls are validated, including those from plugins (KubeVirt, Kiali, Helm, etc.) - not just the core tools.

If any validator fails, the request is rejected with a clear error message before reaching the Kubernetes API.

### 1. Resource Validation

Validates that the requested resource type (Group/Version/Kind) exists in the cluster.

**What it catches:**
- Typos in Kind names: "Deploymnt" → should be "Deployment"
- Wrong API versions: "apps/v2" → should be "apps/v1"
- Non-existent custom resources

**Example error:**
```
RESOURCE_NOT_FOUND: Resource apps/v1/Deploymnt does not exist in the cluster
```

### 2. Schema Validation

Validates resource manifests against the cluster's OpenAPI schema for create/update operations.

**What it catches:**
- Invalid field names: "spec.replcias" → should be "spec.replicas"
- Wrong field types: string where integer expected
- Missing required fields

**Example error:**
```
INVALID_FIELD: unknown field "spec.replcias"
```

**Note:** Schema validation uses kubectl's validation library and caches the OpenAPI schema for 15 minutes.

### 3. RBAC Validation

Pre-checks permissions using Kubernetes `SelfSubjectAccessReview` before attempting operations.

**What it catches:**
- Missing permissions: can't create Deployments in namespace X
- Cluster-scoped vs namespace-scoped mismatches
- Read-only access attempting writes

**Example error:**
```
PERMISSION_DENIED: Cannot create deployments.apps in namespace "production"
```

**Note:** RBAC validation uses the same credentials as the actual operation - either the server's service account or the user's token (when OAuth is enabled).

## Error Codes

| Code | Description |
|------|-------------|
| `RESOURCE_NOT_FOUND` | The requested resource type doesn't exist in the cluster |
| `INVALID_FIELD` | A field in the manifest doesn't exist or has the wrong type |
| `INVALID_MANIFEST` | The manifest is malformed (invalid YAML/JSON) |
| `PERMISSION_DENIED` | RBAC denies the requested operation |
