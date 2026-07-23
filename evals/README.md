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
├── core-eval-testing/              # Agent configs and per-suite eval configs (see below)
├── results/                        # Committed eval results (baselines)
└── tasks/                          # Shared task library, grouped by suite (see tasks/README.md)
    └── <suite>/                    # core, config, helm, kiali, kubevirt, tekton, netobserv
        └── <task-name>/
            ├── <task-name>.yaml    # Task definition (prompt, verify, labels). kubevirt/tekton use task.yaml
            ├── setup.sh            # Optional pre-task cluster setup
            ├── verify.sh           # Optional post-task verification
            ├── cleanup.sh          # Optional resource cleanup
            └── artifacts/          # Optional K8s manifests
```

Tasks are grouped into suites via a `suite: <name>` label. The available suites are
`core`, `config`, `helm`, `kiali`, `kubevirt`, `tekton`, and `netobserv`.

## Prerequisites

- A Kubernetes cluster (kind, minikube, or any cluster) and `kubectl` configured for it
- The Kubernetes MCP server running at `http://localhost:8080/mcp` (see `make run-server`)
- `mcpchecker`: `make mcpchecker` installs it under `_output/tools/bin/`. The
  `make run-evals` target calls that path directly; to run the bare `mcpchecker …`
  commands shown below by hand, add `_output/tools/bin` to your `PATH`.
- For the **`acp-anthropic`** agent only:
  - The `claude` CLI (Claude Code) installed, on your `PATH`, and **signed in**.
    The agent runs through your existing Claude Code session, so your Claude
    subscription (or an `ANTHROPIC_API_KEY`) covers the agent and no separate key
    is needed. Run `claude` once interactively to log in if you have not already.
  - The `claude-agent-acp` adapter, which bridges mcpchecker to that `claude` CLI,
    installed with `make claude-agent-acp` (installs locally under
    `_output/tools/node_modules/`, so it needs Node.js/npm). `make run-evals`
    prepends `_output/tools/node_modules/.bin` to `PATH` automatically.

## Quickstart: run one suite locally with Claude Code (ACP)

