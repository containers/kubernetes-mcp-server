# Core Eval Testing

Eval configurations for running task suites across different LLM providers and agent types.

## Directory structure

Each subdirectory follows the pattern `<agent-type>-<provider>/`:

- `builtin-*` — uses the built-in `llm-agent` with the provider's API directly
- `acp-*` — uses the Agent Communication Protocol (ACP) with an external agent process

Each contains:
- `agent.yaml` — agent configuration (model, type)
- `eval-core.yaml` — eval config for the `core` + `config` task suites
- `eval-helm.yaml` — eval config for the `helm` task suite (+ core/config)
- `eval-kubevirt.yaml` — eval config for the `kubevirt` task suite (+ core/config)
- `eval-kiali.yaml` — eval config for the `kiali` task suite (+ core/config)
- `eval-tekton.yaml` — eval config for the `tekton` task suite (+ core/config)
- `eval-netobserv.yaml` — eval config for the `netobserv` task suite (+ core/config)
- `eval-all.yaml` — eval config that runs all task suites
- `eval-core-readonly.yaml` (`builtin-openai` only so far) — eval config for the
  `core-readonly` suite. Proves a model can still complete real diagnostic
  workflows using only the tools exposed when the server is started with
  `--read-only`. It selects on the `readonly: "true"` label instead of
  `suite:`, reusing the read-only-compatible tasks from `core`/`config` without
  duplicating them — see [Read-only suite](../README.md#read-only-suite-core-readonly)
  for the full pattern (including why write-blocking is verified by a Go test
  instead of an eval task). Run it with:
  ```bash
  make run-server READ_ONLY=true TOOLSETS=core,config
  make run-evals SUITE=core-readonly EVAL_LABEL_SELECTOR=
  ```

Not all agent directories have every eval file yet — `builtin-openai` is the most complete set, used by the CI workflow.

## Design decisions

- **Shared tasks**: All eval configs reference the same task definitions via `../../tasks/*/*/*.yaml` with `labelSelector` to filter by suite.
- **Shared MCP config**: All configs use `../../mcp-config.yaml` to connect to the same MCP server instance.
- **Per-suite eval files**: Separate eval files per suite (instead of one combined file) allow running suites independently and setting different assertions (e.g., `maxToolCalls`).
- **Core + config always included**: Every per-suite eval file includes the `core` and `config` task sets alongside the suite-specific tasks, ensuring baseline coverage in every run.
