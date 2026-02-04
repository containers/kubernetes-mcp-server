# OpenTelemetry Observability

The kubernetes-mcp-server supports distributed tracing and metrics via OpenTelemetry (OTEL). Observability is **optional** and disabled by default.

## What Gets Traced

The server automatically traces all operations through middleware without requiring any code changes to individual tools:

1. **MCP Tool Calls** - Every tool invocation with details:
   - Tool name
   - Success/failure status
   - Duration
   - Error details (when applicable)

2. **HTTP Requests** - All HTTP endpoints when running in HTTP mode:
   - Request method and path
   - Response status
   - Client information
   - Duration

**Note**: When running in STDIO mode only MCP tool calls are traced since there is no HTTP server.

## Metrics

The server collects and exposes metrics through two mechanisms:

1. **Stats Endpoint** (`/stats`) - JSON endpoint for real-time statistics:
   - Tool call counts by name
   - Tool call errors
   - HTTP request counts by method/path/status
   - Server uptime

2. **OTLP Export** - When an endpoint is configured, metrics are also exported to your OTLP backend every 30 seconds.

## Quick Start

### 1. Run an OTLP Backend Locally

**Option A: Jaeger (traces only)**

```bash
docker run -d --name jaeger \
  -e COLLECTOR_OTLP_ENABLED=true \
  -p 16686:16686 \
  -p 4317:4317 \
  -p 4318:4318 \
  docker.io/jaegertracing/all-in-one:latest
```

Access the Jaeger UI at http://localhost:16686

> **Note**: Jaeger only supports traces, not metrics. To disable metrics export and avoid warnings about `MetricsService` being unimplemented, set `OTEL_METRICS_EXPORTER=none`.

**Option B: Grafana LGTM Stack (traces + metrics + logs)**

For full observability with metrics support:

```bash
docker run -d --name lgtm \
  -p 3000:3000 \
  -p 4317:4317 \
  -p 4318:4318 \
  docker.io/grafana/otel-lgtm:latest
```

Access Grafana at http://localhost:3000 (default credentials: admin/admin)

### 2. Enable Tracing

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317

# Run the server
npx -y kubernetes-mcp-server@latest
```

### 3. View Traces

Make some tool calls through your MCP client, then view traces in the Jaeger UI.

### Example Trace

When you call `resources_get` for a Pod, you'll see a trace like this in Jaeger:

```
Trace ID: abc123def456789
Duration: 145ms

└─ tools/call resources_get [145ms]
   ├─ mcp.method.name: tools/call
   ├─ gen_ai.tool.name: resources_get
   ├─ gen_ai.operation.name: execute_tool
   ├─ rpc.jsonrpc.version: 2.0
   ├─ network.transport: pipe
   └─ Status: OK
```

If the tool call triggers an HTTP request (in HTTP mode), you'll also see:

```
Trace ID: abc123def456789
Duration: 150ms

├─ POST /message [150ms]
│  ├─ http.request.method: POST
│  ├─ url.path: /message
│  ├─ http.response.status_code: 200
│  ├─ client.address: 192.168.1.100
│  │
│  └─ tools/call resources_get [145ms]
       ├─ mcp.method.name: tools/call
       ├─ gen_ai.tool.name: resources_get
       ├─ gen_ai.operation.name: execute_tool
       ├─ rpc.jsonrpc.version: 2.0
       ├─ network.transport: tcp
       └─ Status: OK
```

## Configuration

OpenTelemetry can be configured via **TOML config file** or **environment variables**. Environment variables take precedence over TOML config values.

**Note**: Telemetry is automatically enabled when an endpoint is configured. Use `enabled = false` in TOML to explicitly disable it.

### Configuration Reference

| TOML Field | Environment Variable | Description |
|------------|---------------------|-------------|
| `enabled` | - | Explicit enable/disable (overrides all) |
| `endpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint URL |
| `protocol` | `OTEL_EXPORTER_OTLP_PROTOCOL` | Protocol: `grpc` or `http/protobuf` |
| `traces_sampler` | `OTEL_TRACES_SAMPLER` | Sampling strategy |
| `traces_sampler_arg` | `OTEL_TRACES_SAMPLER_ARG` | Sampling ratio (0.0-1.0) |

### TOML Configuration

Add a `[telemetry]` section to your config file:

