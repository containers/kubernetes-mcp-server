package core

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initPrompts() []api.ServerPrompt {
	return []api.ServerPrompt{
		troubleshootPodPrompt(),
		deployApplicationPrompt(),
		scaleDeploymentPrompt(),
		investigateClusterHealthPrompt(),
		debugNetworkingPrompt(),
		reviewResourceUsagePrompt(),
	}
}

func troubleshootPodPrompt() api.ServerPrompt {
	return api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        "troubleshoot-pod",
			Description: "Guide for troubleshooting a failing or crashed pod in Kubernetes",
			Arguments: []api.PromptArgument{
				{
					Name:        "namespace",
					Description: "The namespace where the pod is located",
					Required:    true,
				},
				{
					Name:        "pod_name",
					Description: "The name of the pod to troubleshoot",
					Required:    true,
				},
			},
		},
		Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
			args := params.GetArguments()
			namespace := args["namespace"]
			podName := args["pod_name"]

			messages := []api.PromptMessage{
				{
					Role: "user",
					Content: api.PromptContent{
						Type: "text",
						Text: fmt.Sprintf(`I need help troubleshooting a pod in Kubernetes.

Namespace: %s
Pod name: %s

Please help me investigate why this pod is failing or not working as expected.`, namespace, podName),
					},
				},
				{
					Role: "assistant",
					Content: api.PromptContent{
						Type: "text",
						Text: fmt.Sprintf(`I'll help you troubleshoot the pod %s in namespace %s. Let me investigate systematically:

1. First, I'll check the pod status and recent events
2. Then examine the pod's logs for error messages
3. Check resource constraints and limits
4. Verify the pod's configuration and health checks

Let me start by gathering information about the pod.`, podName, namespace),
					},
				},
			}

			return api.NewPromptCallResult("Guide for troubleshooting a failing or crashed pod in Kubernetes", messages, nil), nil
		},
	}
}

func deployApplicationPrompt() api.ServerPrompt {
	return api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        "deploy-application",
			Description: "Workflow for deploying a new application to Kubernetes",
			Arguments: []api.PromptArgument{
				{
					Name:        "app_name",
					Description: "The name of the application to deploy",
					Required:    true,
				},
				{
					Name:        "namespace",
					Description: "The namespace to deploy to (optional, defaults to 'default')",
					Required:    false,
				},
			},
		},
		Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
			args := params.GetArguments()
			appName := args["app_name"]
			namespace, hasNs := args["namespace"]

			userContent := fmt.Sprintf(`I want to deploy a new application to Kubernetes.

Application name: %s`, appName)
			if hasNs && namespace != "" {
				userContent += fmt.Sprintf("\nNamespace: %s", namespace)
			}
			userContent += "\n\nPlease guide me through the deployment process."

			assistantContent := fmt.Sprintf(`I'll help you deploy %s to Kubernetes. Here's the recommended workflow:

1. Create/verify the namespace`, appName)
			if hasNs && namespace != "" {
				assistantContent += fmt.Sprintf(" (%s)", namespace)
			}
			assistantContent += `
2. Review or create deployment manifests
3. Apply the deployment configuration
4. Verify the deployment status
5. Check that pods are running correctly
6. Set up services and ingress if needed

Let's start by checking the current state of the cluster and namespace.`

			messages := []api.PromptMessage{
				{
					Role: "user",
					Content: api.PromptContent{
						Type: "text",
						Text: userContent,
					},
				},
				{
					Role: "assistant",
					Content: api.PromptContent{
						Type: "text",
						Text: assistantContent,
					},
				},
			}

			return api.NewPromptCallResult("Workflow for deploying a new application to Kubernetes", messages, nil), nil
		},
	}
}

func scaleDeploymentPrompt() api.ServerPrompt {
	return api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        "scale-deployment",
			Description: "Guide for scaling a deployment up or down",
			Arguments: []api.PromptArgument{
				{
					Name:        "deployment_name",
					Description: "The name of the deployment to scale",
					Required:    true,
				},
				{
					Name:        "namespace",
					Description: "The namespace of the deployment",
					Required:    true,
				},
				{
					Name:        "replicas",
					Description: "The desired number of replicas",
					Required:    true,
				},
			},
		},
		Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
			args := params.GetArguments()
			deploymentName := args["deployment_name"]
			namespace := args["namespace"]
			replicas := args["replicas"]

			messages := []api.PromptMessage{
				{
					Role: "user",
					Content: api.PromptContent{
						Type: "text",
						Text: fmt.Sprintf(`I need to scale a deployment in Kubernetes.

Deployment: %s
Namespace: %s
Desired replicas: %s

Please help me scale this deployment safely.`, deploymentName, namespace, replicas),
					},
				},
				{
					Role: "assistant",
					Content: api.PromptContent{
						Type: "text",
						Text: fmt.Sprintf(`I'll help you scale the deployment %s to %s replicas in namespace %s.

Before scaling, let me:
1. Check the current deployment status and replica count
2. Verify the deployment is healthy
3. Scale the deployment to %s replicas
4. Monitor the scaling process
5. Verify all new pods are running correctly

Let's begin by checking the current state of the deployment.`, deploymentName, replicas, namespace, replicas),
					},
				},
			}

			return api.NewPromptCallResult("Guide for scaling a deployment up or down", messages, nil), nil
		},
	}
}

