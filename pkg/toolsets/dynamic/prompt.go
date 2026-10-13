package dynamic

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// compiledPrompt is a validated Prompt definition: optional gathering steps
// (run with the same engine as tools) and message templates.
type compiledPrompt struct {
	name     string
	spec     PromptSpec
	runner   *compiledTool
	messages []compiledMessage
}

type compiledMessage struct {
	role string
	text *template
	when *expression
}

func newCompiledPrompt(prompt Prompt) (*compiledPrompt, error) {
	if err := prompt.Validate(); err != nil {
		return nil, err
	}
	runner, err := compileStepsOnly(prompt.Spec.Steps)
	if err != nil {
		return nil, err
	}
	c := &compiledPrompt{name: prompt.PromptName(), spec: prompt.Spec, runner: runner}
	for i, message := range prompt.Spec.Messages {
		text, err := parseTemplate(message.Text)
		if err != nil {
			return nil, fmt.Errorf("spec.messages[%d].text: %w", i, err)
		}
		compiled := compiledMessage{role: message.Role, text: text}
		if message.When != "" {
			if compiled.when, err = compileExpression(message.When); err != nil {
				return nil, fmt.Errorf("spec.messages[%d].when: %w", i, err)
			}
		}
		c.messages = append(c.messages, compiled)
	}
	return c, nil
}

// serverPrompt converts the definition into the ServerPrompt the MCP server
// registers.
func (c *compiledPrompt) serverPrompt(ctx context.Context, discovery api.AggregateDiscovery) api.ServerPrompt {
	arguments := make([]config.PromptArgument, 0, len(c.spec.Arguments))
	for _, argument := range c.spec.Arguments {
		arguments = append(arguments, config.PromptArgument{Name: argument.Name, Description: argument.Description, Required: argument.Required})
	}
	return api.ServerPrompt{
		Prompt: config.Prompt{
			Name:        c.name,
			Title:       c.spec.Title,
			Description: c.spec.Description,
			Arguments:   arguments,
		},
		RBAC:    c.runner.rbac(ctx, discovery),
		Handler: c.handle,
	}
}

func (c *compiledPrompt) handle(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	raw := params.GetArguments()
	vars := evalVars{args: make(map[string]any, len(raw)), steps: map[string]any{}}
	for key, value := range raw {
		vars.args[key] = value
	}
	for _, argument := range c.spec.Arguments {
		if argument.Required {
			if value, ok := raw[argument.Name]; !ok || value == "" {
				return nil, fmt.Errorf("%s argument is required", argument.Name)
			}
		}
	}
	env := runEnv{ctx: params, client: params.KubernetesClient, elicitor: params.Elicitor}
	if _, _, err := c.runner.runSteps(env, &vars); err != nil {
		return nil, fmt.Errorf("%s failed: %w", c.name, err)
	}
	messages := make([]api.PromptMessage, 0, len(c.messages))
	for _, message := range c.messages {
		if message.when != nil {
			value, err := message.when.eval(vars)
			if err != nil {
				return nil, fmt.Errorf("%s failed: %w", c.name, err)
			}
			if include, ok := value.(bool); !ok || !include {
				continue
			}
		}
		text, err := message.text.render(vars)
		if err != nil {
			return nil, fmt.Errorf("%s failed: %w", c.name, err)
		}
		messages = append(messages, api.PromptMessage{Role: message.role, Content: api.PromptContent{Type: "text", Text: text}})
	}
	return api.NewPromptCallResult(c.spec.Description, messages, nil), nil
}
