# Testing with Gevals

This document explains how to test the Kubernetes MCP Server using gevals.

## Prerequisites

1. **Gevals installed**

   Install gevals from the official releases or build from source:
   ```bash
   # Option 1: Download from releases
   # Visit https://github.com/genmcp/gevals/releases

   # Option 2: Build from source
   git clone https://github.com/genmcp/gevals
   cd gevals
   make build
   sudo cp gevals /usr/local/bin/
   ```

   Ensure `gevals` is in your PATH or set `GEVALS_BIN`:
   ```bash
   export GEVALS_BIN=/path/to/gevals
   ```

2. **Local environment setup** (Kind cluster)
   ```bash
   make local-env-setup
   ```

   For KubeVirt testing:
   ```bash
   make local-env-setup-kubevirt
   ```

## Available Targets

### MCP Server Management

### `make run-server`
Start the MCP server in the background and wait for it to be ready.

```bash
# Start with default port (8008)
make run-server

# Start with custom port
make run-server MCP_PORT=9000

# Start with specific toolsets
make run-server TOOLSETS=core,config,helm,kubevirt
```

This target will:
1. Build the MCP server binary
2. Start the server in the background
3. Wait for the health check endpoint to respond
4. Save the process ID to `.mcp-server.pid`

Configuration:
- `MCP_PORT`: Port for the server (default: 8008)
- `TOOLSETS`: Comma-separated list of toolsets to enable
- `MCP_HEALTH_TIMEOUT`: Health check timeout in seconds (default: 60)
- `MCP_HEALTH_INTERVAL`: Health check interval in seconds (default: 2)

### `make stop-server`
Stop the MCP server that was started by `run-server`.

```bash
make stop-server
```

This will kill the process identified in `.mcp-server.pid` and clean up the pid file.

### Gevals Testing

### `make gevals-check`
Check if gevals is available and properly configured.

```bash
make gevals-check
```

### `make gevals-run`
Run gevals tests using the OpenAI-compatible agent.

```bash
# Set your AI model credentials
export MODEL_BASE_URL='https://your-api-endpoint.com/v1'
export MODEL_KEY='your-api-key'
export MODEL_NAME='your-model-name'  # e.g., 'gpt-4', 'gemini-2.0-flash'

# Run all tests
make gevals-run

# Run only kubevirt tests
make gevals-run GEVALS_ARGS="-r kubevirt"

# Run only kubernetes tests (excluding kubevirt)
make gevals-run GEVALS_ARGS="-r '^kubernetes'"

# Run a specific test by name
make gevals-run GEVALS_ARGS="-r create-vm-basic"
```

This target will:
1. Check that gevals is available
2. Export KUBECONFIG for task verification scripts
3. Create a temporary eval configuration with corrected paths
4. Run the gevals evaluation

**Note:** The LLM judge is optional. If `JUDGE_BASE_URL`, `JUDGE_API_KEY`, or `JUDGE_MODEL_NAME` are not set, the judge will be disabled and tasks will only use their verification scripts.

### `make gevals-run-claude`
Run gevals tests using the Claude Code agent.

```bash
make gevals-run-claude
```

This target exports KUBECONFIG for task verification scripts.

Requires Claude Code to be installed and available in PATH.

## Complete Workflow

Here's a complete end-to-end workflow for testing:

### 1. Setup Environment

```bash
# Install gevals (if not already installed)
# See Prerequisites section above

# Setup local environment (Kind cluster)
make local-env-setup

# Or for KubeVirt testing
make local-env-setup-kubevirt
```

### 2. Run Tests

The gevals tests will start the MCP server automatically:

```bash
# Configure your AI model
export MODEL_BASE_URL='https://api.openai.com/v1'
export MODEL_KEY='sk-...'
export MODEL_NAME='gpt-4'  # Or 'gemini-2.0-flash' for Gemini

# Run gevals tests
make gevals-run
```

### Alternative: Manual Server Management

If you prefer to manage the MCP server manually (e.g., for debugging):

```bash
# Terminal 1: Start the MCP server
make run-server

# Terminal 2: Run gevals tests (configure MODEL_* env vars first)
make gevals-run

# When done, stop the server
make stop-server
```

This approach is useful for:
- Debugging server issues by watching logs
- Running multiple test suites against the same server instance
- Testing with specific server configurations (custom ports, toolsets)

