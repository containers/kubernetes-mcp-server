package prompts

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	klogutil "github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/tasks"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// perCheckTimeout is the maximum time allowed for the gather phase of a single
// health check. The value was chosen based on testing against clusters with
// ~6,000 pods and ~1,700 namespaces so that the gather phase completes within
// this budget.
const perCheckTimeout = 15 * time.Second

// eventCheckTimeout is the per-namespace event gather timeout. It is slightly
// shorter than perCheckTimeout so that individual event fetches complete (or
// time out) before the wrapper task's overall deadline, leaving time to
// aggregate and return partial results.
const eventCheckTimeout = perCheckTimeout - 2*time.Second

// maxEventConcurrency caps the number of concurrent namespace event-gather
// tasks to avoid exhausting client or server resources when RESTConfig.QPS is
// very large.
const maxEventConcurrency = 20

// eventConcurrencyFromQPS converts a RESTConfig QPS value into a sane
// concurrency limit for event gathering. Non-positive values fall back to 5.
func eventConcurrencyFromQPS(qps float32) int {
	concurrency := int(math.Round(float64(qps)))
	if concurrency <= 0 {
		return 5
	}
	if concurrency > maxEventConcurrency {
		return maxEventConcurrency
	}
	return concurrency
}

// InitHealthChecks returns the cluster-wide health check prompts.
func InitHealthChecks() []api.ServerPrompt {
	return []api.ServerPrompt{initClusterHealthCheck()}
}

// initClusterHealthCheck returns the cluster-wide health check prompt definition.
func initClusterHealthCheck() api.ServerPrompt {
	return api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        "cluster-health-check",
			Title:       "Cluster Health Check",
			Description: "Perform comprehensive health assessment of Kubernetes/OpenShift cluster",
			Arguments: []api.PromptArgument{
				{
					Name:        "check_events",
					Description: "Include recent warning/error events (true/false, default: true)",
					Required:    false,
				},
			},
		},
		Handler: clusterHealthCheckHandler,
	}
}

// clusterHealthCheckHandler implements the cluster health check prompt.
// It orchestrates three phases: gather -> analyze -> render.
//
// Performance note: this prompt gathers pods, deployments, statefulsets,
// daemonsets, and PVCs across all namespaces. On very large clusters this can
// be significantly more expensive than namespace-scoped checks, regardless of
// the output line budget. perCheckTimeout was chosen based on testing against
// clusters with ~6,000 pods and ~1,700 namespaces.
func clusterHealthCheckHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	checkEvents := parseCheckEvents(args["check_events"])
	collectionTime := time.Now()

	klogutil.FromContext(params.Context).Info("Starting cluster health check...")

	// — Phase 1: Gather (parallel API calls) —
	gatherStart := time.Now()
	gather := tasks.New(params.Context, perCheckTimeout)

	// Execute namespace list first (blocking) to cache for event filtering and render
	// If the list fails the error is surfaced in the rendered output and event
	// gathering is skipped (we cannot determine the relevant namespaces).
	nsListResult := gather.Execute(gatherNamespaceListTask(params))
	var cachedNamespaces []v1.Namespace
	var nsListErr error
	if nsListResult == nil {
		nsListErr = fmt.Errorf("namespace list task returned no result")
	} else if nsListResult.Err != nil {
		nsListErr = nsListResult.Err
	} else if nsList, ok := nsListResult.Output.(*v1.NamespaceList); ok && nsList != nil {
		cachedNamespaces = nsList.Items
	}

	gather.AddTask(gatherNodeTask(params))
	gather.AddTask(gatherPodTask(params, ""))
	gather.AddTask(gatherDeploymentTask(params, ""))
	gather.AddTask(gatherStatefulSetTask(params, ""))
	gather.AddTask(gatherDaemonSetTask(params, ""))
	gather.AddTask(gatherPVCTask(params, ""))

	// Detect OpenShift ClusterOperators to conditionally gather them.
	// We check for the exact GVK we will list rather than inferring OpenShift from
	// an unrelated group, so the prompt only attempts the list when the resource
	// is actually available.
	hasClusterOperator, err := api.HasGVKs(params.DiscoveryClient(), []schema.GroupVersionKind{
		{Group: "config.openshift.io", Version: "v1", Kind: "ClusterOperator"},
	})
	isOpenShift := err == nil && hasClusterOperator
	if isOpenShift {
		gather.AddTask(gatherClusterOperatorTask(params))
	}

	// Gather events from filtered namespaces using the cached namespace list
	// Skip event gathering if the namespace list is unavailable.
	if checkEvents && nsListErr == nil {
		eventNamespaces := filterEventNamespaces(cachedNamespaces)
		gather.AddTask(gatherFilteredEventTasks(params, eventNamespaces))
	}

	gather.Complete()
	gatherResults := gather.All()
	logPhaseStats(params.Context, "cluster-health-check", "gather", gatherStart, gatherResults)

	// — Phase 2: Analyze (parallel analysis tasks) —
	analyzeStart := time.Now()
	analyze := tasks.New(params.Context, perCheckTimeout)

	analyze.AddTask(analyzeNodeHealthTask(gatherResults))
	analyze.AddTask(analyzePodHealthTask(gatherResults))
	analyze.AddTask(analyzeDeploymentHealthTask(gatherResults))
	analyze.AddTask(analyzeStatefulSetHealthTask(gatherResults))
	analyze.AddTask(analyzeDaemonSetHealthTask(gatherResults))
	analyze.AddTask(analyzePVCHealthTask(gatherResults))

	if isOpenShift {
		analyze.AddTask(analyzeClusterOperatorHealthTask(gatherResults))
	}

	if checkEvents {
		oneHourAgo := time.Now().Add(-1 * time.Hour)
		// Unwrap the filteredEventGatherResult to get per-namespace EventLists.
		feg, err := extractRequiredTaskOutput[*filteredEventGatherResult](gatherResults, taskNameEventsGather)
		if err != nil {
			analyze.AddTask(analyzeEventsTask(tasks.TaskResult{
				Name: taskNameEventsGather,
				Err:  err,
			}, oneHourAgo))
		} else {
			if feg.err != nil {
				analyze.AddTask(analyzeEventsTask(tasks.TaskResult{
					Name: taskNameEventsGather,
					Err:  feg.err,
				}, oneHourAgo))
			}
			for _, r := range feg.taskResults {
				analyze.AddTask(analyzeEventsTask(r, oneHourAgo))
			}
		}
	}

	analyze.Complete()
	analyzeResults := analyze.All()
	logPhaseStats(params.Context, "cluster-health-check", "analyze", analyzeStart, analyzeResults)

	// — Phase 3: Render —
	renderStart := time.Now()
	clusterContext := currentContext(params)
	totalNamespaces := len(cachedNamespaces)
	promptText := formatClusterHealthPrompt(collectionTime, clusterContext, totalNamespaces, analyzeResults, isOpenShift, checkEvents, nsListErr)
	klogutil.FromContext(params.Context).Info("cluster-health-check render phase completed", "duration", time.Since(renderStart).Round(time.Millisecond))

	return api.NewPromptCallResult(
		"Cluster health diagnostic data gathered successfully",
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
					Text: "I'll analyze the cluster health diagnostic data and provide a comprehensive assessment.",
				},
			},
		},
		nil,
	), nil
}

