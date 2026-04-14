# Kubernetes MCP Server Test Examples

This directory contains examples for testing the **same Kubernetes MCP server** using different AI agents.

## Structure

```
kube-mcp-server/
└── evals/
    ├── README.md                        # This file
    ├── mcp-config.yaml                  # Shared MCP server configuration
    ├── core-eval-testing/               # Core eval configurations per provider/agent type
    │   ├── acp-anthropic/               # ACP agent with Anthropic
    │   ├── acp-google/                  # ACP agent with Google
    │   ├── builtin-anthropic/           # Built-in agent with Anthropic
    │   ├── builtin-google/              # Built-in agent with Google
    │   └── builtin-openai/              # Built-in agent with OpenAI
    ├── claude-code/                     # Claude Code agent configuration
    ├── openai-agent/                    # OpenAI-compatible agent configuration
    ├── results/                         # Weekly eval results
    └── tasks/                           # Shared test tasks organized by suite (see tasks/README.md)
        └── <toolset>/
            └── <task-name>/
                ├── task.yaml            # Task definition (prompt, verify, labels)
                ├── setup.sh             # Pre-task cluster setup
                ├── verify.sh            # Post-task verification
                ├── cleanup.sh           # Resource cleanup
                └── artifacts/           # Optional K8s manifests

```

## What This Tests

Both examples test the **same Kubernetes MCP server** using **shared task definitions**:
- Creates an nginx pod named `web-server` in the `create-pod-test` namespace
- Verifies the pod is running
- Validates that the agent called appropriate Kubernetes tools
- Cleans up resources

The tasks and MCP configuration are shared - only the agent configuration differs.

## Prerequisites

- Kubernetes cluster (kind, minikube, or any cluster)
- kubectl configured
- Kubernetes MCP server running at `http://localhost:8080/mcp`
- Built binaries: `mcpchecker` and `agent`

## Running Examples

### Option 1: Claude Code

```bash
mcpchecker check examples/kube-mcp-server/claude-code/eval.yaml
```

**Requirements:**
- Claude Code installed and in PATH

**Tool Usage:**
- Claude typically uses pod-specific tools like `pods_run`, `pods_create`

---

### Option 2: OpenAI-Compatible Agent (Built-in)

```bash
# Set your model credentials
export MODEL_BASE_URL='https://your-api-endpoint.com/v1'
export MODEL_KEY='your-api-key'

# Run the test
mcpchecker check examples/kube-mcp-server/openai-agent/eval.yaml
```

**Note:** Different AI models may choose different tools from the MCP server (`pods_*` or `resources_*`) to accomplish the same task. Both approaches work correctly.

## Assertions

Both examples use flexible assertions that accept either tool approach:

```yaml
toolPattern: "(pods_.*|resources_.*)"  # Accepts both pod-specific and generic resource tools
```

This makes the tests robust across different AI models that may prefer different tools.

## Key Difference: Agent Configuration

### Claude Code (claude-code/agent.yaml)
```yaml
commands:
  argTemplateMcpServer: "--mcp-config {{ .File }}"
  argTemplateAllowedTools: "mcp__{{ .ServerName }}__{{ .ToolName }}"
  runPrompt: |-
    claude {{ .McpServerFileArgs }} --print "{{ .Prompt }}"
```

### OpenAI ACP (openai-agent/agent.yaml)
```yaml
builtin:
  type: "llm-agent"
  model: "openai:gpt-5"
```

Uses the built-in OpenAI ACP agent with model configuration.

## Filtering Tasks by Suite

Tasks are organized into suites using labels. You can filter which tasks to run using the `--label-selector` flag:

### Run only Kubernetes tasks (default)

```bash
# Using OpenAI agent
mcpchecker check evals/openai-agent/eval.yaml --label-selector suite=kubernetes

# Using Claude Code agent
mcpchecker check evals/claude-code/eval.yaml --label-selector suite=kubernetes
```

**Requirements:**
- Kubernetes cluster (kind, minikube, etc.)
- MCP server running with default toolsets

### Run only Kiali tasks

```bash
# Using OpenAI agent
mcpchecker check evals/openai-agent/eval.yaml --label-selector suite=kiali

# Using Claude Code agent
mcpchecker check evals/claude-code/eval.yaml --label-selector suite=kiali
```

**Requirements:**
- Kubernetes cluster with Istio and Kiali installed
- MCP server running with kiali toolset enabled (`TOOLSETS=kiali`)

To set up Kiali infrastructure:
```bash
make setup-kiali
```

### Run all tasks

If you omit the `--label-selector` flag, the eval configuration's `labelSelector` settings in the YAML file determine which tasks run. See the taskSets section in the eval.yaml files.

## Expected Results

Both examples should produce:
- ✅ Task passed - pod created successfully
- ✅ Assertions passed - appropriate tools were called
- ✅ Verification passed - pod exists and is running

Results saved to: `mcpchecker-<eval-name>-out.json`
