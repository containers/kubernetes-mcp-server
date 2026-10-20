package tekton

import (
	"fmt"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
)

func pipelineTroubleshootPrompts() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "pipeline-troubleshoot",
				Title:       "Tekton PipelineRun Troubleshoot",
				Description: "Gather PipelineRun status, its Pipeline definition, TaskRuns, failed or errored step logs, warning events, Pipeline-as-Code Repository, and TektonConfig context for Tekton troubleshooting",
				Arguments: []config.PromptArgument{
					{
						Name:        "namespace",
						Description: "Namespace of the PipelineRun to troubleshoot",
						Required:    true,
					},
					{
						Name:        "name",
						Description: "Name of the PipelineRun to troubleshoot",
						Required:    true,
					},
				},
			},
			RBAC: api.RBACBounded(
				api.RBACRequirement{
					Verbs:        []string{"get"},
					Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "tekton.dev", Resource: "pipelineruns"}},
					Namespace:    &api.RBACNamespace{Argument: "namespace"},
					ResourceName: &api.RBACResourceName{Argument: "name"},
				},
				api.RBACRequirement{
					Verbs:     []string{"get"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "tekton.dev", Resource: "pipelines"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "tekton.dev", Resource: "taskruns"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"get"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "pods", Subresource: "log"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "events"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "pipelinesascode.tekton.dev", Resource: "repositories"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:  []string{"list"},
					Target: api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "operator.tekton.dev", Resource: "tektonconfigs"}},
				},
			),
			Handler: pipelineTroubleshootHandler,
		},
	}
}

const pipelineTroubleshootAssistantText = "I'll analyze the collected Tekton data to identify the PipelineRun issue."

func pipelineTroubleshootHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]
	name := args["name"]
	if namespace == "" {
		return nil, fmt.Errorf("namespace argument is required")
	}
	if name == "" {
		return nil, fmt.Errorf("name argument is required")
	}

	b := api.NewPromptBudget(params.Context, pipelineTroubleshootAssistantText)
	writePipelineTroubleshootGuide(params, namespace, name, b)
	return pipelineTroubleshootResult(b.String(), true), nil
}

// writePipelineTroubleshootGuide writes the guide into one prompt budget.
// Each get or list waits until the previous section still fits. The log and
// event collectors are unchanged and write into the room that remains.
func writePipelineTroubleshootGuide(params api.PromptHandlerParams, namespace, name string, b *api.BuilderWithBudget) {
	var pipelineRun *unstructured.Unstructured
	var pipelineRunText string
	var taskRuns []tektonv1.TaskRun
	_ = b.Sections(
		func(b *api.BuilderWithBudget) error {
			pipelineRun, pipelineRunText = fetchPipelineRunForPrompt(params, namespace, name)
			b.WriteString(fmt.Sprintf(`# Tekton PipelineRun Troubleshooting Guide

## PipelineRun: %s/%s

**Collected:** %s
**Current status hint:** %s

Analyze the collected data and use these exact section headings in the response:
1. PipelineRun Status - overall status, conditions, and timing
2. TaskRun Status - per-task pass, failure, or blocked state
3. Failed Step Logs - errors from failed or errored steps
4. Events - related warnings and errors
5. Troubleshooting Analysis - diagnosis based on the Pipeline definition and collected context
6. Fix Suggestions - concrete next actions for the identified failure pattern

---

## PipelineRun Status

`, namespace, name, time.Now().Format(time.RFC3339), pipelineRunStatusHint(pipelineRun)))
			return nil
		},
		func(b *api.BuilderWithBudget) error {
			b.WriteString(pipelineRunText)
			return nil
		},
		api.Text("\n\n---\n\n## Pipeline Definition\n\n"),
		func(b *api.BuilderWithBudget) error {
			b.WriteString(fetchPipelineDefinitionForPrompt(params, namespace, pipelineRun))
			return nil
		},
		api.Text("\n\n---\n\n## TaskRun Status\n\n"),
		func(b *api.BuilderWithBudget) error {
			taskRuns = fetchPipelineRunTaskRunsForPrompt(params, namespace, name, b)
			return nil
		},
		api.Text("\n\n---\n\n## Failed Step Logs\n\n"),
		func(b *api.BuilderWithBudget) error {
			fetchPipelineRunLogsForPrompt(params, namespace, taskRuns, b)
			return nil
		},
		api.Text("\n\n---\n\n## Pipeline-as-Code Repositories\n\n"),
		func(b *api.BuilderWithBudget) error {
			writePipelineRunPACRepositoriesForPrompt(params, namespace, b)
			return nil
		},
		api.Text("\n\n---\n\n## TektonConfig\n\n"),
		func(b *api.BuilderWithBudget) error {
			writeTektonConfigsForPrompt(params, b)
			return nil
		},
		api.Text("\n\n---\n\n## Events\n\n"),
		func(b *api.BuilderWithBudget) error {
			var pipelineRunObject metav1.Object
			if pipelineRun != nil {
				pipelineRunObject = pipelineRun
			}
			fetchPipelineRunEventsForPrompt(params, namespace, name, pipelineRunObject, taskRuns, b)
			return nil
		},
		api.Text(`

---

## Troubleshooting Analysis

Use the collected data to identify:
1. The first failed or blocked PipelineTask
2. The failed step and its relevant log error
3. Pipeline definition, parameter, workspace, dependency, or scheduling problems
4. Warning events that confirm the failure path
5. Pipeline-as-Code or TektonConfig settings that contributed to the failure

---

## Fix Suggestions

Recommend only fixes supported by the collected evidence. Common actions include correcting missing Pipeline or Task references, invalid parameters or workspaces, image pull failures, insufficient permissions or resources, and timeout or cancellation settings.
`),
	)
}

