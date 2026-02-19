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

The validation layer catches these types of issues:

1. **Resource Existence** - Catches typos like "Deploymnt" instead of "Deployment" (checked in access control)
2. **Schema Validation** - Catches invalid fields like "spec.replcias" instead of "spec.replicas"
3. **RBAC Validation** - Pre-checks permissions before attempting operations

## Configuration

Validation is **disabled by default**. Schema and RBAC validators run together when enabled. Resource existence is always checked as part of access control.

```toml
# Enable all validation (default: false)
validation_enabled = true
```

### Configuration Reference

| TOML Field | Default | Description |
|------------|---------|-------------|
| `validation_enabled` | `false` | Enable/disable all validators |

**Note:** The schema validator caches the OpenAPI schema for 15 minutes internally.

## How It Works

### Validation Flow

Validation happens at the HTTP RoundTripper level, intercepting all Kubernetes API calls:

```
MCP Tool Call → Kubernetes Client → HTTP RoundTripper → Kubernetes API
                                          ↓
                                   Access Control
                                   - Check deny list
                                   - Check resource exists
                                          ↓
                                   Schema Validator (if enabled)
                                   "Are the fields valid?"
                                          ↓
                                   RBAC Validator (if enabled)
                                   "Does the user have permission?"
                                          ↓
                                   Forward to K8s API
```

This HTTP-layer approach ensures **all** Kubernetes API calls are validated, including those from plugins (KubeVirt, Kiali, Helm, etc.) - not just the core tools.

If any validator fails, the request is rejected with a clear error message before reaching the Kubernetes API.

### 1. Resource Existence (Access Control)

The access control layer validates that the requested resource type exists in the cluster. This check runs regardless of whether validation is enabled.

**What it catches:**
- Typos in Kind names: "Deploymnt" → should be "Deployment"
- Wrong API versions: "apps/v2" → should be "apps/v1"
- Non-existent custom resources

**Example error:**
```
RESOURCE_NOT_FOUND: Resource deployments.apps does not exist in the cluster
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
| `PERMISSION_DENIED` | RBAC denies the requested operation |
