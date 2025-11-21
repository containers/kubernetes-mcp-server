package core

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/promptsets"
)

const (
	Name        = "core"
	Description = "Core prompts for common Kubernetes/OpenShift operations including cluster health diagnostics"
)

type PromptSet struct{}

func (t *PromptSet) GetName() string {
	return Name
}

func (t *PromptSet) GetDescription() string {
	return Description
}

func (t *PromptSet) GetPrompts(o internalk8s.Openshift) []api.ServerPrompt {
	prompts := make([]api.ServerPrompt, 0)

	// Health check prompts
	prompts = append(prompts, initHealthCheckPrompts()...)

	// Future: Add more prompts here
	// prompts = append(prompts, initTroubleshootingPrompts(o)...)
	// prompts = append(prompts, initDeploymentPrompts(o)...)

	return prompts
}

func init() {
	promptsets.Register(&PromptSet{})
}
