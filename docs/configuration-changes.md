# Configuration Changes
- [Ecosystem toolsets as Tool and Prompt resources](#ecosystem-toolsets-as-tool-and-prompt-resources)
  - [Removed toolsets and configuration keys](#removed-toolsets-and-configuration-keys)
  - [Installing the replacement charts](#installing-the-replacement-charts)
  - [Trust model](#trust-model)
  - [Windows EULA confirmation](#windows-eula-confirmation)
  - [Tool and prompt differences](#tool-and-prompt-differences)
  - [Multi-cluster and reload](#multi-cluster-and-reload)
- [Unified configuration surface](#unified-configuration-surface)
  - [Dropped CLI flags](#dropped-cli-flags)
  - [Drop-in directory](#drop-in-directory)
  - [Environment variables](#environment-variables)
  - [Bootstrap env rename](#bootstrap-env-rename)
  - [Unknown keys](#unknown-keys)
  - [SIGHUP](#sighup)
  - [Stdio and OAuth](#stdio-and-oauth)
  - [Vectors at a glance](#vectors-at-a-glance)
  - [Migration examples](#migration-examples)
- [Upgrading from 0.0.66](#upgrading-from-0066)

This document records configuration changes that may require action when upgrading between releases. Add newer release transitions as separate sections so upgrade guidance remains available without cluttering the current [configuration reference](configuration.md).

## Ecosystem toolsets as Tool and Prompt resources

The Kiali, NetObserv, Tekton, KubeVirt and kcp toolsets are no longer compiled
into the server. The same tools and prompts are delivered as cluster-scoped
`Tool` and `Prompt` custom resources (`tools.kubernetes-mcp-server.io/v1alpha1`),
served by the new opt-in `dynamic` toolset and packaged as Helm charts under
[`examples/dynamic-tools/charts`](../examples/dynamic-tools/charts). The model is
described in [`specs/dynamic-tools.md`](specs/dynamic-tools.md); the per-tool
migration record is
[`examples/dynamic-tools/conversion-report.md`](../examples/dynamic-tools/conversion-report.md).

The `core`, `config` and `helm` toolsets are unchanged.

### Removed toolsets and configuration keys

| Removed | Replacement |
|---------|-------------|
| `toolsets = [..., "kiali"]` | `"dynamic"` plus the `kiali-mcp-tools` chart |
| `toolsets = [..., "netobserv"]` | `"dynamic"` plus the `netobserv-mcp-tools` chart |
| `toolsets = [..., "tekton"]` | `"dynamic"` plus the `tekton-mcp-tools` chart |
| `toolsets = [..., "kubevirt"]` | `"dynamic"` plus the `kubevirt-mcp-tools` chart |
| `toolsets = [..., "kcp"]` | nothing: `kcp_workspace_describe` ships in `examples/dynamic-tools/converted/kcp.yaml`; `kcp_workspaces_list` was redundant with the kcp provider's `workspace_list` |
| `[toolset_configs.kiali]` (`url`, `insecure`, `certificate_authority`, ...) | chart values `url`, `insecureSkipTLSVerify`, `certificateAuthority` |
| `[toolset_configs.netobserv]` (`url`, `namespace`, `service`, `port`, ...) | chart value `url` (the full plugin URL), plus the TLS values above |

A configuration that still names a removed toolset fails validation
(`invalid toolset name`), and a `toolset_configs.kiali` or
`toolset_configs.netobserv` table fails the load as an unknown key. Downstream
builds whose default overrides enable or rebrand these toolsets must switch to
the charts; the rebranded names become chart values rather than Go overrides.

### Installing the replacement charts

```bash
# once per cluster
kubectl apply -f examples/dynamic-tools/crd.yaml

# per project; http-backed charts take the backend URL
helm upgrade --install kiali-mcp-tools examples/dynamic-tools/charts/kiali-mcp-tools \
  --set url=https://kiali.istio-system.svc:20001/kiali
helm upgrade --install netobserv-mcp-tools examples/dynamic-tools/charts/netobserv-mcp-tools \
  --set url=https://netobserv-plugin.netobserv.svc:9001
helm upgrade --install tekton-mcp-tools examples/dynamic-tools/charts/tekton-mcp-tools
helm upgrade --install kubevirt-mcp-tools examples/dynamic-tools/charts/kubevirt-mcp-tools
```

```toml
toolsets = ["core", "config", "dynamic"]
```

The server lists Tool and Prompt resources with **its own** identity, so its
ServiceAccount needs `get`, `list` and `watch` on `tools` and `prompts` in the
`tools.kubernetes-mcp-server.io` group (the server Helm chart's
`rbac.extraClusterRoles` can carry this rule). Tool calls run with the
**caller's** identity, as before. Tool names, descriptions, input schemas and
annotations are unchanged, so `enabled_tools`, `disabled_tools`,
`tool_overrides` and confirmation rules keyed by tool name keep working.
`require_tls` still refuses plain-HTTP backends at request time.

For `make run-server` based evaluation setups, `make dynamic-tools-install
DYNAMIC_TOOLS_CHARTS="kiali-mcp-tools ..."` installs the CRDs and charts.

### Trust model

A Tool or Prompt resource is prompt text handed to every agent that connects,
and an `http` Tool names a URL that receives the caller's bearer token.
Previously those URLs came only from the server's own configuration. Treat
the ability to create `tools` and `prompts` resources as cluster-admin level:
restrict it with RBAC, review charts before installing them, and audit the
`url` values of installed Tools. The resources are cluster-scoped on purpose
so that namespace administrators cannot add them.

### Windows EULA confirmation

The compiled-in validator that asked for Microsoft EULA acceptance before a
`windows-efi-installer` PipelineRun was created has been removed. The same
check is now a kube-level confirmation rule that inspects the request body,
using the new `expression` field of `confirmation_rules` (see
[configuration.md](configuration.md#confirmation-rules)). It is **no longer
on by default**: add
[`examples/dynamic-tools/charts/kubevirt-mcp-tools/confirmation-rules.toml`](../examples/dynamic-tools/charts/kubevirt-mcp-tools/confirmation-rules.toml)
to the server's `--config-dir` wherever the KubeVirt tools are used. The
`windows-golden-image` prompt still asks for consent through elicitation
before it renders a PipelineRun with `acceptEula: "true"`.

### Tool and prompt differences

The ported definitions keep the previous contracts; these are the
behavioral differences:

| Where | Change |
|-------|--------|
| `vm_create` | Removed. The new `vm-create` prompt gathers the DataSources, instancetypes, preferences and templates and asks the model to choose; `vm_create_from_template` is the deterministic alternative. |
| `vm_lifecycle` | `start`/`stop`/`restart` change `runStrategy` with a merge patch instead of a full-object update. |
| `vm_troubleshoot` | Includes virt-launcher logs with secrets redacted and redacts cloud-init volumes wholesale; the PVC fallback and cloud-init field analysis are not reproduced. |
| `vm_console_screenshot` | The text description no longer states the image dimensions. |
| `tekton_pipelinerun_logs`, `tekton_taskrun_logs` | One request per container instead of a stream; empty logs are not skipped; output is one header per container rather than grouped by TaskRun. |
| `tekton_pipeline_start`, `tekton_task_start` | Do not check that the Pipeline or Task exists before creating the run. |
| `windows-golden-image` prompt | The Tekton Hub catalog name is the upstream default; downstream overrides must edit the chart. |
| NetObserv tools | Response size is capped at 8 MiB (previously 4 MiB for JSON and 2 MiB for exports, with a truncation note). |
| Prompts | Titles declared in Prompt resources are not advertised; the server's prompt listing only forwards descriptions (pre-existing behavior). |

### Multi-cluster and reload

Tool and Prompt resources are read from the default target only; in
multi-cluster mode they are exposed for every target. Changes to the
resources are picked up within the toolset's poll interval (15 seconds) and
announced through `tools/list_changed`; the `dynamic` toolset must be enabled
at startup to be watched.

## Unified configuration surface

Runtime configuration is a single Option model: each setting is resolved once per load from defaults, then files, then env. Provenance is logged at startup (`value (source)`). Unknown TOML keys fail the load. String option values (including string-list elements and duration strings) have surrounding whitespace stripped at load.

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
| `--config` | `MCP_CONFIG_PATH` (ignored if the flag is set) | Main TOML path |
| `--config-dir` | — | Directory of `.toml` files (usable alone; no default) |

### Drop-in directory

`--config-dir` loads a directory of `.toml` files and can be used without `--config`. A sibling `conf.d/` next to `--config` is no longer loaded automatically. If that directory still contains `.toml` files that the old implicit lookup would have applied, the load fails and tells you to pass `--config-dir`. Relative `--config` and `--config-dir` are resolved against the working directory, independently of each other.

### Environment variables

Existing env names for runtime options are kept. Empty env is unset (does not override files). Values are applied at load, including SIGHUP — not at getter time.

| Env | TOML | Notes |
|-----|------|-------|
| `MCP_CONFIG_PATH` | — | Bootstrap only. **Renamed** from `K8S_MCP_CONFIG_PATH` (no shim) |
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

### Bootstrap env rename

`K8S_MCP_CONFIG_PATH` is **renamed** to `MCP_CONFIG_PATH`. The old name is not read; there is no deprecation window and no compatibility shim. `--config` still wins when the flag is set.

Since a config is now required for http mode (`port` must be set), the container image now writes `/etc/kubernetes-mcp-server/config.toml` and points `ENV MCP_CONFIG_PATH` to it. `docker run -e MCP_CONFIG_PATH=/cfg/config.toml …` overrides the image default; an explicit `--config` still wins over the env var.

### Unknown keys

A file that contains any key not in the schema fails that load. Startup and SIGHUP exit. `toolset_configs.<name>` and `cluster_provider_configs.<name>` remain valid for registered extensions; unknown fields *inside* those blocks also fail.

Removed `sts_*` / `token_exchange_strategy` keys are rejected with a pointer to the `[token_exchange]` table (see [below](#upgrading-from-0066)).

### SIGHUP

SIGHUP re-reads files and re-applies env. A parse, unknown-key, non-reloadable, or Validate failure exits the process. Validate failures dump the rejected config first. Non-reloadable options whose resolved value would change fail the load (checked before `toolset_configs` / `cluster_provider_configs` are parsed). An invalid or changed `cluster_provider_configs` table fails the load. Surrounding whitespace on string values is stripped at load, so padding a non-reloadable path (for example `tls_cert`) is not a change.

On SIGHUP the option dump includes `changed=true` and `previous` for values that differ from the prior config.

Requires restart: listen/TLS (`port`, `bind_address`, `metrics_port`, `tls_*`, `http.read_header_timeout`), `stateless`, `disable_localhost_protection`, `server_instructions`, `apps_enabled`, `kubeconfig`, `cluster_provider_strategy`, Kubernetes client QPS/burst and watcher timings, `[telemetry]`, `cluster_provider_configs`.

Reloadable: logging, `list_output`, access-control and tool filtering, toolsets/prompts/confirmation, OAuth and `[token_exchange]`, `trust_proxy_headers`, HTTP body/rate-limit settings, `toolset_configs`.

Still requires `--config` and/or `--config-dir`. Unavailable on Windows.

### Stdio and OAuth

Empty `port` (stdio) with `require_oauth = true` fails the load. OAuth is HTTP-only.

### Vectors at a glance

`—` means that vector is off. Reload `yes` means a SIGHUP that changes the value takes effect; `no` means a SIGHUP that would change the value is rejected and the process exits.

| TOML | Env | Reload |
|------|-----|--------|
| `log_level`, `log_file` | — | yes |
| `port`, `bind_address`, `metrics_port`, `stateless`, `disable_localhost_protection`, `server_instructions`, `apps_enabled` | — | no |
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

Chart `tls.enabled` writes `tls_cert` / `tls_key` into the mounted ConfigMap. Chart `extraArgs` and container `args` that still pass removed flags (`--tls-cert`, `--port`, …) fail with cobra's `unknown flag`.

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
