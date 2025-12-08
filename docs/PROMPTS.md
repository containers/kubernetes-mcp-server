# MCP Prompts Support

The Kubernetes MCP Server supports [MCP Prompts](https://modelcontextprotocol.io/docs/concepts/prompts), which provide pre-defined workflow templates and guidance to AI assistants.

## What are MCP Prompts?

MCP Prompts are pre-defined templates that guide AI assistants through specific workflows. They combine:
- **Structured guidance**: Step-by-step instructions for common tasks
- **Parameterization**: Arguments that customize the prompt for specific contexts
- **Conversation templates**: Pre-formatted messages that guide the interaction

## Creating Custom Prompts

Define custom prompts in your `config.toml` file - no code changes or recompilation needed!

### Example

```toml
[[prompts]]
name = "check-pod-logs"
title = "Check Pod Logs"
description = "Quick way to check pod logs"

[[prompts.arguments]]
name = "pod_name"
description = "Name of the pod"
required = true

[[prompts.arguments]]
name = "namespace"
description = "Namespace of the pod"
required = false

[[prompts.messages]]
role = "user"
content = "Show me the logs for pod {{pod_name}} in {{namespace}}"

[[prompts.messages]]
role = "assistant"
content = "I'll retrieve and analyze the logs for you."
```

## Configuration Reference

### Prompt Fields
- **name** (required): Unique identifier for the prompt
- **title** (optional): Human-readable display name
- **description** (required): Brief explanation of what the prompt does
- **arguments** (optional): List of parameters the prompt accepts
- **messages** (required): Conversation template with role/content pairs

### Argument Fields
- **name** (required): Argument identifier
- **description** (optional): Explanation of the argument's purpose
- **required** (optional): Whether the argument must be provided (default: false)

### Argument Substitution
Use `{{argument_name}}` placeholders in message content. The template engine replaces these with actual values when the prompt is called.

## Configuration File Location

Place your prompts in the `config.toml` file used by the MCP server. Specify the config file path using the `--config` flag when starting the server.