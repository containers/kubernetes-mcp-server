# Guest Observability

The Guest Observability toolset provides access to virtual machine guest logs stored in Loki. It enables investigation of operating system events that occur inside KubeVirt virtual machines and may not be visible from Kubernetes infrastructure telemetry alone.

The toolset currently provides a read-only LogQL query tool for investigating guest logs.

## Prerequisites

Before enabling Guest Observability:

- A Loki backend must be reachable from the Kubernetes MCP Server.
- VM guest logs must already be collected and stored in Loki.
- Guest telemetry should include the recommended identity and classification labels described below.
- For HTTPS Loki endpoints using a private CA, the CA certificate must be accessible to the Kubernetes MCP Server.

Guest Observability does not deploy Loki or guest log collectors.

## Configuration

Enable the toolset and configure the Loki backend in the server configuration:

```toml
toolsets = ["core", "guest-observability"]

[toolset_configs.guest-observability.loki]
url = "https://loki.example.com"
tenant = "application"
certificate_authority = "/path/to/ca.crt"
```

### Loki configuration

| Option | Required | Description |
|--------|----------|-------------|
| `url` | Yes | Base URL of the Loki backend |
| `tenant` | No | Loki tenant sent using the `X-Scope-OrgID` header |
| `certificate_authority` | No | Path to a PEM CA certificate used to verify the Loki server |
| `insecure` | No | Disables TLS certificate verification when set to `true` |

Relative `certificate_authority` paths are resolved relative to the server configuration directory.

For development or testing with an HTTPS endpoint whose certificate cannot be verified:

```toml
[toolset_configs.guest-observability.loki]
url = "https://127.0.0.1:3100"
insecure = true
```

> **Warning:** `insecure = true` disables certificate verification and should not be used for production deployments.

When the global `require_tls` option is enabled, Guest Observability rejects HTTP Loki endpoints and configurations using `insecure = true`.

See [Configuration Reference](configuration.md) for global TLS settings.

## Available tools

### `guest-observability_loki_query`

Executes a LogQL range query against the configured Loki backend.

The tool is read-only and does not modify Loki data, virtual machines, or Kubernetes resources.

| Parameter | Required | Description |
|-----------|----------|-------------|
| `query` | Yes | LogQL query expression |
| `start` | No | RFC3339 start timestamp with an explicit timezone |
| `end` | No | RFC3339 end timestamp with an explicit timezone |
| `limit` | No | Maximum number of raw log entries to return. Defaults to `100` and is capped at `1000` |
| `direction` | No | Log ordering direction: `backward` or `forward`. Defaults to `backward` |
| `step` | No | Query resolution for metric-style LogQL range queries, for example `30s`, `1m`, or `5m` |

When both `start` and `end` are omitted, the query defaults to the previous hour ending at the server's current time.

## Guest telemetry labels

Guest telemetry uses the following label contract:

| Label | Purpose |
|-------|---------|
| `namespace` | Kubernetes namespace associated with the VM |
| `vm_name` | KubeVirt VirtualMachine name |
| `os` | Guest operating system classification, such as `windows` |
| `source` | Guest telemetry source, such as `windows_eventlog` |
| `collector` | Collector identity that may remain available for orphan telemetry |

`namespace` and `vm_name` together establish reliable VM identity.

`os` and `source` classify the telemetry and can be used to narrow guest-log searches.

For example, a Windows guest log stream may contain:

```text
namespace="my-namespace"
vm_name="windows-vm"
os="windows"
source="windows_eventlog"
```

## Incomplete telemetry labels

Loki label matchers exclude streams where the matched label is absent. An empty query using `namespace`, `vm_name`, `os`, or `source` therefore does not necessarily mean that the guest event is absent.

When telemetry may have incomplete labels, queries can be broadened by removing unavailable contract-label matchers while preserving filters for the event being investigated.

A missing label must not be inferred from unrelated Kubernetes resources or from another log stream.

Identity and classification labels have different implications:

- Missing `namespace` or `vm_name` prevents reliable VM attribution.
- Missing `os` or `source` represents classification uncertainty but does not by itself invalidate VM identity when both `namespace` and `vm_name` are present.
- Orphan telemetry may retain a `collector` label even when the standard identity and classification labels are absent.

Query results can include `guestTelemetryContractWarnings` describing missing labels and whether VM attribution is reliable.

## Query examples

### Query guest logs for a specific VM

```logql
{namespace="my-namespace",vm_name="windows-vm"}
```

### Query Windows Event Log telemetry

```logql
{namespace="my-namespace",source="windows_eventlog"}
```

### Search for Windows storage reset events

```logql
{namespace="my-namespace",source="windows_eventlog"} |~ "(?i)(storport|EventID.?129)"
```

Event identifiers and case-insensitive matching can be useful when provider or component capitalization varies.

### Search orphan guest telemetry

When standard contract labels are unavailable but the collector label remains present:

```logql
{collector="guest-agent"} |~ "(?i)(storport|EventID.?129)"
```

An event found without `namespace` and `vm_name` can establish that the event occurred, but it cannot reliably establish which VM or namespace produced it.

## TLS

HTTPS Loki endpoints use the system trust store by default.

For a Loki endpoint using a private CA:

```toml
[toolset_configs.guest-observability.loki]
url = "https://loki.example.com"
certificate_authority = "/path/to/ca.crt"
```

The configured CA file must contain at least one valid PEM certificate.

Global TLS settings such as `require_tls`, `tls_min_version`, and `tls_cipher_suites` also apply to Loki connections. See [Configuration Reference](configuration.md).

## Troubleshooting

### Loki URL validation fails

Verify that `url` contains a valid URL with both a scheme and host.

### Loki query returns no results

Check:

- The query time range.
- The LogQL expression.
- Whether labels used as matchers are actually present on the guest telemetry.
- Whether the event may exist in telemetry with incomplete labels.

### VM attribution is unreliable

Check whether both `namespace` and `vm_name` are present on the returned stream. Both are required for reliable VM identity.

### TLS certificate validation fails

Verify that the Loki certificate is trusted by the system or configure `certificate_authority` with the appropriate PEM CA certificate.

Use `insecure = true` only for development or testing.

### Loki returns an authorization error

Verify the Loki backend access configuration and, when applicable, the configured `tenant`.