## Filtering Tasks

You can run a subset of tasks using the `GEVALS_ARGS` variable with the `-r` flag (regular expression matching):

```bash
# Run only KubeVirt tasks
make gevals-run GEVALS_ARGS="-r kubevirt"

# Run only Kubernetes tasks (excluding KubeVirt and Kiali)
make gevals-run GEVALS_ARGS="-r '^kubernetes/'"

# Run specific task categories
make gevals-run GEVALS_ARGS="-r 'create-vm|delete-vm'"  # VM creation/deletion tasks
make gevals-run GEVALS_ARGS="-r 'create-pod'"           # Pod creation tasks

# Run a single specific task
make gevals-run GEVALS_ARGS="-r '^create-vm-basic$'"

# Combine with other gevals flags
make gevals-run GEVALS_ARGS="-r kubevirt -v"  # Verbose output for KubeVirt tasks
```

The `-r` flag uses unanchored regular expressions (like `go test -run`), so:
- `kubevirt` matches any task with "kubevirt" in the path
- `^kubernetes/` matches tasks starting with "kubernetes/"
- `create-vm-basic$` matches tasks ending with "create-vm-basic"

Available task categories:
- **kubernetes/** - Core Kubernetes operations (pods, deployments, services, etc.)
- **kubevirt/** - KubeVirt virtual machine operations
- **kiali/** - Kiali/Istio service mesh operations

## Configuration

### Environment Variables

You can customize the behavior with these environment variables:

```bash
# MCP Server configuration (for run-server target)
export MCP_PORT=8008                    # Server port (default: 8008)
export MCP_HEALTH_TIMEOUT=60           # Health check timeout in seconds
export MCP_HEALTH_INTERVAL=2           # Health check interval in seconds

# Toolsets configuration (for both run-server and gevals targets)
export TOOLSETS=core,config,helm,kubevirt  # Toolsets to enable (default: core,config,helm,kubevirt)

# Gevals binary location (default: looks in PATH)
export GEVALS_BIN=/path/to/gevals

# Model configuration (default: gemini-2.0-flash)
export MODEL_NAME=gpt-4

# Task filtering (optional - omit to run all tasks)
export GEVALS_ARGS="-r kubevirt"  # Or any gevals flag

# LLM judge configuration (optional - omit to disable judge)
export JUDGE_BASE_URL='https://api.openai.com/v1'
export JUDGE_API_KEY='sk-...'
export JUDGE_MODEL_NAME='gpt-4'
```

### In-Tree Evaluation Files

The tests use evaluation files included in this repository:
- `evals/openai-agent/eval-inline.yaml` - OpenAI-compatible agent tests
- `evals/claude-code/eval-inline.yaml` - Claude Code agent tests
- `evals/tasks/` - Shared task definitions for both agents

## How It Works

The gevals tests run against the MCP server using the STDIO transport. The Makefile:

1. **Verifies gevals installation**: Checks that gevals is available in PATH
2. **Exports KUBECONFIG**: Sets KUBECONFIG environment variable for verification scripts
3. **Creates temporary eval config**: Adjusts paths in the eval files to work from the repository root
4. **Optionally disables LLM judge**: If judge environment variables aren't set
5. **Runs gevals**: Executes the tests using the configured agent

## Troubleshooting

### "Gevals not found"

Install gevals and ensure it's in your PATH:

```bash
# Download from releases
# Visit https://github.com/genmcp/gevals/releases

# Or build from source
git clone https://github.com/genmcp/gevals
cd gevals
make build
sudo cp gevals /usr/local/bin/
```

Or set `GEVALS_BIN` to the full path:
```bash
export GEVALS_BIN=/path/to/gevals
```

### "Cluster not found" or "connection refused"

Ensure your Kind cluster is running:
```bash
# Check cluster status
kubectl cluster-info

# If cluster doesn't exist, create it
make local-env-setup
```

### Tests fail with "kubectl: command not found"

Ensure kubectl is installed and in your PATH:
```bash
# Check kubectl
kubectl version --client

# Install if needed (example for Linux)
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/
```

## Related Documentation

- [Gevals Documentation](https://github.com/genmcp/gevals)
- [MCP Server Local Development](../README.md#local-development)
- [Kind Cluster Setup](../dev/config/kind/README.md)
