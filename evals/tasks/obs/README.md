# Observability (obs) evaluation tasks

These tasks exercise the observability MCP toolsets (`list_metrics`, `execute_instant_query`, `execute_range_query`, `get_alerts`, `tempo_list_instances`, `tempo_search_traces`, `loki_list_instances`, `loki_query_range`, `otelcol_get_versions`, `otelcol_validate_config`, and others) against live Prometheus, Tempo, and Loki backends.

## Prerequisites

Install Prometheus, Tempo, and Loki with `make obs-setup` (shared with the obs e2e suite).
Because evals start the MCP server on the host via `make run-server`, also port-forward
the backends to localhost (`make expose-obs`; URLs configured in `dev/config/mcp-configs/obs.toml`).

**One-command reproducer** (install stacks, port-forward, start MCP server, run evals, clean up):

```bash
make minikube-create-cluster   # skip if you already have a cluster
export KUBECONFIG="$PWD/_output/kubeconfig"
export MODEL_BASE_URL='https://your-api-endpoint/v1'
export MODEL_KEY='your-api-key'
# Optional: separate judge endpoint (defaults to MODEL_* when unset)
export JUDGE_BASE_URL='https://your-judge-endpoint/v1'
export JUDGE_API_KEY='your-judge-key'
export JUDGE_MODEL_NAME='gpt-5'

make setup-obs-evals      # obs-setup + expose-obs (port-forwards)
make run-obs-evals
```

**Manual steps** (same flow, useful for debugging):

```bash
make minikube-create-cluster
export KUBECONFIG="$PWD/_output/kubeconfig"
make obs-setup            # Prometheus, Tempo Operator, Loki Operator
make expose-obs           # port-forward all backends
make run-server TOOLSETS='observability/metrics,observability/traces,observability/logs,observability/otelcol' \
  MCP_CONFIG_DIR=dev/config/mcp-configs
make run-evals SUITE=obs
make stop-server stop-obs
```

Ensure `dev/config/mcp-configs/obs.toml` points at the port-forwards:

```toml
[toolset_configs."observability/metrics"]
auth_mode = "kubeconfig"
insecure = true
prometheus_url = "http://127.0.0.1:9090"
alertmanager_url = "http://127.0.0.1:9093"

[toolset_configs."observability/traces"]
auth_mode = "kubeconfig"
insecure = true
tempo_url = "http://127.0.0.1:3200"

[toolset_configs."observability/logs"]
auth_mode = "kubeconfig"
insecure = true
loki_url = "http://127.0.0.1:3100"
```

| Host port | Backend |
|-----------|---------|
| `9090` | Prometheus (`monitoring/prometheus-k8s`) |
| `9093` | Alertmanager (`monitoring/alertmanager-main`) |
| `3200` | Tempo query-frontend (`tracing/tempo-tempo1`) |
| `3100` | Loki gateway (`obs-loki/obs-loki-gateway-http`) — not `8080` (MCP uses that) |

E2E tests do **not** need port-forwards — they deploy the MCP server in-cluster and use
service DNS (see `test/e2e/obs_helpers_test.go`).

### Pass rate target

Target **≥ 80%** task and assertion pass rate. The CI workflow uses `task-pass-threshold: 0.8` and `assertion-pass-threshold: 0.8`. Obs evals are **not** in the default weekly `core` suite; run them locally or trigger `/run-mcpchecker obs` on a PR.

## Filter a single task

```bash
make run-evals SUITE=obs EVAL_TASK_FILTER=cpu-usage
```

## Tasks

| Category | Tasks | Tools exercised |
|----------|-------|-----------------|
| metrics / discovery | list-metrics, list-node-metrics, backend-reachability | `list_metrics`, `execute_instant_query` |
| labels | label-names, label-values, get-series, series-by-namespace | `list_metrics`, `get_label_names`, `get_label_values`, `get_series` |
| queries (instant/range) | cpu-usage, memory-usage, network-traffic, pending-pods, pods-created, crashlooping-pods, prometheus-head-series, prometheus-requests, prometheus-wal-size, time-range-query, visualize-cpu-usage | `execute_instant_query`, `execute_range_query`, `show_timeseries` |
| error handling | nonexistent-metric, nonexistent-namespace, high-cardinality-rejection | `list_metrics`, `execute_instant_query` |
| multi-step reasoning | namespace-resource-usage, namespace-pod-count, diagnose-cluster-health | `list_metrics`, `execute_instant_query`, `get_alerts` |
| alerts | get-alerts, filtered-alerts, alert-investigation, get-silences | `get_alerts`, `get_silences` |
| traces | search-error-traces, search-semconv, latency-investigation, latency-baseline | `tempo_list_instances`, `tempo_search_traces` |
| logs | loki-list-instances, loki-backend-reachability, loki-label-names, loki-label-values, loki-query-network-flows | `loki_list_instances`, `loki_label_names`, `loki_label_values`, `loki_query_range` |
| otelcol | get-versions, list-components, get-batch-processor-schema, validate-batch-processor-config-valid, validate-batch-processor-config-invalid | `otelcol_get_versions`, `otelcol_list_components`, `otelcol_get_component_schema`, `otelcol_validate_config` |

## Adding a New Task

1. Create a new subdirectory under `obs/` (e.g., `obs/my-new-task/my-new-task.yaml`).
2. Include `suite: obs` and `requires: obs` in `metadata.labels`.
3. Add a corresponding `taskSet` entry in each `evals/core-eval-testing/*/eval-obs.yaml`.
4. Document the tool assertions (toolsUsed, callOrder, minToolCalls, maxToolCalls).

## Maintainer trigger

On a PR, submit a review (Files changed → Review changes) containing:

```text
/run-mcpchecker obs
```

Or use **Actions → mcpchecker MCP Evaluation → Run workflow** with suite `obs`.
