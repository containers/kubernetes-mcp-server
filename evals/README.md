# Kubernetes MCP Server Test Examples

This directory contains examples for testing the **same Kubernetes MCP server** using different AI agents.

## Structure

```
kube-mcp-server/
├── README.md                    # This file
├── mcp-config.yaml              # Shared MCP server configuration
├── tasks/                       # Shared test tasks
│   ├── create-pod.yaml
│   ├── setup.sh
│   ├── verify.sh
│   └── cleanup.sh
├── claude-code/                 # Claude Code agent configuration
│   ├── agent.yaml
│   ├── eval.yaml
│   └── eval-inline.yaml
└── openai-agent/                # OpenAI-compatible agent configuration
    ├── agent.yaml
    ├── eval.yaml
    └── eval-inline.yaml
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
- Built binaries: `gevals` and `agent`

## Running Examples

### Option 1: Claude Code

```bash
./gevals eval examples/kube-mcp-server/claude-code/eval.yaml
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
export MODEL_NAME='your-model-name'

# Run the test
./gevals eval examples/kube-mcp-server/openai-agent/eval.yaml
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

### OpenAI Agent (openai-agent/agent.yaml)
```yaml
builtin:
  type: "openai-agent"
  model: "gpt-4"
```

Uses the built-in OpenAI agent with model configuration.

## Expected Results

Both examples should produce:
- ✅ Task passed - pod created successfully
- ✅ Assertions passed - appropriate tools were called
- ✅ Verification passed - pod exists and is running

Results saved to: `gevals-<eval-name>-out.json`