// logPhaseStats logs the total wall-clock duration of a phase together with
// per-task execution timings for diagnostics.
func logPhaseStats(ctx context.Context, prompt string, phase string, phaseStart time.Time, results []tasks.TaskResult) {
	var taskStats []string
	for _, r := range results {
		status := "ok"
		if r.Err != nil {
			status = "err"
		}
		taskStats = append(taskStats, fmt.Sprintf("%s %s [%s]", r.Name, r.Duration.Round(time.Millisecond), status))
	}
	klogutil.FromContext(ctx).Info("health check phase completed",
		"prompt", prompt,
		"phase", phase,
		"duration", time.Since(phaseStart).Round(time.Millisecond),
		"taskCount", len(results),
		"taskStats", taskStats)
}

// currentContext returns the current kubeconfig context name from the
// prompt handler params. Returns an empty string if the context cannot be
// determined (e.g. in-cluster config without a meaningful context name).
func currentContext(params api.PromptHandlerParams) string {
	raw, err := params.ToRawKubeConfigLoader().RawConfig()
	if err != nil {
		return ""
	}
	return raw.CurrentContext
}

// filterEventNamespaces returns the namespace names relevant for cluster-wide
// event collection: "default", "openshift-*", and "*-system".
func filterEventNamespaces(namespaces []v1.Namespace) []string {
	filtered := []string{"default"}
	for _, ns := range namespaces {
		if strings.HasPrefix(ns.Name, "openshift-") || strings.HasSuffix(ns.Name, "-system") {
			filtered = append(filtered, ns.Name)
		}
	}
	return filtered
}

// gatherFilteredEventTasks returns a single task that fans out event collection
// across the provided namespaces using rate-limited concurrency.
func gatherFilteredEventTasks(params api.PromptHandlerParams, namespaces []string) *tasks.Task {
	return &tasks.Task{
		Name: taskNameEventsGather,
		Run: func(ctx context.Context) (any, error) {
			concurrency := eventConcurrencyFromQPS(params.RESTConfig().QPS)
			wt := tasks.NewWithLimit(ctx, eventCheckTimeout, concurrency)
			for _, ns := range namespaces {
				wt.AddTask(gatherEventTask(params, ns))
			}
			wt.Complete()

			// Return a wrapper that holds all per-namespace results. If the
			// wrapper context was cancelled (e.g. the overall gather phase hit
			// perCheckTimeout), still return whatever results were collected so
			// the renderer can show partial event data.
			result := &filteredEventGatherResult{taskResults: wt.All()}
			if err := ctx.Err(); err != nil {
				if r := len(result.taskResults); r == 0 {
					result.err = fmt.Errorf("event gather timed out before any namespace completed: %w", err)
				} else {
					result.err = fmt.Errorf("event gather did not complete in time; partial results for %d namespace(s): %w", r, err)
				}
			}
			return result, nil
		},
	}
}

// filteredEventGatherResult wraps per-namespace event gather task results
// so the cluster health check can fan them out to the analyze phase.
// err captures a wrapper-level timeout or cancellation so that partial results
// can still be rendered alongside an explanatory error.
type filteredEventGatherResult struct {
	taskResults []tasks.TaskResult
	err         error
}
