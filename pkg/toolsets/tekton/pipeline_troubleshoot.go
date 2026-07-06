package tekton

import (
	"fmt"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
)

func pipelineTroubleshootPrompts() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "pipeline-troubleshoot",
				Title:       "Tekton PipelineRun Troubleshoot",
				Description: "Gather PipelineRun status, TaskRuns, logs, events, Pipeline-as-Code Repository, and TektonConfig context for Tekton troubleshooting",
				Arguments: []api.PromptArgument{
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
			Handler: pipelineTroubleshootHandler,
		},
	}
}

type pipelineTroubleshootData struct {
	namespace        string
	name             string
	pipelineRun      *unstructured.Unstructured
	pipelineRunText  string
	taskRunsText     string
	logsText         string
	eventsText       string
	pacText          string
	tektonConfigText string
}

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

	pipelineRun, pipelineRunText := fetchPipelineRunForPrompt(params, namespace, name)
	taskRuns, taskRunsText := fetchPipelineRunTaskRunsForPrompt(params, namespace, name)
	data := pipelineTroubleshootData{
		namespace:        namespace,
		name:             name,
		pipelineRun:      pipelineRun,
		pipelineRunText:  pipelineRunText,
		taskRunsText:     taskRunsText,
		logsText:         fetchPipelineRunLogsForPrompt(params, namespace, taskRuns),
		eventsText:       fetchPipelineRunEventsForPrompt(params, namespace, name, taskRuns),
		pacText:          fetchPipelineRunPACRepositoriesForPrompt(params, namespace),
		tektonConfigText: fetchTektonConfigsForPrompt(params),
	}

	promptText := buildPipelineTroubleshootPrompt(data)
	return api.NewPromptCallResult(
		"PipelineRun troubleshooting data gathered successfully",
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
					Text: "I'll analyze the collected Tekton data to identify the PipelineRun issue.",
				},
			},
		},
		nil,
	), nil
}

func fetchPipelineRunForPrompt(params api.PromptHandlerParams, namespace, name string) (*unstructured.Unstructured, string) {
	pipelineRun, err := params.DynamicClient().Resource(pipelineRunGVR).Namespace(namespace).Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Sprintf("*Error fetching PipelineRun: %v*", err)
	}
	return pipelineRun, yamlBlock("PipelineRun", pipelineRun)
}

func fetchPipelineRunTaskRunsForPrompt(params api.PromptHandlerParams, namespace, pipelineRunName string) ([]tektonv1.TaskRun, string) {
	taskRuns, err := pipelineRunTaskRuns(params.Context, params.DynamicClient(), namespace, pipelineRunName)
	if err != nil {
		return nil, fmt.Sprintf("*Error listing TaskRuns: %v*", err)
	}
	if len(taskRuns) == 0 {
		return nil, "*No TaskRuns found for this PipelineRun*"
	}

	var sb strings.Builder
	for _, taskRun := range taskRuns {
		fmt.Fprintf(&sb, "### TaskRun: %s\n\n", taskRun.Name)
		if status, err := output.MarshalYaml(taskRun.Status); err == nil {
			fmt.Fprintf(&sb, "```yaml\n%s```\n\n", status)
		} else {
			klogutil.LogWarn(klogutil.FromContext(params.Context), "Failed to marshal TaskRun status for PipelineRun troubleshoot",
				klogutil.Field("taskrun", taskRun.Name), klogutil.Err(err))
		}
	}
	return taskRuns, sb.String()
}

