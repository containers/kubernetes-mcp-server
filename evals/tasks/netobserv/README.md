# NetObserv evaluation tasks

These tasks exercise the **netobserv** MCP toolset (`netobserv_list_flows`, `netobserv_get_flow_metrics`, `netobserv_export_flows`) against the NetObserv console plugin HTTP API (Loki flow records, Prometheus metrics, CSV export).

## Eval-first methodology

Before relying on these tools, the domain was validated with **baseline evals** using only `core` (and `config`) toolsets. Generic Kubernetes tools cannot query NetObserv flow logs or aggregated flow metrics through the console plugin API, so dedicated tools fill a genuine capability gap rather than wrapping `pods_exec` or `resources_get`.

Run a baseline locally when changing the toolset:

```bash
mcpchecker check evals/openai-agent/eval.yaml --label-selector suite=netobserv \
  # with toolsets = ["core"] only — tasks should fail or produce poor results
```

Then re-run with `netobserv` enabled (see below) and compare pass rates and tool usage (`netobserv_.*` assertions in `evals/*/eval.yaml`).

Each task is **self-contained**: `spec.setup` runs `shared/setup-mock.sh`, which deploys the in-cluster mock plugin and ensures `http://127.0.0.1:9001` is reachable before the agent runs.

## Prerequisites

### Kind / local (mock plugin — recommended)

No NetObserv operator is required. The mock in `shared/mock-plugin.yaml` implements the same paths as the real console plugin (`/api/loki/flow/records`, `/api/loki/export`, etc.).

**One-command reproducer** (deploy mock, start MCP server, run evals, clean up):

```bash
make kind-create-cluster   # skip if you already have kubectl context
export MODEL_BASE_URL='https://your-api-endpoint/v1'
export MODEL_KEY='your-api-key'
# Optional: separate judge endpoint (defaults to MODEL_* when unset)
export JUDGE_BASE_URL='https://your-judge-endpoint/v1'
export JUDGE_API_KEY='your-judge-key'
export JUDGE_MODEL_NAME='gpt-5'

make run-netobserv-evals
```

**Manual steps** (same flow, useful for debugging):

```bash
make kind-create-cluster
make setup-netobserv          # deploy mock + port-forward :9001
make run-server TOOLSETS=core,netobserv MCP_CONFIG_DIR=dev/config/mcp-configs
make run-evals EVAL_LABEL_SELECTOR=suite=netobserv
make stop-server stop-netobserv teardown-netobserv
```

Ensure `dev/config/mcp-configs/netobserv.toml` points at the port-forward:

```toml
[toolset_configs.netobserv]
url = "http://127.0.0.1:9001"
insecure = true
```

### Pass rate target

OpenShift MCP integration expects **≥ 80%** task and assertion pass rate. The CI workflow uses `task-pass-threshold: 0.8` and `assertion-pass-threshold: 0.8`. NetObserv evals are **not** in the default weekly `core` suite; run them locally or trigger `/run-mcpchecker netobserv` on a PR.

### OpenShift (real NetObserv)

On a cluster with the [NetObserv operator](https://github.com/netobserv-network-observability/netobserv-operator) and FlowCollector:

1. Deploy kubernetes-mcp-server with `toolsets` including `netobserv` (see [docs/NETOBSERV.md](../../../docs/NETOBSERV.md)).
2. Allow the MCP namespace in `FlowCollector.spec.networkPolicy.additionalNamespaces` if network policies block the plugin.
3. Port-forward the plugin if the MCP server runs outside the cluster:

   ```bash
   oc port-forward -n netobserv svc/netobserv-plugin 9001:9001
   ```

4. Run evals with the same label selector:

   ```bash
   mcpchecker check evals/openai-agent/eval.yaml --label-selector suite=netobserv
   ```

LLM judge strings in tasks assume the **mock** responses (`netobserv-eval`, `eval-flow-1`, …). On a live cluster, adjust `verify.llmJudge.contains` or rely on tool assertions only.

## Tasks

| Task | Tool exercised | Mock expectation |
|------|----------------|------------------|
| list-flows | `netobserv_list_flows` | includes `eval-flow-1` |
| get-flow-metrics | `netobserv_get_flow_metrics` | status `success` |
| export-flows | `netobserv_export_flows` | CSV header `TimeFlowStartMs` |
| tls-breakdown | `netobserv_get_flow_metrics` | includes `TLS 1.3` |

## Maintainer trigger

On a PR, comment:

```text
/run-mcpchecker netobserv
```

Or use **Actions → mcpchecker MCP Evaluation → Run workflow** with suite `netobserv`.