func pipelineRunStatusHint(pipelineRun *unstructured.Unstructured) string {
	statusHint := "unknown"
	if pipelineRun == nil {
		return statusHint
	}
	conditions, found, _ := unstructured.NestedSlice(pipelineRun.Object, "status", "conditions")
	if !found || len(conditions) == 0 {
		return statusHint
	}
	condition, ok := conditions[len(conditions)-1].(map[string]any)
	if !ok {
		return statusHint
	}
	if reason, ok := condition["reason"].(string); ok {
		statusHint = reason
	}
	return statusHint
}

func pipelineTroubleshootResult(guide string, includeAssistant bool) *api.PromptCallResult {
	messages := []api.PromptMessage{{
		Role: "user",
		Content: api.PromptContent{
			Type: "text",
			Text: guide,
		},
	}}
	if includeAssistant {
		messages = append(messages, api.PromptMessage{
			Role: "assistant",
			Content: api.PromptContent{
				Type: "text",
				Text: pipelineTroubleshootAssistantText,
			},
		})
	}
	return api.NewPromptCallResult("PipelineRun troubleshooting data gathered successfully", messages, nil)
}

func fetchPipelineRunForPrompt(params api.PromptHandlerParams, namespace, name string) (*unstructured.Unstructured, string) {
	pipelineRun, err := params.DynamicClient().Resource(pipelineRunGVR).Namespace(namespace).Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Sprintf("*Error fetching PipelineRun: %v*", err)
	}
	return pipelineRun, yamlBlock("PipelineRun", pipelineRun)
}

func fetchPipelineDefinitionForPrompt(params api.PromptHandlerParams, namespace string, pipelineRun *unstructured.Unstructured) string {
	if pipelineRun == nil {
		return "*Pipeline definition unavailable because the PipelineRun could not be fetched*"
	}

	pipelineSpec, found, err := unstructured.NestedMap(pipelineRun.Object, "spec", "pipelineSpec")
	if err != nil {
		return fmt.Sprintf("*Pipeline definition unavailable: invalid embedded PipelineSpec: %v*", err)
	}
	if found {
		return pipelineSpecBlock("Embedded", pipelineSpec)
	}

	pipelineSpec, found, err = unstructured.NestedMap(pipelineRun.Object, "status", "pipelineSpec")
	if err != nil {
		return fmt.Sprintf("*Pipeline definition unavailable: invalid resolved PipelineSpec: %v*", err)
	}
	if found {
		return pipelineSpecBlock("Resolved", pipelineSpec)
	}

	pipelineName, found, err := unstructured.NestedString(pipelineRun.Object, "spec", "pipelineRef", "name")
	if err != nil {
		return fmt.Sprintf("*Pipeline definition unavailable: invalid Pipeline reference: %v*", err)
	}
	if !found || pipelineName == "" {
		return "*Pipeline definition unavailable because the PipelineRun has no named Pipeline reference or embedded PipelineSpec*"
	}

	pipeline, err := params.DynamicClient().Resource(pipelineGVR).Namespace(namespace).Get(params.Context, pipelineName, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("*Pipeline definition unavailable: failed to fetch Pipeline %q: %v*", pipelineName, err)
	}
	return yamlBlock("Pipeline", pipeline)
}

func pipelineSpecBlock(source string, pipelineSpec map[string]interface{}) string {
	yaml, err := output.MarshalYaml(pipelineSpec)
	if err != nil {
		return fmt.Sprintf("*Error formatting %s PipelineSpec: %v*", strings.ToLower(source), err)
	}
	return fmt.Sprintf("**%s PipelineSpec**\n\n```yaml\n%s```", source, yaml)
}

