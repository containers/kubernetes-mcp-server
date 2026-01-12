# Testing with Gevals

This document explains how to test the Kubernetes MCP Server using gevals with OAuth authentication.

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

2. **Local environment setup** (Kind cluster with Keycloak)
   ```bash
   make local-env-setup
   ```

3. **MCP server running** with OAuth enabled
   ```bash
   make local-mcp-server
   ```

   The server will run on port 8008 with OAuth authentication enabled.

## Available Targets

### `make gevals-check`
Check if gevals is available and properly configured.

```bash
make gevals-check
```

### `make gevals-config`
Generate the MCP configuration file for gevals with OAuth support.

```bash
make gevals-config
```

This creates `_output/gevals/mcp-config.yaml` with:
- HTTP endpoint configuration
- OAuth Bearer token header
- Environment variable substitution

### `make gevals-get-token`
Get an OAuth token from Keycloak for testing purposes.

```bash
make gevals-get-token
```

This will:
1. Connect to the local Keycloak instance
2. Authenticate with the test user (mcp/mcp)
3. Display the access token
4. Show how to export it for manual use

### `make gevals-run`
Run gevals tests using the OpenAI-compatible agent with OAuth authentication.

```bash
# Set your AI model credentials
export MODEL_BASE_URL='https://your-api-endpoint.com/v1'
export MODEL_KEY='your-api-key'
export MODEL_NAME='your-model-name'  # e.g., 'gpt-4', 'gemini-2.0-flash'

# Run the tests
make gevals-run
```

This target will:
1. Check that gevals is available
2. Generate the MCP config
3. Obtain an OAuth token from Keycloak
4. Verify the MCP server is running
5. Run the gevals evaluation

**Note:** The LLM judge is optional. If `JUDGE_BASE_URL`, `JUDGE_API_KEY`, or `JUDGE_MODEL_NAME` are not set, the judge will be disabled and tasks will only use their verification scripts.

### `make gevals-run-claude`
Run gevals tests using the Claude Code agent with OAuth authentication.

```bash
make gevals-run-claude
```

Requires Claude Code to be installed and available in PATH.

### `make local-mcp-server`
Start the MCP server locally with OAuth enabled.

```bash
make local-mcp-server
```

The server will:
- Listen on port 8008
- Require OAuth authentication
- Use the config from `_output/config.toml`

## Complete Workflow

Here's a complete end-to-end workflow for testing:

### 1. Setup Environment

```bash
# Install gevals (if not already installed)
# See Prerequisites section above

# Setup local environment (Kind + Keycloak)
make local-env-setup
```

### 2. Start MCP Server

In one terminal:
```bash
make local-mcp-server
```

### 3. Run Tests

In another terminal:
```bash
# Configure your AI model
export MODEL_BASE_URL='https://api.openai.com/v1'
export MODEL_KEY='sk-...'
export MODEL_NAME='gpt-4'  # Or 'gemini-2.0-flash' for Gemini

# Run gevals tests
make gevals-run
```

## Configuration

### Environment Variables

You can customize the behavior with these environment variables:

```bash
# Gevals binary location (default: looks in PATH)
export GEVALS_BIN=/path/to/gevals

# MCP server port (default: 8008)
export MCP_SERVER_PORT=9000

# Model configuration (default: gemini-2.0-flash)
export MODEL_NAME=gpt-4

# LLM judge configuration (optional - omit to disable judge)
export JUDGE_BASE_URL='https://api.openai.com/v1'
export JUDGE_API_KEY='sk-...'
export JUDGE_MODEL_NAME='gpt-4'

# Keycloak credentials (defaults: mcp/mcp)
export KEYCLOAK_USERNAME=myuser
export KEYCLOAK_PASSWORD=mypassword
```

### In-Tree Evaluation Files

The tests use evaluation files included in this repository:
- `evals/openai-agent/eval-inline.yaml` - OpenAI-compatible agent tests
- `evals/claude-code/eval-inline.yaml` - Claude Code agent tests
- `evals/tasks/` - Shared task definitions for both agents

These evaluation files are automatically configured to use the OAuth-enabled MCP server.

## How It Works

### OAuth Flow

1. **Token Acquisition**: The Makefile obtains an OAuth token from Keycloak using the Resource Owner Password Credentials grant
2. **Environment Variables**: The token is exported as `MCP_ACCESS_TOKEN`
3. **Header Injection**: Gevals adds the token as a Bearer header to all MCP requests
4. **MCP Config**: The generated config uses environment variable substitution:
   ```yaml
   mcpServers:
     kubernetes:
       type: http
       url: ${MCP_SERVER_URL}
       headers:
         Authorization: Bearer ${MCP_ACCESS_TOKEN}
       enableAllTools: true
   ```

### Gevals Integration

The gevals implementation already supports OAuth through HTTP headers:
- Headers are configured in `mcp-config.yaml`
- Environment variables in header values are automatically expanded
- A custom `HeaderRoundTripper` adds headers to every HTTP request

No modifications to gevals are needed - it's purely configuration-based.

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

### "Failed to get OAuth token"

Ensure Keycloak is running:
```bash
make keycloak-status
```

If not running, recreate the environment:
```bash
make local-env-teardown
make local-env-setup
```

### "MCP server not responding"

Ensure the server is running:
```bash
# In another terminal
make local-mcp-server
```

Or check if it's listening:
```bash
curl -H "Authorization: Bearer $(make gevals-get-token 2>&1 | tail -1)" \
     http://localhost:8008/mcp
```

### Token Expired

OAuth tokens expire after 5 minutes by default. Re-run the test to get a fresh token automatically:
```bash
make gevals-run
```

## Test Credentials

The local development environment includes test credentials:

- **Username**: `mcp`
- **Password**: `mcp`
- **Client ID**: `mcp-client` (public client)
- **Realm**: `openshift`
- **Keycloak URL**: `https://keycloak.127-0-0-1.sslip.io:8443`

These are configured in the Keycloak setup (see `build/keycloak.mk`).

## Related Documentation

- [Gevals Documentation](https://github.com/genmcp/gevals)
- [MCP Server Local Development](../README.md#local-development)
- [Keycloak Setup](../dev/config/keycloak/README.md)
