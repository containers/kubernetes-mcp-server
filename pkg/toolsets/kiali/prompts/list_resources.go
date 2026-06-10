package prompts

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

func InitListApplications() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "list-applications",
				Title:       "List Applications",
				Description: "List applications in the mesh namespaces",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to filter applications (default: all namespaces)",
						Required:    false,
					},
				},
			},
			Handler: listResourceHandler("app"),
		},
	}
}

func InitListNamespaces() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "list-namespaces",
				Title:       "List Namespaces",
				Description: "List all namespaces with their sidecar injection status and Istio labels",
			},
			Handler: listResourceHandler("namespace"),
		},
	}
}

func InitListServices() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "list-services",
				Title:       "List Services",
				Description: "List services in the mesh namespaces",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to filter services (default: all namespaces)",
						Required:    false,
					},
				},
			},
			Handler: listResourceHandler("service"),
		},
	}
}

func InitListWorkloads() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "list-workloads",
				Title:       "List Workloads",
				Description: "List workloads in the mesh namespaces",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to filter workloads (default: all namespaces)",
						Required:    false,
					},
				},
			},
			Handler: listResourceHandler("workload"),
		},
	}
}

func InitListIstioConfig() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "list-istio-config",
				Title:       "List Istio Configuration",
				Description: "List Istio configuration resources in the mesh namespaces",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to filter Istio configuration (default: all namespaces)",
						Required:    false,
					},
				},
			},
			Handler: listResourceHandler("istio"),
		},
	}
}

func InitMeshTopology() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "mesh-topology",
				Title:       "Mesh Topology Overview",
				Description: "Show the mesh topology including control plane components and cluster connectivity",
			},
			Handler: meshTopologyHandler,
		},
	}
}

func listResourceHandler(resourceType string) api.PromptHandlerFunc {
	return func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
		args := params.GetArguments()
		namespace := args["namespace"]

		klog.Infof("Starting list %s prompt...", resourceType)

		reqArgs := map[string]any{"resourceType": resourceType}
		if namespace != "" {
			reqArgs["namespace"] = namespace
		}

		kiali := kialiclient.NewKiali(params, params.RESTConfig())
		content, err := kiali.ExecuteRequest(params.Context, tools.KialiListOrGetResourcesEndpoint, reqArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to list %s: %w", resourceType, err)
		}

		scope := "all namespaces"
		if namespace != "" {
			scope = fmt.Sprintf("namespace '%s'", namespace)
		}

		promptText := fmt.Sprintf(`# List %s

## Scope
%s

## Data

%s

## Instructions

Summarize the %s listed above. Highlight any that need attention.
`, resourceType, scope, content, resourceType)

		return api.NewPromptCallResult(
			fmt.Sprintf("%s data retrieved successfully", resourceType),
			[]api.PromptMessage{
				{
					Role: "user",
					Content: api.PromptContent{
						Type: "text",
						Text: promptText,
					},
				},
			},
			nil,
		), nil
	}
}

func meshTopologyHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	klog.Info("Starting mesh topology prompt...")

	kiali := kialiclient.NewKiali(params, params.RESTConfig())

	statusContent := fetchKialiData(kiali, params, tools.KialiGetMeshStatusEndpoint, nil)
	graphContent := fetchKialiData(kiali, params, tools.KialiGetMeshTrafficGraphEndpoint, nil)

	promptText := fmt.Sprintf(`# Mesh Topology Overview

## Mesh Status
%s

## Traffic Graph
%s

## Instructions

Summarize the mesh topology covering control plane components, cluster connectivity, and data plane overview.
`, statusContent, graphContent)

	return api.NewPromptCallResult(
		"Mesh topology data gathered successfully",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: promptText,
				},
			},
		},
		nil,
	), nil
}
