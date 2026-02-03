#!/usr/bin/env bash
set -e

# This task works with the MCP server's current kubeconfig
# No setup needed - the agent will query the actual kubeconfig via MCP tools
echo "Using MCP server's current kubeconfig (no setup required)"
