# Kubernetes MCP Server Documentation

Welcome to the Kubernetes MCP Server documentation! This directory contains guides to help you set up and use the Kubernetes MCP Server with your Kubernetes cluster and Claude Code CLI.

## Getting Started Guides

Choose the guide that matches your needs:

| Guide | Description | Best For |
|-------|-------------|----------|
| **[Getting Started with Kubernetes](GETTING_STARTED_KUBERNETES.md)** | Base setup: Create ServiceAccount, token, and kubeconfig | Everyone - **start here first** |
| **[Using with Claude Code CLI](GETTING_STARTED_CLAUDE_CODE.md)** | Configure MCP server with Claude Code CLI | Claude Code CLI users |

## Recommended Workflow

1. **Complete the base setup**: Start with [Getting Started with Kubernetes](GETTING_STARTED_KUBERNETES.md) to create a ServiceAccount and kubeconfig file
2. **Configure Claude Code**: Then follow the [Claude Code CLI guide](GETTING_STARTED_CLAUDE_CODE.md)

## Configuration

- **[Configuration Reference](configuration.md)** - Complete reference for TOML configuration files, including all options, drop-in configuration, and dynamic reload

## Toolset Guides

- **[Kiali](KIALI.md)** - Tools for Kiali ServiceMesh with Istio

## Advanced Topics

- **[OpenTelemetry Observability](OTEL.md)** - Distributed tracing and metrics configuration
- **[MCP Prompts](prompts.md)** - Custom workflow templates for AI assistants
- **[Keycloak OIDC Setup](KEYCLOAK_OIDC_SETUP.md)** - Developer guide for local Keycloak environment and testing with MCP Inspector

## Reference

- **[Main README](../README.md)** - Project overview and general information



