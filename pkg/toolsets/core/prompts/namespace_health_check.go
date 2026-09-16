package prompts

import (
	"fmt"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	klogutil "github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/tasks"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// InitNamespaceHealthChecks returns the namespace-scoped health check prompts.
func InitNamespaceHealthChecks() []api.ServerPrompt {
	return []api.ServerPrompt{initNamespaceHealthCheck()}
}

// initNamespaceHealthCheck returns the namespace-scoped health check prompt definition.
func initNamespaceHealthCheck() api.ServerPrompt {
	return api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        "namespace-health-check",
			Title:       "Namespace Health Check",
			Description: "Perform health assessment of workloads, PVCs, and events in a specific namespace",
			Arguments: []api.PromptArgument{
				{
					Name:        "namespace",
					Description: "Namespace to check (required)",
					Required:    true,
				},
				{
					Name:        "check_events",
					Description: "Include recent warning/error events (true/false, default: true)",
					Required:    false,
				},
			},
		},
		Handler: namespaceHealthCheckHandler,
	}
}

// namespaceHealthCheckHandler implements the namespace health check prompt.
// It orchestrates three phases: gather -> analyze -> render, scoped to a single namespace.
func namespaceHealthCheckHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]
	if namespace == "" {
		return nil, fmt.Errorf("namespace parameter is required")
	}
	checkEvents := parseCheckEvents(args["check_events"])
	collectionTime := time.Now()

	klogutil.FromContext(params.Context).Info("Starting namespace health check...", "namespace", namespace)

	// — Phase 1: Gather (parallel API calls, scoped to namespace) —
	gatherStart := time.Now()
	gather := tasks.New(params.Context, perCheckTimeout)

	// Check namespace existence. Only a definitive NotFound aborts further
	// checks. Other errors (Forbidden, Unauthorized, etc.) are surfaced as
	// warnings while workload analysis proceeds - the user may still have
	// permissions for the individual resource APIs.
	nsResult := gather.Execute(gatherNamespaceGetTask(params, namespace))
	var nsLookupErr error
	if nsResult == nil {
		nsLookupErr = fmt.Errorf("namespace lookup task returned no result")
	} else if nsResult.Err != nil {
		nsLookupErr = nsResult.Err
	}

	nsNotFound := nsLookupErr != nil && apierrors.IsNotFound(nsLookupErr)
	if !nsNotFound {
		gather.AddTask(gatherPodTask(params, namespace))
		gather.AddTask(gatherDeploymentTask(params, namespace))
		gather.AddTask(gatherStatefulSetTask(params, namespace))
		gather.AddTask(gatherDaemonSetTask(params, namespace))
		gather.AddTask(gatherPVCTask(params, namespace))

		if checkEvents {
			gather.AddTask(gatherEventTask(params, namespace))
		}
	}

	gather.Complete()
	gatherResults := gather.All()
	logPhaseStats(params.Context, "namespace-health-check", "gather", gatherStart, gatherResults)

	// — Phase 2: Analyze (parallel analysis tasks) —
	analyzeStart := time.Now()
	analyze := tasks.New(params.Context, perCheckTimeout)

	if !nsNotFound {
		analyze.AddTask(analyzePodHealthTask(gatherResults))
		analyze.AddTask(analyzeDeploymentHealthTask(gatherResults))
		analyze.AddTask(analyzeStatefulSetHealthTask(gatherResults))
		analyze.AddTask(analyzeDaemonSetHealthTask(gatherResults))
		analyze.AddTask(analyzePVCHealthTask(gatherResults))

		if checkEvents {
			oneHourAgo := time.Now().Add(-1 * time.Hour)
			if eventGatherResult, ok := extractTaskResult(gatherResults, eventTaskName(namespace)); ok {
				analyze.AddTask(analyzeEventsTask(eventGatherResult, oneHourAgo))
			} else {
				analyze.AddTask(analyzeEventsTask(tasks.TaskResult{
					Name: eventTaskName(namespace),
					Err:  fmt.Errorf("missing gather result for events in %s", namespace),
				}, oneHourAgo))
			}
		}
	}

	analyze.Complete()
	analyzeResults := analyze.All()
	logPhaseStats(params.Context, "namespace-health-check", "analyze", analyzeStart, analyzeResults)

	// — Phase 3: Render —
	renderStart := time.Now()
	clusterContext := currentContext(params)
	promptText := formatNamespaceHealthPrompt(collectionTime, clusterContext, namespace, analyzeResults, checkEvents, nsLookupErr)
	klogutil.FromContext(params.Context).Info("namespace-health-check render phase completed", "duration", time.Since(renderStart).Round(time.Millisecond))

	return api.NewPromptCallResult(
		"Namespace health diagnostic data gathered successfully",
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
					Text: "I'll analyze the namespace health diagnostic data and provide a comprehensive assessment.",
				},
			},
		},
		nil,
	), nil
}
