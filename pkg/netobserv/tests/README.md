# NetObserv MCP Contract Tests

Contract tests verify NetObserv console plugin API endpoints work with MCP server.

## Prerequisites

- A running NetObserv console plugin instance
- Go 1.24 or later

## Configuration

Via environment variables:

- `NETOBSERV_URL` - Plugin base URL (default: `http://localhost:9001`)
- `NETOBSERV_TOKEN` - Bearer token if auth required
- `TEST_NAMESPACE` - Namespace for queries (default: `default`)

## Running

```bash
# Against mock plugin
kubectl apply -f ../../../evals/tasks/netobserv/shared/mock-plugin.yaml
kubectl port-forward svc/netobserv-mock 9001:9001

NETOBSERV_URL=http://localhost:9001 go test -tags netobserv_contract -v ./backend/

# Against real plugin (requires NetObserv operator deployed)
oc port-forward -n netobserv svc/netobserv-plugin 9001:9001

NETOBSERV_URL=http://localhost:9001 go test -tags netobserv_contract -v ./backend/
```

## Test Structure

- **ContractTestSuite** - Main suite using testify/suite
- Each test validates one API endpoint
- Tests ensure endpoints return non-404 (registered)
- Schema validation for successful responses

## Integration with CI

Contract tests run in kubernetes-mcp-server CI to catch plugin API regressions.

## What These Tests Cover

✅ **API Endpoints:**
- `/api/loki/flow/records` - List flows endpoint registration
- `/api/flow/metrics` - Get metrics endpoint registration
- `/api/loki/export` - Export flows endpoint registration
- `/api/status` - Plugin status endpoint

✅ **Response Validation:**
- HTTP status codes (200, 400, 403, 404)
- Response structure (JSON/CSV format)
- Error handling

## What These Tests Don't Cover

❌ **Data Plane:** FlowCollector reconciliation, eBPF correctness (netobserv-operator tests)
❌ **Console UI:** Dashboard panels, topology widgets (netobserv-web-console Cypress tests)
❌ **MCP Tools:** Tool contracts and parameters (test/e2e/ tests)
❌ **LLM Evals:** Agent task completion (rhobs/troubleshooting-scenarios evals)