// fetchPipelineRunTaskRunsForPrompt lists TaskRuns once and writes each status
// into b. Further statuses are not formatted once b is spent.
func fetchPipelineRunTaskRunsForPrompt(params api.PromptHandlerParams, namespace, pipelineRunName string, b *api.BuilderWithBudget) []tektonv1.TaskRun {
	if b == nil {
		return nil
	}
	taskRuns, _, err := pipelineRunTaskRuns(params.Context, params.DynamicClient(), namespace, pipelineRunName, "", metav1.ListOptions{})
	if err != nil {
		b.WriteString(fmt.Sprintf("*Error listing TaskRuns: %v*", err))
		return nil
	}
	if len(taskRuns) == 0 {
		b.WriteString("*No TaskRuns found for this PipelineRun*")
		return nil
	}

	steps := make([]api.Step, 0, len(taskRuns))
	for _, taskRun := range taskRuns {
		steps = append(steps, func(b *api.BuilderWithBudget) error {
			b.WriteString(fmt.Sprintf("### TaskRun: %s\n\n", taskRun.Name))
			status, err := output.MarshalYaml(taskRun.Status)
			if err != nil {
				klogutil.LogWarn(klogutil.FromContext(params.Context), "Failed to marshal TaskRun status for PipelineRun troubleshoot",
					klogutil.Field("taskrun", taskRun.Name), klogutil.Err(err))
				return nil
			}
			return b.Sections(func(b *api.BuilderWithBudget) error {
				b.WriteString(fmt.Sprintf("```yaml\n%s```\n\n", status))
				return nil
			})
		})
	}
	_ = b.Sections(steps...)
	return taskRuns
}

func fetchPipelineRunLogsForPrompt(params api.PromptHandlerParams, namespace string, taskRuns []tektonv1.TaskRun, b *api.BuilderWithBudget) {
	if b == nil || b.Truncated() {
		return
	}
	if len(taskRuns) == 0 {
		b.WriteString("*No TaskRuns found, so no failed or errored step logs are available*")
		return
	}

	stopped := false
	for _, taskRun := range taskRuns {
		header := fmt.Sprintf("### TaskRun: %s\n\n", taskRun.Name)
		if !appendTaskBlock(b, header, "\n", func(taskLogs *api.BuilderWithBudget) {
			for _, step := range taskRun.Status.Steps {
				if step.Name == "" || !failedOrErroredStep(step) {
					continue
				}
				collectTaskRunLogsWithClient(params.Context, params.KubernetesClient, taskLogs, namespace, &taskRun, step.Name, kubernetes.DefaultTailLines)
			}
		}) {
			stopped = true
			break
		}
	}
	if b.Len() == 0 && !stopped {
		b.WriteString("*No failed or errored step logs available for this PipelineRun*")
	}
	// A scratch with no room left does not write the block, so the notice
	// still has to land on the parent.
	if stopped && !b.Truncated() {
		b.WriteString(api.ResultTruncationNotice)
	}
}

func failedOrErroredStep(step tektonv1.StepState) bool {
	if step.Terminated != nil {
		return step.Terminated.ExitCode != 0 || (step.Terminated.Reason != "" && step.Terminated.Reason != "Completed")
	}
	if step.Waiting != nil {
		return step.Waiting.Reason != "" && step.Waiting.Reason != "PodInitializing" && step.Waiting.Reason != "ContainerCreating"
	}
	return false
}

type pipelineEventTarget struct {
	kind string
	name string
	uid  types.UID
}

func (t pipelineEventTarget) fieldSelector() string {
	selector := fields.Set{
		"involvedObject.kind": t.kind,
		"involvedObject.name": t.name,
		"type":                corev1.EventTypeWarning,
	}
	if t.uid != "" {
		selector["involvedObject.uid"] = string(t.uid)
	}
	return fields.SelectorFromSet(selector).String()
}

