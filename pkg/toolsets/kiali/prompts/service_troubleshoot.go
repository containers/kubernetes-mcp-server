package prompts

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

func InitServiceTroubleshoot() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "service-troubleshoot",
				Title:       "Troubleshoot Service Errors",
				Description: "Investigate service errors using logs, traces, and Istio configuration to identify root causes",
				Arguments: []config.PromptArgument{
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
					{
						Name:        "workload",
						Description: "Optional workload or pod name to fetch logs from (if omitted, uses the service name)",
						Required:    false,
					},
				},
			},
			RBAC:    tools.PromptServiceTroubleshootRBAC(),
			Handler: serviceTroubleshootHandler,
		},
	}
}

func serviceTroubleshootHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]
	service := args["service"]
	workload := args["workload"]

	if namespace == "" {
		return nil, fmt.Errorf("namespace argument is required")
	}
	if service == "" {
		return nil, fmt.Errorf("service argument is required")
	}

	logTarget := service
	if workload != "" {
		logTarget = workload
	}

	klogutil.FromContext(params.Context).Info("Starting service troubleshoot prompt...", "namespace", namespace, "service", service)

	kiali, err := kialiclient.NewKiali(params.Config, params.RESTConfig())
	if err != nil {
		return nil, err
	}

	b := api.NewPromptBudget(params.Context, serviceTroubleshootAssistantText)
	writeServiceTroubleshootPrompt(kiali, params, namespace, service, logTarget, b)

	return api.NewPromptCallResult(
		"Service troubleshooting data gathered successfully",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: b.String(),
				},
			},
			{
				Role: "assistant",
				Content: api.PromptContent{
					Type: "text",
					Text: serviceTroubleshootAssistantText,
				},
			},
		},
		nil,
	), nil
}

const serviceTroubleshootAssistantText = "I'll analyze the collected data to diagnose the service issues systematically."

// writeServiceTroubleshootPrompt writes the guide into one prompt budget.
// The Istio config request does not start once that budget is spent.
func writeServiceTroubleshootPrompt(kiali *kialiclient.Kiali, params api.PromptHandlerParams, namespace, service, logTarget string, b *api.BuilderWithBudget) {
	_ = b.Sections(
		api.Text(fmt.Sprintf(`# Service Troubleshooting Guide

## Target: %s in namespace %s

Use this guide to diagnose issues with the service. The relevant data has been collected below.

---

## Step 1: Workload Logs

Review the workload logs for error messages, stack traces, or unusual patterns:

`, service, namespace)),
		func(b *api.BuilderWithBudget) error {
			writeKialiData(b, kiali, params, tools.KialiGetLogsEndpoint,
				map[string]any{"namespace": namespace, "name": logTarget})
			return nil
		},
		api.Text(`

---

## Step 2: Istio Configuration

Review the Istio configuration (VirtualServices, DestinationRules, etc.) for routing issues, fault injection, or misconfiguration:

`),
		func(b *api.BuilderWithBudget) error {
			writeKialiData(b, kiali, params, tools.KialiManageIstioConfigReadEndpoint,
				map[string]any{"namespace": namespace, "action": "list"})
			return nil
		},
		api.Text(`

---

## Analysis Instructions

Based on the data above, provide:

1. **Error Summary**: What errors or issues are visible in the logs?
2. **Istio Impact**: Are there any Istio configurations (fault injection, traffic routing, retries) that could cause or contribute to the errors?
3. **Root Cause**: What is the most likely root cause?
4. **Recommendations**: What actions should be taken to resolve the issue?
`),
	)
}

// writeKialiData calls a Kiali endpoint and writes the body into b.
// On error it writes a placeholder instead of failing the prompt.
// Sections decides whether this call starts.
func writeKialiData(b *api.BuilderWithBudget, kiali *kialiclient.Kiali, params api.PromptHandlerParams, endpoint string, args map[string]any) {
	if b == nil {
		return
	}
	content, err := kiali.ExecuteRequest(params.Context, endpoint, args)
	if err != nil {
		klogutil.LogWarn(klogutil.FromContext(params.Context), "Failed to fetch data from endpoint", klogutil.Field("endpoint", endpoint), klogutil.Err(err))
		b.WriteString(fmt.Sprintf("(data unavailable: %v)", err))
		return
	}
	b.WriteString(content)
}
