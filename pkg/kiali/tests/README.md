# Kiali API Backend Tests

This directory contains test files. 

The file api_backend_test.go contains contract tests for the Kiali API endpoints used by the `kubernetes-mcp-server`. 
These tests validate that the API contract/interface remains stable and that all endpoints return expected response structures.
Are excluded by tests run by default. 

## Overview

Contract tests ensure that:
- All Kiali API endpoints used by the MCP server return expected status codes
- Response structures match expected types
- Required fields are present in responses
- The API contract remains stable across Kiali versions

## Prerequisites

- A running Kiali instance
- Go 1.24 or later
- Access to a Kubernetes cluster with Kiali installed (or a test environment)

## Configuration

Tests are configured via environment variables:

### Required

- `KIALI_URL` - Base URL of the Kiali instance (default: `http://localhost:20001/kiali`)
  ```bash
  export KIALI_URL="http://localhost:20001/kiali"
  ```

### Optional

- `KIALI_TOKEN` - Bearer token for authentication (if required)
  ```bash
  export KIALI_TOKEN="your-token-here"
  ```

- `TEST_NAMESPACE` - Namespace to use for tests (default: `bookinfo`)
  ```bash
  export TEST_NAMESPACE="bookinfo"
  ```

- `TEST_SERVICE` - Service name to use for tests (default: `productpage`)
  ```bash
  export TEST_SERVICE="productpage"
  ```

- `TEST_WORKLOAD` - Workload name to use for tests (default: `productpage-v1`)
  ```bash
  export TEST_WORKLOAD="productpage-v1"
  ```

- `TEST_APP` - Application name to use for tests (default: `productpage`)
  ```bash
  export TEST_APP="productpage"
  ```

- `TEST_POD` - Pod name to use for tests (optional, will be extracted from workload if not set)
  ```bash
  export TEST_POD="productpage-v1-54bb874995-4gvgd"
  ```

## Running Tests

### Run all contract tests

```bash
go test ./pkg/kiali/tests/backend/api_backend_test.go -v
```

### Run from the test directory

```bash
cd pkg/kiali/tests/backend/api_backend_test.go
go test -v
```

### Run a specific test

```bash
go test ./pkg/kiali/tests/backend/api_backend_test.go -v -run TestAuthInfo
```

### Run with verbose output

```bash
go test ./pkg/kiali/tests/backend/api_backend_test.go -v -count=1
```

## Test Structure

Tests are organized using the `testify/suite` pattern:

- **ContractTestSuite** - Main test suite that contains all contract tests
- Each test method validates a specific Kiali API endpoint
- Tests use the endpoint constants from `pkg/kiali/endpoints.go` to ensure consistency

### Test Coverage

The tests cover all Kiali API endpoints used by the MCP server:

- **Authentication**: `/api/auth/info`
- **Namespaces**: `/api/namespaces`
- **Graph**: `/api/mesh/graph`, `/api/namespaces/graph`
- **Health**: `/api/clusters/health`
- **Istio Config**: `/api/istio/config`, `/api/istio/validations`
- **Services**: `/api/clusters/services`, `/api/namespaces/{namespace}/services/{service}`, `/api/namespaces/{namespace}/services/{service}/metrics`
- **Workloads**: `/api/clusters/workloads`, `/api/namespaces/{namespace}/workloads/{workload}`, `/api/namespaces/{namespace}/workloads/{workload}/metrics`
- **Pods**: `/api/namespaces/{namespace}/pods/{pod}`, `/api/namespaces/{namespace}/pods/{pod}/logs`
- **Traces**: `/api/namespaces/{namespace}/apps/{app}/traces`, `/api/namespaces/{namespace}/services/{service}/traces`, `/api/namespaces/{namespace}/workloads/{workload}/traces`, `/api/traces/{traceId}`
- **Istio Objects**: CRUD operations for Istio configuration objects

## Dynamic Test Data

Some tests automatically extract test data from API responses:

- **Pod names**: Extracted from workload details if `TEST_POD` is not set
- **Trace IDs**: Extracted from traces list responses for trace detail tests
- **ServiceEntry names**: Created dynamically for Istio object CRUD tests

## Adding New Tests

When adding a new endpoint to `pkg/kiali/endpoints.go`, you must add a corresponding test:

1. Add the endpoint constant to `pkg/kiali/endpoints.go`
2. Add a test method in `pkg/kiali/tests/backend/api_backend_test.go` that uses the endpoint constant
3. The pre-commit hook will validate that all endpoints have tests (static check)

Example:

```go
func (s *ContractTestSuite) TestNewEndpoint() {
    s.Run("returns expected structure", func() {
        endpoint := kiali.NewEndpoint
        resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
        s.Require().NoError(err)
        s.Equal(http.StatusOK, resp.StatusCode)
        
        var data map[string]interface{}
        err = json.Unmarshal(body, &data)
        s.NoError(err)
        s.Contains(data, "expectedField")
    })
}
```

## Validation

The endpoints_converage_test validates that all endpoints have corresponding tests.

To manually run it:

```bash
cd pkg/kiali/tests/backend
go test api_backend_test.go -v
```

## Troubleshooting

### Tests fail with 404 errors

- Verify that `KIALI_URL` is correct and Kiali is running
- Check that the test namespace exists in your cluster
- Ensure test resources (services, workloads, pods) exist

### Tests fail with authentication errors

- Set `KIALI_TOKEN` if your Kiali instance requires authentication
- Verify the token has necessary permissions

### Tests timeout

- Some tests wait for resources to be available (e.g., Istio objects)
- Increase timeout if your cluster is slow to propagate changes
- Check network connectivity to Kiali

### Pod tests are skipped

- Ensure `TEST_WORKLOAD` is set to a workload that has running pods
- Or set `TEST_POD` directly to a pod name

## Integration with CI/CD

These tests can be integrated into CI/CD pipelines:

```yaml
# Example GitHub Actions workflow
- name: Run Kiali Contract Tests
  env:
    KIALI_URL: ${{ secrets.KIALI_URL }}
    KIALI_TOKEN: ${{ secrets.KIALI_TOKEN }}
    TEST_NAMESPACE: bookinfo
  run: go test ./pkg/kiali/tests -v
```

## Related Documentation

- [Kiali API Documentation](https://kiali.io/documentation/latest/api/)
- [Main Project README](../../../../README.md)
- [Testing Guidelines](../../../../AGENTS.md#tests)

