# MCP Prompts Support

The Kubernetes MCP Server supports [MCP Prompts](https://modelcontextprotocol.io/docs/concepts/prompts), which provide pre-defined workflow templates and guidance to AI assistants.

## What are MCP Prompts?

MCP Prompts are pre-defined templates that guide AI assistants through specific workflows. They combine:
- **Structured guidance**: Step-by-step instructions for common tasks
- **Parameterization**: Arguments that customize the prompt for specific contexts
- **Conversation templates**: Pre-formatted messages that guide the interaction

## Available Built-in Prompts

The server provides these prompts in the `core` toolset:

1. **troubleshoot-pod** - Debug failing or crashed pods
2. **deploy-application** - Deploy new applications
3. **scale-deployment** - Scale deployments safely
4. **investigate-cluster-health** - Check overall cluster health
5. **debug-networking** - Debug connectivity issues
6. **review-resource-usage** - Analyze resource consumption

## Creating Custom Prompts

Define custom prompts in your `config.toml` file - no code changes or recompilation needed!

### Basic Example

```toml
[[prompts]]
name = "check-pod-logs"
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

### Complex Example with Multi-Step Workflow

```toml
[[prompts]]
name = "troubleshoot-deployment"
description = "Comprehensive deployment troubleshooting"

[[prompts.arguments]]
name = "deployment_name"
required = true

[[prompts.arguments]]
name = "namespace"
required = true

[[prompts.messages]]
role = "user"
content = """
My deployment {{deployment_name}} in {{namespace}} is having issues.
Can you investigate?
"""

[[prompts.messages]]
role = "assistant"
content = """
I'll troubleshoot deployment {{deployment_name}}. Let me:

1. **Check Deployment Status**
   - Review current vs desired replicas
   - Check rollout status

2. **Investigate Pods**
   - List pod states
   - Review pod events and logs

3. **Analyze Resources**
   - Check CPU/memory limits
   - Verify quotas

Starting investigation...
"""
```

### Argument Substitution

Use `{{argument_name}}` in message content to insert values:

```toml
[[prompts.messages]]
role = "user"
content = "Check {{resource_type}} named {{resource_name}}"
```

### Overriding Built-in Prompts

Replace built-in prompts by using the same name:

```toml
[[prompts]]
name = "troubleshoot-pod"  # This overrides the built-in version
description = "Our custom pod troubleshooting process"

[[prompts.arguments]]
name = "pod_name"
required = true

[[prompts.messages]]
role = "user"
content = "Pod {{pod_name}} needs help"

[[prompts.messages]]
role = "assistant"
content = "Using our custom troubleshooting workflow..."
```

### Disabling Built-in Prompts

Use only your custom prompts:

```toml
# Disable all built-in prompts
disable_embedded_prompts = true

# Then define your own
[[prompts]]
name = "my-prompt"
# ...
```

## For Toolset Developers

If you're creating a custom toolset, define prompts directly in Go code (similar to how tools are defined):

```go
// pkg/toolsets/yourtoolset/prompts.go
package yourtoolset

import (
	"fmt"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func (t *Toolset) GetPrompts(_ internalk8s.Openshift) []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "your-prompt",
				Description: "What it does",
				Arguments: []api.PromptArgument{
					{
						Name:        "arg1",
						Description: "First argument",
						Required:    true,
					},
				},
			},
			Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
				args := params.GetArguments()
				arg1, _ := args["arg1"]

				messages := []api.PromptMessage{
					{
						Role: "user",
						Content: api.PromptContent{
							Type: "text",
							Text: fmt.Sprintf("Message with %s", arg1),
						},
					},
					{
						Role: "assistant",
						Content: api.PromptContent{
							Type: "text",
							Text: "Response template",
						},
					},
				}

				return api.NewPromptCallResult("What it does", messages, nil), nil
			},
		},
	}
}
```

