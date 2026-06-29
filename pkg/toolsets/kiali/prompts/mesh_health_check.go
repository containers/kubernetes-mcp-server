package prompts

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

func InitMeshHealthCheck() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "mesh-health-check",
				Title:       "Mesh Health Check",
				Description: "Perform a comprehensive health assessment of the Istio service mesh including control plane and data plane status",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to focus the health check on (default: all namespaces)",
						Required:    false,
					},
				},
			},
			Handler: meshHealthCheckHandler,
		},
	}
}

func meshHealthCheckHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]

	klog.FromContext(params.Context).Info("Starting mesh health check prompt...")

	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	statusContent, err := kiali.ExecuteRequest(params.Context, tools.KialiGetMeshStatusEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve mesh status: %w", err)
	}

	promptText := buildMeshHealthPrompt(statusContent, namespace)

	return api.NewPromptCallResult(
		"Mesh health diagnostic data gathered successfully",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: promptText,
				},
			},
			{
				Role: "assistant",
				Content: api.PromptContent{
					Type: "text",
					Text: "I'll analyze the mesh health data and provide a comprehensive assessment.",
				},
			},
		},
		nil,
	), nil
}

func buildMeshHealthPrompt(statusData string, namespace string) string {
	scope := "the entire mesh"
	if namespace != "" {
		scope = fmt.Sprintf("the '%s' namespace", namespace)
	}

	return fmt.Sprintf(`# Mesh Health Check

## Scope
Analyze the health of %s.

## Collected Data

### Mesh Status
%s

## Instructions

Based on the data above, provide a comprehensive health report covering:

1. **Control Plane Status**: Is istiod running and healthy? Report any issues with the Istio control plane.
2. **Data Plane Health**: Which namespaces are healthy, degraded, or unhealthy? Summarize the overall data plane status.
3. **Observability Stack**: Are Prometheus, Grafana, and tracing backends connected and operational?
4. **Issues Found**: List any problems discovered, ordered by severity.
5. **Recommendations**: Suggest actions to resolve any issues found.
`, scope, statusData)
}
