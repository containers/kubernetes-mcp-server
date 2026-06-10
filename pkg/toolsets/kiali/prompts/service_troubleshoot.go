package prompts

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

func InitServiceTroubleshoot() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "service-troubleshoot",
				Title:       "Troubleshoot Service Errors",
				Description: "Investigate service errors using logs, traces, and Istio configuration to identify root causes",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespace",
						Description: "Namespace where the service is deployed",
						Required:    true,
					},
					{
						Name:        "service",
						Description: "Name of the service to troubleshoot",
						Required:    true,
					},
				},
			},
			Handler: serviceTroubleshootHandler,
		},
	}
}

func serviceTroubleshootHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]
	service := args["service"]

	if namespace == "" {
		return nil, fmt.Errorf("namespace argument is required")
	}
	if service == "" {
		return nil, fmt.Errorf("service argument is required")
	}

	klog.Infof("Starting service troubleshoot prompt for %s/%s...", namespace, service)

	kiali := kialiclient.NewKiali(params, params.RESTConfig())

	logsContent := fetchKialiData(kiali, params, tools.KialiGetLogsEndpoint,
		map[string]any{"namespace": namespace, "name": service + "-v1"})

	istioContent := fetchKialiData(kiali, params, tools.KialiManageIstioConfigReadEndpoint,
		map[string]any{"namespace": namespace, "action": "list"})

	promptText := buildServiceTroubleshootPrompt(namespace, service, logsContent, istioContent)

	return api.NewPromptCallResult(
		"Service troubleshooting data gathered successfully",
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
					Text: "I'll analyze the collected data to diagnose the service issues systematically.",
				},
			},
		},
		nil,
	), nil
}

func fetchKialiData(kiali *kialiclient.Kiali, params api.PromptHandlerParams, endpoint string, args map[string]any) string {
	content, err := kiali.ExecuteRequest(params.Context, endpoint, args)
	if err != nil {
		klog.Warningf("Failed to fetch data from %s: %v", endpoint, err)
		return fmt.Sprintf("(data unavailable: %v)", err)
	}
	return content
}

func buildServiceTroubleshootPrompt(namespace, service, logsData, istioData string) string {
	return fmt.Sprintf(`# Service Troubleshooting Guide

## Target: %s in namespace %s

Use this guide to diagnose issues with the service. The relevant data has been collected below.

---

## Step 1: Workload Logs

Review the workload logs for error messages, stack traces, or unusual patterns:

%s

---

## Step 2: Istio Configuration

Review the Istio configuration (VirtualServices, DestinationRules, etc.) for routing issues, fault injection, or misconfiguration:

%s

---

## Analysis Instructions

Based on the data above, provide:

1. **Error Summary**: What errors or issues are visible in the logs?
2. **Istio Impact**: Are there any Istio configurations (fault injection, traffic routing, retries) that could cause or contribute to the errors?
3. **Root Cause**: What is the most likely root cause?
4. **Recommendations**: What actions should be taken to resolve the issue?
`, service, namespace, logsData, istioData)
}
