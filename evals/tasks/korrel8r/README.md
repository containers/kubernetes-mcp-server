# Korrel8r correlation eval

This suite starts the published `quay.io/korrel8r/korrel8r:0.13.3` image with
`web --unsafe-shared-session`. Its ConfigMap enables Kubernetes and direct log
stores, using Korrel8r's built-in correlation rules and the pod's ServiceAccount
to read resources and container logs from the Kubernetes API. There are no Loki,
Prometheus, or Tempo backends. The account is bound to the built-in read-only
`view` role.
**Unsafe shared session disables user isolation.** This is for an isolated
local eval cluster only: there is no Service or ingress, and the port-forward
binds only to `127.0.0.1`. Do not expose it to other users.

The single task investigates a `checkout-eval` Deployment in `korrel8r-eval`.
It traces the live ReplicaSet and Pod to a mounted ConfigMap, ServiceAccount,
application log, and `Unhealthy` Warning Event. A healthy decoy workload tests
whether the agent attributes the log and Event to the right Pod. The probe
fails intentionally, so the target Deployment never becomes Available. Setup
waits for the signals and checks that both kubectl and Korrel8r's direct log
store can retrieve the log. The judge checks observed objects and signals,
not just graph queries; exact Event counts are not required. The task does
not validate metrics, alerts, or traces.

```bash
make minikube-create-cluster # or use an existing Kubernetes cluster
export KUBECONFIG="$PWD/_output/kubeconfig" # if using the make-created cluster
export MODEL_BASE_URL='https://your-model-endpoint/v1'
export MODEL_KEY='your-api-key'
make run-korrel8r-evals AGENT=builtin-openai
```

The target builds and starts the MCP server, runs the suite (which deploys
Korrel8r and port-forwards localhost `:18080`), then stops the server and
removes the eval pod, binding, and port-forward even if the eval fails.
The eval TOML disables `korrel8r_get_console` and `korrel8r_show_in_console`;
the task does not use a connected OpenShift console. For manual testing, run
`bash evals/tasks/korrel8r/shared/setup-signals.sh` and `make run-server
TOOLSETS='core,config,observability/korrel8r'`; finish with `bash
evals/tasks/korrel8r/shared/teardown-signals.sh` and `make stop-server`.

The task uses `shared/setup-signals.sh` and `shared/teardown-signals.sh`, so an
already-running MCP server with `dev/config/mcp-configs/korrel8r.toml` can run
it directly. The Make wrapper also calls backend teardown if setup fails.
If you start the fixture manually, run `shared/teardown-signals.sh` to remove
both the workload and backend. Use `KORREL8R_EVAL_PORT=18080` (the default);
if changing it, update the TOML URL as well. For a valid baseline using the
*same task, fixture, prompt, and judge*:

```bash
make run-korrel8r-baseline AGENT=builtin-openai
make run-korrel8r-evals AGENT=builtin-openai
```

The baseline target starts the server with only `core,config`; its
`eval-korrel8r-baseline.yaml` requires a core resource/pod/event tool rather
than a Korrel8r graph. The toolset target enables Korrel8r and uses
`eval-korrel8r.yaml`, which requires a Korrel8r graph call. The agent may
inspect objects using either Korrel8r or core tools; the shared answer judge
checks that reported names and relationships are supported by object data.
Tool-specific assertions belong to the eval configs, **not** the shared task:
both runs are judged against the same answer criteria. Do not run baseline and
toolset evals simultaneously against the same cluster.

The two runs produce `mcpchecker-korrel8r-core-baseline-out.json` and
`mcpchecker-korrel8r-e2e-out.json`. Check both answer quality (`taskPassed`)
and tool-use assertions (`allAssertionsPassed`) with `make check-evals` (or
pass the two filenames via `EVAL_RESULTS`). This command fails if either
result fails either check. Compare tool calls and elapsed time over repeated
runs with the same agent, model, and cluster. A `maxToolCalls` assertion guards
against wandering; it does not measure speed. These result files are local
artifacts, not committed baselines. This suite is not part of default core CI.
