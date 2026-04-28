# Kiali MCP Contract Tests

This directory contains contract tests for the Kiali MCP tool endpoints used by the `kubernetes-mcp-server`.

## Overview

After the v2.25 refactor, the kubernetes-mcp-server delegates all Kiali tool calls to
Kiali's backend via `POST /api/chat/mcp/<tool>`. The contract tests in
`backend/api_backend_test.go` verify that every MCP endpoint constant defined in
`pkg/toolsets/kiali/tools/endpoints.go` is registered and responsive on a live Kiali instance.

Contract tests ensure that:
- All `/api/chat/mcp/<tool>` endpoints return non-404 status codes (i.e. they are registered)
- The API contract remains stable across Kiali versions
- Tracing endpoints (`list_traces`, `get_trace_details`) correctly return 404 when tracing is disabled

## Prerequisites

- A running Kiali instance (with bookinfo demo app deployed)
- Go 1.24 or later

## Configuration

Tests are configured via environment variables:

### Required

- `KIALI_URL` - Base URL of the Kiali instance (default: `http://localhost:20001/kiali`)

### Optional

- `KIALI_TOKEN` - Bearer token for authentication (if required)
- `TEST_NAMESPACE` - Namespace to use for tests (default: `bookinfo`)

## Running Tests

The tests use the `kiali_contract` build tag and are excluded from default test runs.

```bash
cd pkg/kiali/tests/backend
go test -tags kiali_contract -v ./...
```

### Run a specific test

```bash
go test -tags kiali_contract -v -run TestContract/TestGetMeshStatus ./...
```

## Test Structure

Tests are organized using the `testify/suite` pattern:

- **ContractTestSuite** - Main test suite that POSTs to each `/api/chat/mcp/<tool>` endpoint
- Each test method validates a specific MCP tool endpoint using the constants from `pkg/toolsets/kiali/tools/endpoints.go`

### Endpoint Coverage

The `endpoints_coverage_test.go` in the parent directory validates that every endpoint
constant in `endpoints.go` is referenced by at least one tool implementation.

## Integration with Kiali CI

These tests are executed by Kiali's CI via the `mcp-contract-tests.yml` workflow,
which checks out this repository and runs the contract tests against a live Kiali
instance on a KinD cluster.
