package prompts

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

func InitListApplications(ctx context.Context, inspector api.ClusterInspector) []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "mesh-list-applications",
				Title:       "List Mesh Applications",
				Description: "List applications in the mesh namespaces",
				Arguments: []config.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to filter applications (default: all namespaces)",
						Required:    false,
					},
				},
			},
			RBAC:    tools.ResourcesRBAC(ctx, inspector),
			Handler: listResourceHandler("app"),
		},
	}
}

func InitListNamespaces(ctx context.Context, inspector api.ClusterInspector) []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "mesh-list-namespaces",
				Title:       "List Mesh Namespaces",
				Description: "List all namespaces with their sidecar injection status and Istio labels",
			},
			RBAC:    tools.ResourcesRBAC(ctx, inspector),
			Handler: listResourceHandler("namespace"),
		},
	}
}

func InitListServices(ctx context.Context, inspector api.ClusterInspector) []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "mesh-list-services",
				Title:       "List Mesh Services",
				Description: "List services in the mesh namespaces",
				Arguments: []config.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to filter services (default: all namespaces)",
						Required:    false,
					},
				},
			},
			RBAC:    tools.ResourcesRBAC(ctx, inspector),
			Handler: listResourceHandler("service"),
		},
	}
}

func InitListWorkloads(ctx context.Context, inspector api.ClusterInspector) []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "mesh-list-workloads",
				Title:       "List Mesh Workloads",
				Description: "List workloads in the mesh namespaces",
				Arguments: []config.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to filter workloads (default: all namespaces)",
						Required:    false,
					},
				},
			},
			RBAC:    tools.ResourcesRBAC(ctx, inspector),
			Handler: listResourceHandler("workload"),
		},
	}
}

func InitListIstioConfig() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "list-istio-config",
				Title:       "List Istio Configuration",
				Description: "List Istio configuration resources in the mesh namespaces",
				Arguments: []config.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to filter Istio configuration (default: all namespaces)",
						Required:    false,
					},
				},
			},
			RBAC:    tools.IstioConfigReadRBAC(),
			Handler: listIstioConfigHandler,
		},
	}
}

func listIstioConfigHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]

	klogutil.FromContext(params.Context).Info("Starting list istio config prompt...")

	reqArgs := map[string]any{"action": "list"}
	if namespace != "" {
		reqArgs["namespace"] = namespace
	}

	kiali, err := kialiclient.NewKiali(params.Config, params.RESTConfig())
	if err != nil {
		return nil, err
	}
	content, err := kiali.ExecuteRequest(params.Context, tools.KialiManageIstioConfigReadEndpoint, reqArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to list istio config: %w", err)
	}

	scope := "all namespaces"
	if namespace != "" {
		scope = fmt.Sprintf("namespace '%s'", namespace)
	}

	promptText := fmt.Sprintf(`# List Istio Configuration

## Scope
%s

## Data

%s

## Instructions

Summarize the Istio configuration listed above. Highlight any that need attention.
`, scope, content)

	return api.NewPromptCallResult(
		"Istio configuration data retrieved successfully",
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

func InitMeshTopology() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "mesh-topology",
				Title:       "Mesh Topology Overview",
				Description: "Show the mesh topology including control plane components and cluster connectivity",
			},
			RBAC:    tools.MeshStatusRBAC(),
			Handler: meshTopologyHandler,
		},
	}
}

func listResourceHandler(resourceType string) api.PromptHandlerFunc {
	return func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
		args := params.GetArguments()
		namespace := args["namespace"]

		klogutil.FromContext(params.Context).Info("Starting list prompt...", "resourceType", resourceType)

		reqArgs := map[string]any{"resourceType": resourceType}
		if namespace != "" {
			reqArgs["namespaces"] = namespace
		}

		kiali, err := kialiclient.NewKiali(params.Config, params.RESTConfig())
		if err != nil {
			return nil, err
		}
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
	klogutil.FromContext(params.Context).Info("Starting mesh topology prompt...")

	kiali, err := kialiclient.NewKiali(params.Config, params.RESTConfig())
	if err != nil {
		return nil, err
	}

	b := api.NewBuilderWithBudget(api.ResultLimit(params.Context))
	if err := writeMeshTopologyPrompt(kiali, params, b); err != nil {
		return nil, err
	}

	return api.NewPromptCallResult(
		"Mesh topology data gathered successfully",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: b.String(),
				},
			},
		},
		nil,
	), nil
}

// writeMeshTopologyPrompt writes the overview into one prompt budget.
// Namespace discovery and the traffic graph do not start once that budget is spent.
func writeMeshTopologyPrompt(kiali *kialiclient.Kiali, params api.PromptHandlerParams, b *api.BuilderWithBudget) error {
	b.WriteString(`# Mesh Topology Overview

## Mesh Status
`)
	if b.Spent() {
		return nil
	}
	writeKialiData(b, kiali, params, tools.KialiGetMeshStatusEndpoint, nil)
	if b.Spent() {
		return nil
	}
	resolved, err := resolveNamespaces(kiali, params, allNamespacesKeyword)
	if err != nil {
		return fmt.Errorf("failed to resolve mesh namespaces: %w", err)
	}
	b.WriteString("\n\n## Traffic Graph\n")
	if b.Spent() {
		return nil
	}
	writeKialiData(b, kiali, params, tools.KialiGetMeshTrafficGraphEndpoint,
		map[string]any{"namespaces": resolved})
	if b.Spent() {
		return nil
	}
	b.WriteString(`

## Instructions

Summarize the mesh topology covering control plane components, cluster connectivity, and data plane overview.
`)
	return nil
}