```toml
[telemetry]
# Optional: explicitly enable/disable (omit to auto-enable when endpoint is set)
enabled = true

endpoint = "http://localhost:4317"

# Protocol: "grpc" (default) or "http/protobuf"
protocol = "grpc"

# Trace sampling strategy
# Options: "always_on", "always_off", "traceidratio", "parentbased_always_on", "parentbased_always_off", "parentbased_traceidratio"
traces_sampler = "traceidratio"

# Sampling ratio for ratio-based samplers (0.0 to 1.0)
traces_sampler_arg = 0.1
```

#### TOML Examples

**Enable with endpoint:**
```toml
[telemetry]
endpoint = "http://localhost:4317"
```

**Production with sampling:**
```toml
[telemetry]
endpoint = "http://tempo-distributor:4317"
traces_sampler = "traceidratio"
traces_sampler_arg = 0.05  # 5% sampling
```

**Explicitly disable:**
```toml
[telemetry]
enabled = false
```

### Environment Variables

Environment variables take precedence over TOML config. This allows you to override config file settings at runtime.

#### Endpoint

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
```

**Note**: The server gracefully handles failures. If the endpoint is unreachable, the server logs a warning and continues without tracing.

#### Optional Variables

```bash
# Service name (defaults to "kubernetes-mcp-server")
export OTEL_SERVICE_NAME=kubernetes-mcp-server

# Service version (auto-detected from binary, rarely needs manual override)
export OTEL_SERVICE_VERSION=1.0.0

# Additional resource attributes (useful for multi-environment deployments)
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production,team=platform"
```

#### Endpoint Protocols

The server supports both gRPC and HTTP/protobuf protocols:

```bash
# gRPC (default, port 4317)
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317

# HTTP/protobuf (port 4318)
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf

# Secure endpoints (HTTPS/gRPC with TLS)
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp-secure.example.com:4317

# Custom CA certificate (for self-signed certificates)
export OTEL_EXPORTER_OTLP_CERTIFICATE=/path/to/ca.crt
```

#### Sampling Configuration

By default, the server uses **`ParentBased(AlwaysSample)`** sampling:
- **Root spans** (no parent): Always sampled (100%)
- **Child spans**: Inherit parent's sampling decision

This is ideal for development but may generate high trace volumes in production.

#### Production Sampling

For production with high traffic, use ratio-based sampling:

```bash
# Sample 10% of traces
export OTEL_TRACES_SAMPLER=traceidratio
export OTEL_TRACES_SAMPLER_ARG=0.1
```

#### Available Samplers

- `always_on` - Sample everything (default for root spans)
- `always_off` - Disable tracing entirely
- `traceidratio` - Sample a percentage (requires `OTEL_TRACES_SAMPLER_ARG` between 0.0 and 1.0)
- `parentbased_always_on` - Respect parent span, default to always_on
- `parentbased_always_off` - Respect parent span, default to always_off
- `parentbased_traceidratio` - Respect parent span, default to ratio

#### Sampling Examples

```bash
# Development: Sample everything
export OTEL_TRACES_SAMPLER=always_on

# Production: 5% sampling (good for high-traffic services)
export OTEL_TRACES_SAMPLER=traceidratio
export OTEL_TRACES_SAMPLER_ARG=0.05

# Temporarily disable tracing
export OTEL_TRACES_SAMPLER=always_off

# Or just unset the endpoint
unset OTEL_EXPORTER_OTLP_ENDPOINT
```

## Deployment Examples

### Claude Code (STDIO Mode)

Add the MCP server to your project's `.mcp.json` or global `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "kubernetes": {
      "command": "npx",
      "args": ["-y", "kubernetes-mcp-server@latest"],
      "env": {
        "OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317",
        "OTEL_TRACES_SAMPLER": "always_on"
      }
    }
  }
}
```

**For Jaeger (traces only)**: Add `"OTEL_METRICS_EXPORTER": "none"` to disable metrics export.

**Note**: In STDIO mode, only MCP tool calls are traced (no HTTP request spans).

### Kubernetes Deployment (HTTP Mode)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernetes-mcp-server
spec:
  template:
    spec:
      containers:
      - name: kubernetes-mcp-server
        image: quay.io/containers/kubernetes_mcp_server:latest
        env:
        # OTLP endpoint (required to enable tracing)
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://tempo-distributor.observability:4317"

        # Sampling (recommended for production)
        - name: OTEL_TRACES_SAMPLER
          value: "traceidratio"
        - name: OTEL_TRACES_SAMPLER_ARG
          value: "0.1"  # 10% sampling

        # Resource attributes (helps identify this deployment)
        - name: OTEL_RESOURCE_ATTRIBUTES
          value: "deployment.environment=production,k8s.cluster.name=prod-us-west-2"

        # Kubernetes metadata (optional, helps correlate traces with K8s resources)
        - name: KUBERNETES_POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: KUBERNETES_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: KUBERNETES_NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
```

