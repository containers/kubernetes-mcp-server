package core

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "core"
}

func (t *Toolset) GetDescription() string {
	return "Most common tools for Kubernetes management (Pods, Generic Resources, Events, etc.)"
}

func (t *Toolset) GetTools(o api.Openshift) []api.ServerTool {
	return slices.Concat(
		initEvents(),
		initNamespaces(o),
		initNodes(),
		initPods(),
		initResources(o),
	)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "cluster-health-check",
				Title:       "Cluster Health Check",
				Description: "Perform comprehensive health assessment of Kubernetes/OpenShift cluster",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to limit health check scope (default: all namespaces)",
						Required:    false,
					},
					{
						Name:        "verbose",
						Description: "Enable detailed resource-level information (true/false, default: false)",
						Required:    false,
					},
					{
						Name:        "check_events",
						Description: "Include recent warning/error events (true/false, default: true)",
						Required:    false,
					},
				},
			},
			Handler: clusterHealthCheckHandler,
		},
	}
}

func init() {
	toolsets.Register(&Toolset{})
}
