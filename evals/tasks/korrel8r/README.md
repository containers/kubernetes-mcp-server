# Korrel8r correlation eval

This suite starts the published `quay.io/korrel8r/korrel8r:0.13.3` image with
`web --config=/dev/null --unsafe-shared-session`. With no store configuration,
Korrel8r uses its built-in Kubernetes correlation rules and the pod's
ServiceAccount to read the Kubernetes API; there are no Loki, Prometheus or
Tempo backends. The account is bound to the built-in read-only `view` role.
**Unsafe shared session disables user isolation.** This is for an isolated
local eval cluster only: there is no Service or ingress, and the port-forward
binds only to `127.0.0.1`. Do not expose it to other users.

The eval uses the Korrel8r Deployment it installs in `korrel8r-eval` as its
fixture, asking for that Deployment's correlated ReplicaSet and Pods. It does
not depend on any pre-existing system workload; setup waits for its own
Deployment to become available before the task starts.

```bash
make minikube-create-cluster # or use an existing Kubernetes cluster
export KUBECONFIG="$PWD/_output/kubeconfig" # if using the make-created cluster
export MODEL_BASE_URL='https://your-model-endpoint/v1'
export MODEL_KEY='your-api-key'
make run-korrel8r-evals AGENT=builtin-openai
```

The target deploys Korrel8r, port-forwards localhost `:18080`, builds and
starts the MCP server, runs the suite, then stops the server and removes the
eval pod, binding, and port-forward even if the eval fails. To keep the backend
running for manual debugging, instead run `shared/setup.sh`, `make run-server
TOOLSETS='core,config,observability/korrel8r'`, and `make run-evals
SUITE=korrel8r`, then clean up with `make stop-server teardown-korrel8r-eval`.

The task's `spec.setup` also runs `shared/setup.sh`, so an already-running
MCP server with `dev/config/mcp-configs/korrel8r.toml` can run it directly.
Use `KORREL8R_EVAL_PORT=18080` (the default) for the setup; if changing it,
update the TOML URL as well. For a baseline with only existing tools, run
`make run-server TOOLSETS=core,config` and re-run the same suite; compare
results with the Korrel8r toolset enabled. The eval asserts graph traversal
and object inspection in addition to judging the final answer. This suite is
not part of the default core CI run.