**Note**: The Kubernetes metadata environment variables are optional but recommended for production deployments. They help correlate traces with specific pods, namespaces, and nodes.

### Docker

```bash
docker run \
  -e OTEL_EXPORTER_OTLP_ENDPOINT=http://host.docker.internal:4317 \
  -e OTEL_TRACES_SAMPLER=always_on \
  quay.io/containers/kubernetes_mcp_server:latest
```

## Trace Attributes

### MCP Tool Call Spans

Each tool call creates a span following MCP and OpenTelemetry semantic conventions:

**Span Name Format**: `{mcp.method.name} {target}` (e.g., "tools/call resources_get")

**Attributes**:
- `mcp.method.name` - MCP protocol method (e.g., "tools/call") **[Required]**
- `gen_ai.tool.name` - Name of the tool being called (e.g., "resources_get", "helm_install") **[Required for tool calls]**
- `gen_ai.operation.name` - Set to "execute_tool" for tool calls **[Recommended]**
- `rpc.jsonrpc.version` - JSON-RPC version (typically "2.0") **[Recommended]**
- `network.transport` - Transport protocol: "pipe" for STDIO, "tcp" for HTTP **[Recommended]**
- `error.type` - Error classification: "tool_error" for tool failures, "_OTHER" for other errors **[Conditional]**

### HTTP Request Spans