This runs the `kubevirt` suite end to end with the `acp-anthropic` agent on a
Sonnet model, with **no OpenAI key required**. With this agent both the agent and
the LLM judge run on your Claude subscription, so every suite is keyless (see
[Eval configs](#eval-configs) below).

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
#    ANTHROPIC_MODEL for ACP agents. Add EVAL_TASK_FILTER=<name> to run one task.
make run-evals SUITE=kubevirt AGENT=acp-anthropic MODEL=sonnet

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
  evals/core-eval-testing/acp-anthropic/eval-kubevirt.yaml \
  --label-selector suite=kubevirt --output json
```

### Model selection

The model for the `acp-anthropic` agent is chosen with the `ANTHROPIC_MODEL`
environment variable (for example `ANTHROPIC_MODEL=sonnet`), which
`claude-agent-acp` reads as its first-priority model source. It **forces** that
model for the run, overriding whatever default your Claude Code session would
otherwise use, and it runs on your existing Claude subscription (no API key
needed). `make run-evals` sets it for you from the `MODEL` variable, so
`make run-evals ... MODEL=sonnet` is how you pin the eval to Sonnet. The same
variable also drives the judge (see [Eval configs](#eval-configs)), so agent and
judge share the model.

The `builtin-*` agents instead take their model from their `agent.yaml` plus the
`MODEL_BASE_URL` and `MODEL_KEY` environment variables.

## Running with a builtin agent

```bash
# Set your model credentials
export MODEL_BASE_URL='https://your-api-endpoint.com/v1'
export MODEL_KEY='your-api-key'

# Run the core suite with builtin-openai (this is what CI runs)
make run-evals SUITE=core AGENT=builtin-openai
# equivalently:
mcpchecker check evals/core-eval-testing/builtin-openai/eval-core.yaml --label-selector suite=core

# Or use a different builtin agent:
make run-evals SUITE=core AGENT=builtin-anthropic
make run-evals SUITE=core AGENT=builtin-google
```

Different models may pick different tools (`pods_*` or `resources_*`) for the same
task. The assertions accept either, for example `toolPattern: "(pods_.*|resources_.*)"`.

## Eval configs

`make run-evals` resolves the eval config in priority order:

1. **Per-suite override** (`evals/tasks/<suite>/<agent>/eval.yaml`) — takes
   precedence when present. No suite currently ships one; this is a hook for
   dropping in a one-off local override.
2. **Core-eval-testing** (`evals/core-eval-testing/<agent>/eval-<suite>.yaml`) —
   the canonical per-suite config for each agent, and what every suite resolves to
   today. **CI** runs `builtin-openai` here. Available agents: `builtin-openai`,
   `builtin-anthropic`, `builtin-google`, `acp-anthropic`, `acp-google` (not all
   have eval configs yet).

Each eval config's `llmJudge` references the same `agent.yaml` used by the agent
itself, so the judge reuses the agent's model and credentials — no separate judge
setup is needed.

A task is "judge-backed" when its verify phase has an `llmJudge` step —
**including the legacy `verify: contains:` short form, which is judge-evaluated
(semantic), not a literal string match.** Judge-backed task counts per suite:
`helm` 0, `core` 4, `kubevirt` 4, `tekton` 5, `config` 3, `kiali` 18. Filter
any config to one suite with `--label-selector suite=<name>`.

## Agent configuration

Agent configs live in `evals/core-eval-testing/<agent>/agent.yaml`. There are two
kinds:

**ACP agents** run through an external CLI adapter:

- **`acp-anthropic`** — uses `claude-agent-acp` (Claude Code via ACP, keyless)
- **`acp-google`** — uses `gemini --experimental-acp`

**Builtin agents** use mcpchecker's built-in LLM agent:

- **`builtin-openai`** — `openai:gpt-5` (what CI runs)
- **`builtin-anthropic`** — `anthropic:claude-sonnet-4-6`
- **`builtin-google`** — `google:gemini-3.1-pro-preview`

## Filtering tasks by suite

The `--label-selector` flag (or the `SUITE` make variable) chooses which tasks run.

```bash
# core tasks (builtin-openai is the default AGENT)
make run-evals SUITE=core

# kiali tasks (needs Istio + Kiali installed; see `make setup-kiali`)
make run-evals SUITE=kiali AGENT=acp-anthropic MODEL=sonnet

# netobserv tasks (needs mock plugin or real NetObserv; see tasks/netobserv/README.md)
make run-evals SUITE=netobserv AGENT=builtin-openai
```

If you omit `--label-selector`, the eval config's own `labelSelector` settings in
each `taskSets` entry determine which tasks run.

Note: with `AGENT=acp-anthropic` every suite is keyless (agent and judge both run
on your Claude subscription). With `builtin-*` agents the judge-backed tasks need
the provider's API key (see [Eval configs](#eval-configs)).

## Versions

`make mcpchecker` installs `mcpchecker@latest`. CI runs the same binary version:
`.github/workflows/mcpchecker.yaml` calls `mcpchecker-action` (currently pinned at
`v0.0.18`) with `mcpchecker-version: latest`. If you need to reproduce CI exactly,
pin the binary with `make mcpchecker MCPCHECKER_VERSION=<version>`.

`make claude-agent-acp` likewise installs `@agentclientprotocol/claude-agent-acp@latest`;
pin it with `make claude-agent-acp CLAUDE_AGENT_ACP_VERSION=<version>` for local
reproducibility. CI never installs the adapter (it runs `builtin-openai`), so this
knob is purely local.

## Expected results

A successful run reports tasks passed, assertions passed (the expected tools were
called), and verification passed (the cluster ends in the expected state). Results
are written to `mcpchecker-<eval-name>-out.json`.
