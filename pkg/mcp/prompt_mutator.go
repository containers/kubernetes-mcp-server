package mcp

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type PromptMutator func(prompt api.ServerPrompt) api.ServerPrompt

// WithPromptTargetParameter adds a target selection argument to the prompt if it is cluster-aware
func WithPromptTargetParameter(defaultTarget, targetParameterName string, isMultiTarget bool) PromptMutator {
	return func(prompt api.ServerPrompt) api.ServerPrompt {
		if !prompt.IsClusterAware() {
			return prompt
		}

		if isMultiTarget {
			prompt.Prompt.Arguments = append(prompt.Prompt.Arguments, config.PromptArgument{
				Name:        targetParameterName,
				Description: fmt.Sprintf("The %s to run against, as listed by %s_list. Defaults to %s.", targetParameterName, targetParameterName, defaultTarget),
				Required:    false,
			})
		}

		return prompt
	}
}
