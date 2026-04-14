# Core Eval Testing

Eval configurations for running **core** task suites (kubernetes, config, helm) across different LLM providers and agent types.

## Naming

"Core" refers to the three foundational task suites: `kubernetes`, `config`, and `helm`. Each subdirectory contains eval configs for all three suites using a specific provider/agent combination.

## Directory structure

Each subdirectory follows the pattern `<agent-type>-<provider>/`:

- `builtin-*` — uses the built-in `llm-agent` with the provider's API directly
- `acp-*` — uses the Agent Communication Protocol (ACP) with an external agent process

Each contains:
- `agent.yaml` — agent configuration (model, type)
- `eval-kubernetes.yaml` — eval config for the `kubernetes` task suite
- `eval-config.yaml` — eval config for the `config` task suite
- `eval-helm.yaml` — eval config for the `helm` task suite

## Design decisions

- **Shared tasks**: All eval configs reference the same task definitions via `../../tasks/*/*/*.yaml` with `labelSelector` to filter by suite.
- **Shared MCP config**: All configs use `../../mcp-config.yaml` to connect to the same MCP server instance.
- **Per-suite eval files**: Separate eval files per suite (instead of one combined file) allow running suites independently and setting different assertions (e.g., `maxToolCalls`).