HTTP requests create spans following [OpenTelemetry HTTP semantic conventions](https://opentelemetry.io/docs/specs/semconv/http/http-spans/):

**Span Name Format**: `{METHOD} {path}` (e.g., "POST /message")

**Attributes**:
- `http.request.method` - Request method (GET, POST, etc.) **[Required]**
- `url.path` - URL path **[Required]**
- `url.scheme` - URL scheme (http or https) **[Required]**
- `server.address` - Server host **[Recommended]**
- `network.protocol.name` - Protocol name (http) **[Recommended]**
- `network.protocol.version` - Protocol version (HTTP/1.1, HTTP/2) **[Recommended]**
- `client.address` - Client IP address **[Recommended]**
- `http.route` - Normalized route pattern (when different from path) **[Conditional]**
- `user_agent.original` - User agent string (when present) **[Conditional]**
- `http.request.body.size` - Request body size (when present) **[Conditional]**
- `http.response.status_code` - Response status code **[Required]**
- `error.type` - HTTP status code for 4xx/5xx responses **[Conditional]**

**Note**: HTTP spans only appear when running in HTTP mode. STDIO mode (Claude Code) only creates MCP tool call spans. The `/healthz` endpoint is not traced to reduce noise.

## Stats Endpoint

When running in HTTP mode, the server exposes a `/stats` endpoint that returns real-time statistics as JSON:

```bash
curl http://localhost:8080/stats
```

Example response:
```json
{
  "total_tool_calls": 42,
  "tool_call_errors": 2,
  "tool_calls_by_name": {
    "resources_list": 15,
    "pods_get": 12,
    "helm_list": 10,
    "resources_get": 5
  },
  "total_http_requests": 100,
  "http_requests_by_path": {
    "/mcp": 50,
    "/sse": 30,
    "/message": 20
  },
  "uptime_seconds": 3600.5
}
```

The stats endpoint is useful for:
- Health monitoring and alerting
- Quick debugging without a full observability stack
- Integration with simple monitoring systems

**Note**: The `/stats` endpoint is only available in HTTP mode. In STDIO mode, use OTLP export for metrics.

## Metrics Endpoint

When running in HTTP mode, the server exposes a `/metrics` endpoint for Prometheus scraping:

```bash
curl http://localhost:8080/metrics
```

This endpoint returns metrics in OpenMetrics/Prometheus text format, suitable for scraping by Prometheus or compatible systems.

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `k8s_mcp_tool_calls_total` | Counter | Total MCP tool calls (labeled by `tool_name`) |
| `k8s_mcp_tool_errors_total` | Counter | Total MCP tool errors (labeled by `tool_name`) |
| `k8s_mcp_tool_duration_seconds` | Histogram | Tool call duration in seconds |
| `k8s_mcp_http_requests_total` | Counter | HTTP requests (labeled by `http_request_method`, `url_path`, `http_response_status_class`) |
| `k8s_mcp_server_info` | Gauge | Server info (labeled by `version`, `go_version`) |

### Prometheus Scrape Configuration

```yaml
scrape_configs:
  - job_name: 'kubernetes-mcp-server'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: /metrics
```

### Kubernetes ServiceMonitor

When deployed in Kubernetes with the Helm chart, enable the ServiceMonitor:

```yaml
metrics:
  serviceMonitor:
    enabled: true
    interval: 30s
```

**Note**: The `/metrics` endpoint is only available in HTTP mode.

## Troubleshooting

### Tracing not working?

1. **Check endpoint is set**:
   ```bash
   echo $OTEL_EXPORTER_OTLP_ENDPOINT
   ```

2. **Check server logs** (increase verbosity):
   ```bash
   # Look for "OpenTelemetry tracing initialized successfully"
   kubernetes-mcp-server -v 2
   ```

   If tracing fails to initialize, you'll see:
   ```
   Failed to create OTLP exporter, tracing disabled: <error details>
   ```

3. **Verify OTLP collector is reachable**:
   ```bash
   # For gRPC endpoint (port 4317)
   telnet localhost 4317

   # For HTTP endpoint (port 4318)
   curl http://localhost:4318/v1/traces
   ```

### No traces appearing in backend?

1. **Check sampling** - you might be sampling at 0% or using `always_off`:
   ```bash
   echo $OTEL_TRACES_SAMPLER
   echo $OTEL_TRACES_SAMPLER_ARG
   ```

2. **Verify service name**:
   ```bash
   echo $OTEL_SERVICE_NAME
   ```
   Search for this service name in your tracing UI (defaults to "kubernetes-mcp-server").

3. **Check backend configuration** - ensure your OTLP collector is forwarding to the right backend.

4. **Verify protocol compatibility**:
   - If using HTTP-based backends, ensure you set `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`
   - Check if you need port 4317 (gRPC) or 4318 (HTTP)

### TLS/Certificate Issues

If using HTTPS/secure endpoints:

1. **Certificate errors**:
   ```bash
   # Provide custom CA certificate
   export OTEL_EXPORTER_OTLP_CERTIFICATE=/path/to/ca.crt
   ```

2. **Self-signed certificates**:
   ```bash
   # For testing only - not recommended for production
   export OTEL_EXPORTER_OTLP_INSECURE=true
   ```

## Performance Impact

Tracing has minimal performance overhead:

- **Middleware tracing**: Typically 1-2ms per tool call
- **Network overhead**: Spans are batched and exported every 5 seconds
- **Memory**: Approximately 1-5MB for span buffers
- **CPU**: Negligible (<1% for most workloads)

For production deployments with high traffic, use ratio-based sampling to reduce costs while maintaining observability.

## Advanced Topics

### Resource Detection

The OpenTelemetry SDK automatically detects and adds resource attributes from the environment:

- **Host information**: hostname, OS, architecture
- **Process information**: PID, executable name
- **Container information**: container ID (when running in containers)
- **Kubernetes information**: pod name, namespace (when K8s env vars are present)

These are merged with any attributes you set via `OTEL_RESOURCE_ATTRIBUTES`.

### Distributed Tracing

When the kubernetes-mcp-server is part of a distributed system:

1. **Parent spans** are automatically detected and respected
2. **Trace context** is propagated via standard W3C Trace Context headers
3. **Sampling decisions** from parent spans are inherited (via ParentBased sampler)

This means traces can span multiple services seamlessly.

### Custom Resource Attributes

Add custom attributes to help identify and filter traces:

```bash
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=staging,team=platform,region=us-west-2,version=v1.2.3"
```

These attributes appear on **all spans** from this service instance and are useful for:
- Filtering traces by environment (prod vs staging)
- Analyzing performance by region or deployment
- Tracking issues to specific versions or teams