func investigateClusterHealthPrompt() api.ServerPrompt {
	return api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        "investigate-cluster-health",
			Description: "Comprehensive workflow for investigating overall cluster health",
			Arguments:   []api.PromptArgument{},
		},
		Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
			messages := []api.PromptMessage{
				{
					Role: "user",
					Content: api.PromptContent{
						Type: "text",
						Text: `I want to investigate the overall health and status of my Kubernetes cluster.
Please help me understand the cluster's current state.`,
					},
				},
				{
					Role: "assistant",
					Content: api.PromptContent{
						Type: "text",
						Text: `I'll perform a comprehensive health check of your Kubernetes cluster. Here's what I'll investigate:

1. **Node Health**: Check status of all nodes, resource usage, and any issues
2. **Critical System Pods**: Verify all system pods are running correctly
3. **Recent Events**: Review cluster-wide events for warnings or errors
4. **Resource Usage**: Check overall resource consumption across the cluster
5. **Namespace Overview**: List all namespaces and their status

Let me start by gathering information about your cluster.`,
					},
				},
			}

			return api.NewPromptCallResult("Comprehensive workflow for investigating overall cluster health", messages, nil), nil
		},
	}
}

func debugNetworkingPrompt() api.ServerPrompt {
	return api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        "debug-networking",
			Description: "Workflow for debugging networking issues between pods or services",
			Arguments: []api.PromptArgument{
				{
					Name:        "source_pod",
					Description: "The source pod name",
					Required:    false,
				},
				{
					Name:        "source_namespace",
					Description: "The source pod namespace",
					Required:    false,
				},
				{
					Name:        "target_service",
					Description: "The target service name",
					Required:    false,
				},
			},
		},
		Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
			args := params.GetArguments()
			sourcePod, hasSrcPod := args["source_pod"]
			sourceNs, hasSrcNs := args["source_namespace"]
			targetSvc, hasTgtSvc := args["target_service"]

			userContent := "I'm experiencing networking issues in my Kubernetes cluster."
			if hasSrcPod && sourcePod != "" {
				userContent += fmt.Sprintf("\nSource pod: %s", sourcePod)
			}
			if hasSrcNs && sourceNs != "" {
				userContent += fmt.Sprintf("\nSource namespace: %s", sourceNs)
			}
			if hasTgtSvc && targetSvc != "" {
				userContent += fmt.Sprintf("\nTarget service: %s", targetSvc)
			}
			userContent += "\n\nPlease help me debug the networking problem."

			messages := []api.PromptMessage{
				{
					Role: "user",
					Content: api.PromptContent{
						Type: "text",
						Text: userContent,
					},
				},
				{
					Role: "assistant",
					Content: api.PromptContent{
						Type: "text",
						Text: `I'll help you debug the networking issue. Let me investigate systematically:

1. **Pod Network Status**: Check if pods have valid IPs and are in Running state
2. **Service Configuration**: Verify service endpoints and selectors
3. **Network Policies**: Check for any network policies that might block traffic
4. **DNS Resolution**: Test if DNS is working correctly
5. **Connectivity Tests**: Perform network tests between pods

Let's start gathering diagnostic information.`,
					},
				},
			}

			return api.NewPromptCallResult("Workflow for debugging networking issues between pods or services", messages, nil), nil
		},
	}
}

func reviewResourceUsagePrompt() api.ServerPrompt {
	return api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        "review-resource-usage",
			Description: "Analyze resource usage across the cluster or specific namespace",
			Arguments: []api.PromptArgument{
				{
					Name:        "namespace",
					Description: "Optional namespace to focus on (leave empty for cluster-wide analysis)",
					Required:    false,
				},
			},
		},
		Handler: func(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
			args := params.GetArguments()
			namespace, hasNs := args["namespace"]

			userContent := "I want to review resource usage in my Kubernetes cluster."
			if hasNs && namespace != "" {
				userContent += fmt.Sprintf("\nFocus on namespace: %s", namespace)
			}
			userContent += "\n\nPlease analyze CPU and memory usage."

			assistantContent := "I'll analyze resource usage "
			if hasNs && namespace != "" {
				assistantContent += fmt.Sprintf("for namespace %s", namespace)
			} else {
				assistantContent += "across your entire cluster"
			}
			assistantContent += `.

Here's what I'll check:
1. **Node Resources**: CPU and memory capacity vs usage on nodes
2. **Pod Resources**: Resource requests and limits for pods
3. **Top Consumers**: Identify pods with highest resource usage
4. **Resource Quota**: Check if namespace quotas are defined and their usage
5. **Recommendations**: Suggest optimizations if needed

Let me gather the resource metrics.`

			messages := []api.PromptMessage{
				{
					Role: "user",
					Content: api.PromptContent{
						Type: "text",
						Text: userContent,
					},
				},
				{
					Role: "assistant",
					Content: api.PromptContent{
						Type: "text",
						Text: assistantContent,
					},
				},
			}

			return api.NewPromptCallResult("Analyze resource usage across the cluster or specific namespace", messages, nil), nil
		},
	}
}
