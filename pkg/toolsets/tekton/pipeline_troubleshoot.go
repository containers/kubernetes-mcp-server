package tekton

import (
	"fmt"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	logsText := fetchPipelineRunLogsForPrompt(params, namespace, taskRuns)
	eventsText := fetchPipelineRunEventsForPrompt(params, namespace, name, taskRuns)
	pacText := fetchPipelineRunPACRepositoriesForPrompt(params, namespace)
	tektonConfigText := fetchTektonConfigsForPrompt(params)

	promptText := buildPipelineTroubleshootPrompt(namespace, name, pipelineRun, pipelineRunText, taskRunsText, logsText, eventsText, pacText, tektonConfigText)
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
		fmt.Fprintf(&sb, "### TaskRun: %s\n\n", taskRun.Name)
		collectTaskRunLogsWithClient(params.Context, params.KubernetesClient, &sb, namespace, &taskRun, kubernetes.DefaultTailLines)
		sb.WriteString("\n")
	}
	return sb.String()
}

func fetchPipelineRunEventsForPrompt(params api.PromptHandlerParams, namespace, pipelineRunName string, taskRuns []tektonv1.TaskRun) string {
	events, err := params.CoreV1().Events(namespace).List(params.Context, metav1.ListOptions{})
	if err != nil {
		return fmt.Sprintf("*Error listing events: %v*", err)
	}

	wanted := map[string]bool{pipelineRunName: true}
	for _, taskRun := range taskRuns {
		wanted[taskRun.Name] = true
		if taskRun.Status.PodName != "" {
			wanted[taskRun.Status.PodName] = true
		}
	}

	matched := make([]corev1.Event, 0)
	for _, event := range events.Items {
		if wanted[event.InvolvedObject.Name] {
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

func buildPipelineTroubleshootPrompt(namespace, name string, pipelineRun *unstructured.Unstructured, pipelineRunText, taskRunsText, logsText, eventsText, pacText, tektonConfigText string) string {
	statusHint := "unknown"
	if pipelineRun != nil {
		if conditions, found, _ := unstructured.NestedSlice(pipelineRun.Object, "status", "conditions"); found && len(conditions) > 0 {
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
`, namespace, name, time.Now().Format(time.RFC3339), statusHint, pipelineRunText, taskRunsText, logsText, pacText, tektonConfigText, eventsText)
}

func yamlBlock(title string, obj *unstructured.Unstructured) string {
	yaml, err := output.MarshalYaml(obj)
	if err != nil {
		return fmt.Sprintf("*Error formatting %s: %v*", title, err)
	}
	return fmt.Sprintf("```yaml\n%s```", yaml)
}