func pipelineRunEventTargets(pipelineRunName string, pipelineRun metav1.Object, taskRuns []tektonv1.TaskRun) []pipelineEventTarget {
	targets := make([]pipelineEventTarget, 0, 1+2*len(taskRuns))
	seen := make(map[pipelineEventTarget]struct{})
	add := func(target pipelineEventTarget) {
		if target.name == "" {
			return
		}
		if _, exists := seen[target]; exists {
			return
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}

	if pipelineRun != nil && pipelineRun.GetUID() != "" {
		add(pipelineEventTarget{kind: "PipelineRun", name: pipelineRunName, uid: pipelineRun.GetUID()})
	}
	for i := range taskRuns {
		if pipelineRun != nil && !metav1.IsControlledBy(&taskRuns[i], pipelineRun) {
			continue
		}
		if taskRuns[i].UID != "" {
			add(pipelineEventTarget{kind: "TaskRun", name: taskRuns[i].Name, uid: taskRuns[i].UID})
		}
		add(pipelineEventTarget{kind: "Pod", name: taskRuns[i].Status.PodName})
	}
	return targets
}

// fetchPipelineRunEventsForPrompt writes warning events into b and does not
// start the next List once b is truncated. Events are not accumulated into a
// slice first. See docs/specs/memory-bounds.md#retention.
func fetchPipelineRunEventsForPrompt(params api.PromptHandlerParams, namespace, pipelineRunName string, pipelineRun metav1.Object, taskRuns []tektonv1.TaskRun, b *api.BuilderWithBudget) {
	if b == nil || b.Truncated() {
		return
	}
	targets := pipelineRunEventTargets(pipelineRunName, pipelineRun, taskRuns)
	seen := make(map[string]struct{})
	wrote := false
	for _, target := range targets {
		if b.Truncated() {
			break
		}
		events, err := params.CoreV1().Events(namespace).List(params.Context, metav1.ListOptions{FieldSelector: target.fieldSelector()})
		if err != nil {
			klogutil.LogWarn(klogutil.FromContext(params.Context), "Failed to list events for PipelineRun troubleshoot target",
				klogutil.Field("kind", target.kind), klogutil.Field("name", target.name), klogutil.Err(err))
			continue
		}
		for _, event := range events.Items {
			if b.Truncated() {
				break
			}
			if event.Type != corev1.EventTypeWarning {
				continue
			}
			key := string(event.GetUID())
			if key == "" {
				key = event.GetNamespace() + "/" + event.GetName()
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			yaml, err := output.MarshalYaml(event)
			if err != nil {
				klogutil.LogWarn(klogutil.FromContext(params.Context), "Failed to marshal event for PipelineRun troubleshoot",
					klogutil.Field("kind", target.kind), klogutil.Field("name", target.name), klogutil.Err(err))
				continue
			}
			if !wrote {
				b.WriteString("```yaml\n")
				wrote = true
				if b.Truncated() {
					break
				}
			}
			b.WriteString(yaml)
			b.WriteString("\n")
		}
	}
	if !wrote {
		// No-op if b.Truncated()
		b.WriteString("*No related warning events found*")
		return
	}
	// No-op if b.Truncated()
	b.WriteString("```")
}

func writePipelineRunPACRepositoriesForPrompt(params api.PromptHandlerParams, namespace string, b *api.BuilderWithBudget) {
	if b == nil {
		return
	}
	list, err := params.DynamicClient().Resource(pacRepositoryGVR).Namespace(namespace).List(params.Context, metav1.ListOptions{})
	if err != nil {
		b.WriteString(fmt.Sprintf("*Pipeline-as-Code Repository resources unavailable: %v*", err))
		return
	}
	if len(list.Items) == 0 {
		b.WriteString("*No Pipeline-as-Code Repository resources found in this namespace*")
		return
	}
	yaml, err := output.MarshalYaml(list)
	if err != nil {
		b.WriteString(fmt.Sprintf("*Error formatting Pipeline-as-Code Repository resources: %v*", err))
		return
	}
	b.WriteString(fmt.Sprintf("```yaml\n%s```", yaml))
}

func writeTektonConfigsForPrompt(params api.PromptHandlerParams, b *api.BuilderWithBudget) {
	if b == nil {
		return
	}
	list, err := params.DynamicClient().Resource(tektonConfigGVR).List(params.Context, metav1.ListOptions{})
	if err != nil {
		b.WriteString(fmt.Sprintf("*TektonConfig resources unavailable: %v*", err))
		return
	}
	if len(list.Items) == 0 {
		b.WriteString("*No TektonConfig resources found*")
		return
	}
	yaml, err := output.MarshalYaml(list)
	if err != nil {
		b.WriteString(fmt.Sprintf("*Error formatting TektonConfig resources: %v*", err))
		return
	}
	b.WriteString(fmt.Sprintf("```yaml\n%s```", yaml))
}

func yamlBlock(title string, obj *unstructured.Unstructured) string {
	yaml, err := output.MarshalYaml(obj)
	if err != nil {
		return fmt.Sprintf("*Error formatting %s: %v*", title, err)
	}
	return fmt.Sprintf("```yaml\n%s```", yaml)
}
