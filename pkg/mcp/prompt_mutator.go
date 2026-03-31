package mcp

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type PromptMutator func(prompt api.ServerPrompt) api.ServerPrompt

// WithPromptTargetParameter adds a target selection argument to the prompt if it is cluster-aware
func WithPromptTargetParameter(defaultTarget, targetParameterName string, isMultiTarget bool) PromptMutator {
	return func(prompt api.ServerPrompt) api.ServerPrompt {
		if !prompt.IsClusterAware() {
			return prompt
		}

		if isMultiTarget {
			prompt.Prompt.Arguments = append(prompt.Prompt.Arguments, api.PromptArgument{
				Name: targetParameterName,
				Description: fmt.Sprintf(
					"Optional parameter selecting which %s to run the prompt in. Defaults to %s if not set",
					targetParameterName,
					defaultTarget,
				),
				Required: false,
			})
		}

		return prompt
	}
}
