# Configuration Changes
- [Unified configuration surface](#unified-configuration-surface)
  - [Dropped CLI flags](#dropped-cli-flags)
  - [Drop-in directory](#drop-in-directory)
  - [Environment variables](#environment-variables)
  - [Unknown keys](#unknown-keys)
  - [SIGHUP](#sighup)
  - [Stdio and OAuth](#stdio-and-oauth)
  - [Vectors at a glance](#vectors-at-a-glance)
  - [Migration examples](#migration-examples)
- [Upgrading from 0.0.66](#upgrading-from-0066)

This document records configuration changes that may require action when upgrading between releases. Add newer release transitions as separate sections so upgrade guidance remains available without cluttering the current [configuration reference](configuration.md).

## Unified configuration surface

Runtime configuration is a single Option model: each setting is resolved once per load from defaults, then files, then env. Provenance is logged at startup (`value (source)`). Unknown TOML keys fail the load.

HTTP mode needs `port` in TOML (`--config` and/or `--config-dir`). The CLI no longer accepts runtime flags.

### Dropped CLI flags

| Dropped flag | Replacement |
|--------------|-------------|
| `--port` | `port` in TOML |
| `--bind-address` | `bind_address` |
| `--metrics-port` | `metrics_port` |
| `--log-level` | `log_level` |
| `--log-file` | `log_file` |
| `--kubeconfig` | `kubeconfig` (`$KUBECONFIG` remains client-go fallback when `kubeconfig` is empty) |
| `--list-output` | `list_output` |
| `--read-only` | `read_only` |
| `--disable-destructive` | `disable_destructive` |
| `--stateless` | `stateless` |
| `--toolsets` | `toolsets` |
| `--cluster-provider` | `cluster_provider_strategy` |
| `--disable-multi-cluster` | `cluster_provider_strategy = "disabled"` |
| `--tls-cert` / `--tls-key` | `tls_cert` / `tls_key` |
| `--require-tls` | `require_tls` |
| Hidden OAuth flags (`--require-oauth`, `--authorization-url`, …) | matching TOML keys |

CLI that remains:

| Flag | Env | Role |
|------|-----|------|
| `--version` | — | Print version |
| `--config` | `K8S_MCP_CONFIG_PATH` (ignored if the flag is set) | Main TOML path |
| `--config-dir` | — | Directory of `.toml` files (usable alone; no default) |

### Drop-in directory

`--config-dir` loads a directory of `.toml` files and can be used without `--config`. A sibling `conf.d/` next to `--config` is no longer loaded automatically. Relative `--config` and `--config-dir` are resolved against the working directory, independently of each other.

### Environment variables

Existing env names are kept. Empty env is unset (does not override files). Values are applied at load, including SIGHUP — not at getter time.

| Env | TOML | Notes |
|-----|------|-------|
| `K8S_MCP_CONFIG_PATH` | — | Bootstrap only |
| `TLS_MIN_VERSION` | `tls_min_version` | |
| `TLS_CIPHER_SUITES` | `tls_cipher_suites` | Comma-separated |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `telemetry.endpoint` | |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `telemetry.protocol` | |
| `OTEL_TRACES_SAMPLER` | `telemetry.traces_sampler` | |
| `OTEL_TRACES_SAMPLER_ARG` | `telemetry.traces_sampler_arg` | |
| `OTEL_LOGS_EXPORTER` | `telemetry.logs_exporter` | |
| `OTEL_METRICS_EXPORTER` | `telemetry.metrics_exporter` | |
| `KUBE_CLIENT_QPS` | `kube_client_qps` | **New TOML key** (was env-only) |
| `KUBE_CLIENT_BURST` | `kube_client_burst` | **New TOML key** |
| `KUBECONFIG_DEBOUNCE_WINDOW_MS` | `kubeconfig_debounce_window` | **New TOML key**; env is integer ms, TOML is a Go duration |
| `CLUSTER_STATE_POLL_INTERVAL_MS` | `cluster_state_poll_interval` | **New TOML key**; env is integer ms |
| `CLUSTER_STATE_DEBOUNCE_WINDOW_MS` | `cluster_state_debounce_window` | **New TOML key**; env is integer ms |
| `WORKSPACE_POLL_INTERVAL_MS` | `workspace_poll_interval` | **New TOML key**; env is integer ms |
| `WORKSPACE_DEBOUNCE_WINDOW_MS` | `workspace_debounce_window` | **New TOML key**; env is integer ms |

No new `K8S_MCP_*` names were added for dropped flags.

### Unknown keys

A file that contains any key not in the schema fails that load. Startup exits. SIGHUP logs the error and keeps the previous config. `toolset_configs.<name>` and `cluster_provider_configs.<name>` remain valid for registered extensions; unknown fields *inside* those blocks also fail.

Removed `sts_*` / `token_exchange_strategy` keys are rejected with a pointer to the `[token_exchange]` table (see [below](#upgrading-from-0066)).

### SIGHUP

SIGHUP re-reads files and re-applies env. Non-reloadable options whose resolved value would change keep the previous value and source; the server logs that a restart is required.

On SIGHUP the option dump includes `changed=true` and `previous` for values that differ from the prior config.

Requires restart: listen/TLS (`port`, `bind_address`, `metrics_port`, `tls_*`, `http.read_header_timeout`), `stateless`, `server_instructions`, `kubeconfig`, `cluster_provider_strategy`, Kubernetes client QPS/burst and watcher timings, `[telemetry]`, `cluster_provider_configs`.

Reloadable: logging, `list_output`, access-control and tool filtering, toolsets/prompts/confirmation, OAuth and `[token_exchange]`, `trust_proxy_headers`, HTTP body/rate-limit settings, `toolset_configs`.

Still requires `--config` and/or `--config-dir`. Unavailable on Windows.

### Stdio and OAuth

Empty `port` (stdio) with `require_oauth = true` fails the load. OAuth is HTTP-only.

### Vectors at a glance

`—` means that vector is off. Reload `yes` means a SIGHUP that changes the value takes effect; `no` means the previous value is pinned.

| TOML | Env | Reload |
|------|-----|--------|
| `log_level`, `log_file` | — | yes |
| `port`, `bind_address`, `metrics_port`, `stateless`, `server_instructions` | — | no |
| `list_output` | — | yes |
| `kubeconfig`, `cluster_provider_strategy` | — | no |
| `cluster_auth_mode`, `denied_resources`, `read_only`, `disable_destructive`, `validation_enabled`, `experimental_enable_target_compatibility_tool_filters` | — | yes |
| `toolsets`, `enabled_tools`, `disabled_tools`, `tool_overrides`, `prompts`, `confirmation_*` | — | yes |
| `tls_cert`, `tls_key`, `require_tls` | — | no |
| `tls_min_version` | `TLS_MIN_VERSION` | no (inbound) |
| `tls_cipher_suites` | `TLS_CIPHER_SUITES` | no (inbound) |
| `http.read_header_timeout` | — | no |
| `http.max_body_bytes`, `http.rate_limit_rps`, `http.rate_limit_burst` | — | yes |
| `require_oauth`, `oauth_*`, `authorization_url`, `skip_jwt_verification`, `disable_dynamic_client_registration`, `server_url`, `certificate_authority`, `trust_proxy_headers`, `token_exchange.*` | — | yes |
| `telemetry.*` | matching `OTEL_*` | no |
| `kube_client_qps`, `kube_client_burst` | `KUBE_CLIENT_QPS`, `KUBE_CLIENT_BURST` | no |
| `kubeconfig_debounce_window`, `cluster_state_*`, `workspace_*` | matching `*_MS` | no |
| `toolset_configs` | — | yes |
| `cluster_provider_configs` | — | no |

### Migration examples

HTTP server that used flags:

```bash
# before
kubernetes-mcp-server --port 8080 --log-level 2 --kubeconfig ~/.kube/config --toolsets core,config,helm
```

```toml
# after: config.toml
port = "8080"
log_level = 2
kubeconfig = "/home/user/.kube/config"
toolsets = ["core", "config", "helm"]
```

```bash
kubernetes-mcp-server --config config.toml
```

Disable multi-cluster:

```toml
cluster_provider_strategy = "disabled"
```

In-cluster Deployment args should be `--config` and/or `--config-dir` pointing at mounted TOML, not `--kubeconfig` / `--cluster-provider`.

## Upgrading from 0.0.66

The legacy top-level `token_exchange_strategy` and `sts_*` settings were removed and are rejected at startup. Migrate them as follows:

| Legacy setting | Replacement |
|----------------|-------------|
| `token_exchange_strategy` | `token_exchange.strategy` |
| `sts_audience` | `token_exchange.audience` |
| `sts_scopes` | `token_exchange.scopes` |
| `sts_subject_token_type` | `token_exchange.subject_token_type` |
| `sts_requested_token_type` | `token_exchange.requested_token_type` |
| `sts_client_id` | `token_exchange.client_auth.client_id` |
| `sts_client_secret` | `token_exchange.client_auth.client_secret` |
| `sts_auth_style` | `token_exchange.client_auth.method` |
| `sts_client_cert_file` | `token_exchange.client_auth.certificate_file` |
| `sts_client_key_file` | `token_exchange.client_auth.private_key_file` |
| `sts_federated_token_file` | `token_exchange.client_auth.token_file` |

Map legacy `sts_auth_style` values as follows: `params` to `client_secret_post`, `header` to `client_secret_basic`, `assertion` to `private_key_jwt`, and `federated` to `jwt_file`. Configurations that omitted `token_exchange_strategy` used the built-in STS path and should use `client_secret_basic` to preserve HTTP Basic client authentication.
