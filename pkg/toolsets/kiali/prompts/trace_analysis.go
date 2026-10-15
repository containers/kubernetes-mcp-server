package prompts

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

func InitTraceAnalysis() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "trace-analysis",
				Title:       "Trace and Latency Investigation",
				Description: "Investigate distributed traces for a service to identify latency bottlenecks, error sources, and slow spans",
				Arguments: []config.PromptArgument{
					{
						Name:        "namespace",
						Description: "Namespace where the service is deployed",
						Required:    true,
					},
					{
						Name:        "service",
						Description: "Name of the service to investigate traces for",
						Required:    true,
					},
				},
			},
			RBAC:    tools.NamespaceAccessRBAC("namespace"),
			Handler: traceAnalysisHandler,
		},
	}
}

func traceAnalysisHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]
	service := args["service"]

	if namespace == "" {
		return nil, fmt.Errorf("namespace argument is required")
	}
	if service == "" {
		return nil, fmt.Errorf("service argument is required")
	}

	klogutil.FromContext(params.Context).Info("Starting trace analysis prompt...", "namespace", namespace, "service", service)

	kiali, err := kialiclient.NewKiali(params.Config, params.RESTConfig())
	if err != nil {
		return nil, err
	}

	b := api.NewPromptBudget(params.Context, traceAnalysisAssistantText)
	writeTraceAnalysisPrompt(kiali, params, namespace, service, b)

	return api.NewPromptCallResult(
		"Trace data gathered successfully",
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
					Text: traceAnalysisAssistantText,
				},
			},
		},
		nil,
	), nil
}

const traceAnalysisAssistantText = "I'll analyze the trace data to identify latency issues and error patterns."

// writeTraceAnalysisPrompt writes the investigation into one prompt budget.
// The error-trace request does not start once that budget is spent.
func writeTraceAnalysisPrompt(kiali *kialiclient.Kiali, params api.PromptHandlerParams, namespace, service string, b *api.BuilderWithBudget) {
	b.WriteString(fmt.Sprintf(`# Trace and Latency Investigation

## Target: %s in namespace %s

Analyze distributed traces to find latency bottlenecks, error patterns, and performance issues.

---

## Recent Traces

`, service, namespace))
	if b.Spent() {
		return
	}
	writeKialiData(b, kiali, params, tools.KialiListTracesEndpoint,
		map[string]any{"namespace": namespace, "serviceName": service, "limit": 20})
	if b.Spent() {
		return
	}
	b.WriteString(`

---

## Error Traces

`)
	if b.Spent() {
		return
	}
	writeKialiData(b, kiali, params, tools.KialiListTracesEndpoint,
		map[string]any{"namespace": namespace, "serviceName": service, "errorOnly": true, "limit": 10})
	if b.Spent() {
		return
	}
	b.WriteString(`

---

## Analysis Instructions

Based on the trace data above, provide:

1. **Latency Overview**: What is the typical request duration? Are there outliers with significantly higher latency?
2. **Slowest Services**: Which downstream services or spans contribute the most latency?
3. **Error Patterns**: Are there traces with errors? What services or operations are failing?
4. **Bottlenecks**: Identify any services that appear as common slow points across multiple traces.
5. **Recommendations**: Suggest optimizations such as caching, connection pooling, retry tuning, or timeout adjustments.

If a specific trace looks problematic, mention its trace ID so the user can drill into details using the get_trace_details tool.
`)
}