func fetchPipelineRunLogsForPrompt(params api.PromptHandlerParams, namespace string, taskRuns []tektonv1.TaskRun) string {
	if len(taskRuns) == 0 {
		return "*No TaskRuns found, so no logs are available*"
	}

	var sb strings.Builder
	for _, taskRun := range taskRuns {
		var taskLogs strings.Builder
		collectTaskRunLogsWithClient(params.Context, params.KubernetesClient, &taskLogs, namespace, &taskRun, kubernetes.DefaultTailLines)
		taskLogsText := taskLogs.String()
		if strings.TrimSpace(taskLogsText) == "" {
			continue
		}
		fmt.Fprintf(&sb, "### TaskRun: %s\n\n", taskRun.Name)
		sb.WriteString(taskLogsText)
		if !strings.HasSuffix(taskLogsText, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	if sb.Len() == 0 {
		return "*No logs available for this PipelineRun*"
	}
	return sb.String()
}

type pipelineEventTarget struct {
	kind string
	name string
}

func fetchPipelineRunEventsForPrompt(params api.PromptHandlerParams, namespace, pipelineRunName string, taskRuns []tektonv1.TaskRun) string {
	targets := []pipelineEventTarget{{kind: "PipelineRun", name: pipelineRunName}}
	for _, taskRun := range taskRuns {
		targets = append(targets, pipelineEventTarget{kind: "TaskRun", name: taskRun.Name})
		if taskRun.Status.PodName != "" {
			targets = append(targets, pipelineEventTarget{kind: "Pod", name: taskRun.Status.PodName})
		}
	}

	matched := make([]corev1.Event, 0)
	seen := make(map[string]struct{})
	for _, target := range targets {
		if target.name == "" {
			continue
		}
		selector := fields.SelectorFromSet(fields.Set{
			"involvedObject.kind": target.kind,
			"involvedObject.name": target.name,
		}).String()
		events, err := params.CoreV1().Events(namespace).List(params.Context, metav1.ListOptions{FieldSelector: selector})
		if err != nil {
			klogutil.LogWarn(klogutil.FromContext(params.Context), "Failed to list events for PipelineRun troubleshoot target",
				klogutil.Field("kind", target.kind), klogutil.Field("name", target.name), klogutil.Err(err))
			continue
		}
		for _, event := range events.Items {
			key := string(event.GetUID())
			if key == "" {
				key = event.GetNamespace() + "/" + event.GetName()
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			matched = append(matched, event)
		}
	}
	if len(matched) == 0 {
		return "*No related events found*"
	}
	yaml, err := output.MarshalYaml(matched)
	if err != nil {
		return fmt.Sprintf("*Error formatting events: %v*", err)
	}
	return fmt.Sprintf("```yaml\n%s```", yaml)
}

func fetchPipelineRunPACRepositoriesForPrompt(params api.PromptHandlerParams, namespace string) string {
	list, err := params.DynamicClient().Resource(pacRepositoryGVR).Namespace(namespace).List(params.Context, metav1.ListOptions{})
	if err != nil {
		return fmt.Sprintf("*Pipeline-as-Code Repository resources unavailable: %v*", err)
	}
	if len(list.Items) == 0 {
		return "*No Pipeline-as-Code Repository resources found in this namespace*"
	}
	yaml, err := output.MarshalYaml(list)
	if err != nil {
		return fmt.Sprintf("*Error formatting Pipeline-as-Code Repository resources: %v*", err)
	}
	return fmt.Sprintf("```yaml\n%s```", yaml)
}

func fetchTektonConfigsForPrompt(params api.PromptHandlerParams) string {
	list, err := params.DynamicClient().Resource(tektonConfigGVR).List(params.Context, metav1.ListOptions{})
	if err != nil {
		return fmt.Sprintf("*TektonConfig resources unavailable: %v*", err)
	}
	if len(list.Items) == 0 {
		return "*No TektonConfig resources found*"
	}
	yaml, err := output.MarshalYaml(list)
	if err != nil {
		return fmt.Sprintf("*Error formatting TektonConfig resources: %v*", err)
	}
	return fmt.Sprintf("```yaml\n%s```", yaml)
}

func buildPipelineTroubleshootPrompt(data pipelineTroubleshootData) string {
	statusHint := "unknown"
	if data.pipelineRun != nil {
		if conditions, found, _ := unstructured.NestedSlice(data.pipelineRun.Object, "status", "conditions"); found && len(conditions) > 0 {
			if condition, ok := conditions[len(conditions)-1].(map[string]any); ok {
				statusHint, _ = condition["reason"].(string)
			}
		}
	}

	return fmt.Sprintf(`# Tekton PipelineRun Troubleshooting Guide

## PipelineRun: %s/%s

**Collected:** %s
**Current status hint:** %s

Analyze the collected data and report:
1. Overall PipelineRun state
2. Failed or blocked TaskRuns
3. Relevant log errors
4. Pipeline-as-Code Repository or TektonConfig context that may affect this run
5. Warning events
6. Recommended next action

---

## PipelineRun

%s

---

## TaskRuns

%s

---

## Logs

%s

---

## Pipeline-as-Code Repositories

%s

---

## TektonConfig

%s

---

## Events

%s
`, data.namespace, data.name, time.Now().Format(time.RFC3339), statusHint, data.pipelineRunText, data.taskRunsText, data.logsText, data.pacText, data.tektonConfigText, data.eventsText)
}

func yamlBlock(title string, obj *unstructured.Unstructured) string {
	yaml, err := output.MarshalYaml(obj)
	if err != nil {
		return fmt.Sprintf("*Error formatting %s: %v*", title, err)
	}
	return fmt.Sprintf("```yaml\n%s```", yaml)
}
