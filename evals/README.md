# Kubernetes MCP Server Evals

This directory contains the [mcpchecker](https://github.com/mcpchecker/mcpchecker)
evaluations for the **Kubernetes MCP server**. The same tasks are run against the
same MCP server using different AI agents (Claude Code, an OpenAI-compatible agent,
and others), so agents can be compared on an identical benchmark.

## Structure

```
evals/
├── README.md                       # This file
├── mcp-config.yaml                 # Shared MCP server connection (http://localhost:8080/mcp)
├── claude-code/                    # Claude Code agent config (ACP) + top-level eval.yaml
├── openai-agent/                   # OpenAI-compatible agent config + top-level eval.yaml
├── core-eval-testing/              # Extra agent/provider combinations for core tasks
├── results/                        # Committed eval results (baselines)
└── tasks/                          # Shared task library, grouped by suite (see tasks/README.md)
    └── <suite>/                    # core, config, helm, kiali, kubevirt, tekton
        └── <task-name>/
            ├── <task-name>.yaml    # Task definition (prompt, verify, labels). kubevirt/tekton use task.yaml
            ├── setup.sh            # Optional pre-task cluster setup
            ├── verify.sh           # Optional post-task verification
            ├── cleanup.sh          # Optional resource cleanup
            └── artifacts/          # Optional K8s manifests
```

Tasks are grouped into suites via a `suite: <name>` label. The available suites are
`core`, `config`, `helm`, `kiali`, `kubevirt`, and `tekton`.

## Prerequisites

- A Kubernetes cluster (kind, minikube, or any cluster) and `kubectl` configured for it
- The Kubernetes MCP server running at `http://localhost:8080/mcp` (see `make run-server`)
- `mcpchecker`: `make mcpchecker` installs it under `_output/tools/bin/`. The
  `make run-evals` target calls that path directly; to run the bare `mcpchecker …`
  commands shown below by hand, add `_output/tools/bin` to your `PATH`.
- For the **Claude Code** agent only:
  - The `claude` CLI (Claude Code) installed, on your `PATH`, and **signed in**.
    The agent runs through your existing Claude Code session, so your Claude
    subscription (or an `ANTHROPIC_API_KEY`) covers the agent and no separate key
    is needed. Run `claude` once interactively to log in if you have not already.
  - The `claude-agent-acp` adapter, which bridges mcpchecker to that `claude` CLI,
    installed with `make claude-agent-acp` (runs
    `npm install -g @agentclientprotocol/claude-agent-acp`, so it needs Node.js/npm).
    Without it, `mcpchecker check` fails at agent-spec load with
    `'claude-agent-acp' binary not found in PATH`.

## Quickstart: run one suite locally with Claude Code

This runs the `kubevirt` suite end to end with the Claude Code agent on a Sonnet
model, with **no OpenAI key required**. With the Claude Code agent both the agent
and the LLM judge run on your Claude subscription, so every suite is keyless (see
[Eval configs](#eval-configs-top-level-vs-per-suite) below).

```bash
# 1. Build the server and install the eval tooling
make build mcpchecker claude-agent-acp

# 2. Create a cluster and point KUBECONFIG at it.
#    KUBECONFIG must be EXPORTED so both the kubernetes extension's setup steps
#    and each task's verify.sh kubectl target the same cluster.
kind create cluster --name mcp-eval-cluster --kubeconfig "$PWD/_output/kubeconfig"
export KUBECONFIG="$PWD/_output/kubeconfig"

# 3. Install KubeVirt (only needed for the kubevirt suite).
#    This also auto-deploys the common-instancetypes (u1.small, u1.medium, ...)
#    via virt-operator, so instancetype tasks work without a separate step.
make kubevirt-install

# 4. Start the MCP server with the toolsets the suite needs
make run-server TOOLSETS=core,config,kubevirt

# 5. Run the suite. AGENT selects the agent, SUITE selects the tasks, MODEL sets
#    ANTHROPIC_MODEL for the Claude Code agent. Add EVAL_TASK_FILTER=<name> to run one task.
make run-evals SUITE=kubevirt AGENT=claude-code MODEL=sonnet

# 6. Tear down
make stop-server
kind delete cluster --name mcp-eval-cluster
```

> A bare `kind create cluster` is enough for kubevirt and most toolset suites.
> `make kind-create-cluster` also installs nginx-ingress, cert-manager, and a
> Keycloak hosts entry, which toolset evals do not need (and which pull from
> registries that are not always reachable).

The make-target invocation in step 5 is equivalent to the underlying command:

```bash
KUBECONFIG="$PWD/_output/kubeconfig" ANTHROPIC_MODEL=sonnet mcpchecker check \
  evals/tasks/kubevirt/claude-code/eval.yaml \
  --label-selector suite=kubevirt --output json
```

### Model selection

The model for the Claude Code agent is chosen with the `ANTHROPIC_MODEL`
environment variable (for example `ANTHROPIC_MODEL=sonnet`), which
`claude-agent-acp` reads as its first-priority model source. It **forces** that
model for the run, overriding whatever default your Claude Code session would
otherwise use, and it runs on your existing Claude subscription (no API key
needed). `make run-evals` sets it for you from the `MODEL` variable, so
`make run-evals ... MODEL=sonnet` is how you pin the eval to Sonnet. The same
variable also drives the Claude Code judge (see [Eval configs](#eval-configs-top-level-vs-per-suite)),
so agent and judge share the model.

The OpenAI-compatible agent instead takes its model from
`evals/openai-agent/agent.yaml` plus the `MODEL_BASE_URL` and `MODEL_KEY`
environment variables.

## Running with the OpenAI-compatible agent

```bash
# Set your model credentials
export MODEL_BASE_URL='https://your-api-endpoint.com/v1'
export MODEL_KEY='your-api-key'

# Run the core suite (this is what CI runs)
make run-evals SUITE=core AGENT=openai-agent
# equivalently:
mcpchecker check evals/openai-agent/eval.yaml --label-selector suite=core
```

Different models may pick different tools (`pods_*` or `resources_*`) for the same
task. The assertions accept either, for example `toolPattern: "(pods_.*|resources_.*)"`.

## Eval configs: top-level vs per-suite

There are two top-level eval configs, one per agent:

- **`evals/claude-code/eval.yaml`** (Claude Code): the agent runs via
  `claude-agent-acp`, and the LLM judge is also `builtin.claude-code`, so **both
  agent and judge run on your Claude subscription — fully keyless, for every
  suite**. Judge-backed tasks are really evaluated (semantic), not skipped. This
  is the recommended local path.
- **`evals/openai-agent/eval.yaml`** (OpenAI-compatible, what **CI** runs): the
  agent uses `MODEL_BASE_URL`/`MODEL_KEY` and the judge is `openai:gpt-5`, so its
  judge-backed tasks need an OpenAI key.

A task is "judge-backed" when its verify phase has an `llmJudge` step —
**including the legacy `verify: contains:` short form, which is judge-evaluated
(semantic), not a literal string match.** Judge-backed task counts per suite:
`helm` 0, `core` 4, `kubevirt` 4, `tekton` 5, `config` 3, `kiali` 18. With the
OpenAI agent those tasks need a key; with the Claude Code agent none do. Filter
either config to one suite with `--label-selector suite=<name>`.

**Per-suite** configs (`evals/tasks/<suite>/{claude-code,openai-agent}/eval.yaml`,
currently only `kubevirt`) are scoped to a single suite. The claude-code one
judges with claude-code (keyless); the openai-agent one declares no judge, so its
judge steps degrade to a no-op pass (mcpchecker's `noopLLMJudge`). `make run-evals`
prefers a per-suite config when one exists for the chosen `SUITE`/`AGENT`, and
otherwise falls back to the agent's top-level config.

## Agent configuration

### Claude Code (`claude-code/agent.yaml`)

The Claude Code agent runs over the Agent Client Protocol (ACP) through the
`claude-agent-acp` adapter:

```yaml
kind: Agent
metadata:
  name: "claude-code-acp"
acp:
  cmd: "claude-agent-acp"
```

### OpenAI-compatible agent (`openai-agent/agent.yaml`)

The OpenAI-compatible agent uses mcpchecker's built-in LLM agent:

```yaml
builtin:
  type: "llm-agent"
  model: "openai:gpt-5"
```

## Filtering tasks by suite

The `--label-selector` flag (or the `SUITE` make variable) chooses which tasks run.

```bash
# core tasks
make run-evals SUITE=core AGENT=claude-code MODEL=sonnet

# kiali tasks (needs Istio + Kiali installed; see `make setup-kiali`)
make run-evals SUITE=kiali AGENT=claude-code MODEL=sonnet
```

If you omit `--label-selector`, the eval config's own `labelSelector` settings in
each `taskSets` entry determine which tasks run.

Note: with `AGENT=claude-code` every suite is keyless (agent and judge both run on
your Claude subscription). With `AGENT=openai-agent` the judge-backed tasks need an
OpenAI key — `core` 4, `config` 3, `kiali` 18, `tekton` 5, `kubevirt` 4, `helm` 0
(see [Eval configs](#eval-configs-top-level-vs-per-suite)).

## Versions

`make mcpchecker` installs `mcpchecker@latest`. CI runs the same binary version:
`.github/workflows/mcpchecker.yaml` calls `mcpchecker-action` (currently pinned at
`v0.0.18`) with `mcpchecker-version: latest`. If you need to reproduce CI exactly,
pin the binary with `make mcpchecker MCPCHECKER_VERSION=<version>`.

`make claude-agent-acp` likewise installs `@agentclientprotocol/claude-agent-acp@latest`;
pin it with `make claude-agent-acp CLAUDE_AGENT_ACP_VERSION=<version>` for local
reproducibility. CI never installs the adapter (it runs the OpenAI agent), so this
knob is purely local.

## Expected results

A successful run reports tasks passed, assertions passed (the expected tools were
called), and verification passed (the cluster ends in the expected state). Results
are written to `mcpchecker-<eval-name>-out.json`.
