package tekton

import (
	"fmt"
	"io"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func taskRunTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "tekton_taskrun_restart",
				Description: "Restart a Tekton TaskRun by creating a new TaskRun with the same spec",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the TaskRun to restart",
						},
						"namespace": {
							Type:        "string",
							Description: "Namespace of the TaskRun",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Tekton: Restart TaskRun",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: restartTaskRun,
		},
		{
			Tool: api.Tool{
				Name:        "tekton_taskrun_logs",
				Description: "Get the logs from a Tekton TaskRun by resolving its underlying pod",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the TaskRun to get logs from",
						},
						"namespace": {
							Type:        "string",
							Description: "Namespace of the TaskRun",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Tekton: Get TaskRun Logs",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: getTaskRunLogs,
		},
	}
}

func restartTaskRun(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	namespace := api.OptionalString(params, "namespace", params.KubernetesClient.NamespaceOrDefault(""))

	tektonClient, err := newTektonClient(params.KubernetesClient)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Tekton client: %w", err)), nil
	}

	existing, err := tektonClient.TektonV1().TaskRuns(namespace).Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get TaskRun %s/%s: %w", namespace, name, err)), nil
	}

	newTR := &tektonv1.TaskRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1",
			Kind:       "TaskRun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: name + "-",
		},
		Spec: existing.Spec,
	}
	newTR.Spec.Status = ""
	if existing.GenerateName != "" {
		newTR.GenerateName = existing.GenerateName
	}

	created, err := tektonClient.TektonV1().TaskRuns(namespace).Create(params.Context, newTR, metav1.CreateOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create restart TaskRun for %s/%s: %w", namespace, name, err)), nil
	}

	return api.NewToolCallResult(fmt.Sprintf("TaskRun '%s' restarted as '%s' in namespace '%s'", name, created.Name, namespace), nil), nil
}

func getTaskRunLogs(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	namespace := api.OptionalString(params, "namespace", params.KubernetesClient.NamespaceOrDefault(""))

	tektonClient, err := newTektonClient(params.KubernetesClient)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Tekton client: %w", err)), nil
	}

	tr, err := tektonClient.TektonV1().TaskRuns(namespace).Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get TaskRun %s/%s: %w", namespace, name, err)), nil
	}

	if tr.Status.PodName == "" {
		return api.NewToolCallResult(fmt.Sprintf("TaskRun '%s' in namespace '%s' has not started a pod yet", name, namespace), nil), nil
	}

	logs := ""
	for _, step := range tr.Status.Steps {
		req := params.KubernetesClient.CoreV1().Pods(namespace).GetLogs(tr.Status.PodName, &corev1.PodLogOptions{
			Container: step.Container,
		})
		stream, err := req.Stream(params.Context)
		if err != nil {
			logs += fmt.Sprintf("[step: %s] error retrieving logs: %v\n", step.Name, err)
			continue
		}
		bytes, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			logs += fmt.Sprintf("[step: %s] error reading logs: %v\n", step.Name, err)
			continue
		}
		if len(bytes) > 0 {
			logs += fmt.Sprintf("[step: %s]\n%s\n", step.Name, string(bytes))
		}
	}

	if logs == "" {
		return api.NewToolCallResult(fmt.Sprintf("No logs available for TaskRun '%s' in namespace '%s'", name, namespace), nil), nil
	}

	return api.NewToolCallResult(logs, nil), nil
}
