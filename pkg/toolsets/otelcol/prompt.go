package otelcol

// ServerPrompt provides instructions for LLMs using the OpenTelemetry Collector toolset.
const ServerPrompt = `
## OpenTelemetry Collector Configuration Workflow

When generating or modifying collector configurations:
1. List available components first - never guess component names
2. Check the component schema to understand configuration options
3. Validate configurations before presenting to the user

If the user specifies a version, use it consistently across all tool calls.`
