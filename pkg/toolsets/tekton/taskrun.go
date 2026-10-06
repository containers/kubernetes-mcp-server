package tekton

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/google/jsonschema-go/jsonschema"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
			RBAC: api.RBACBounded(
				api.RBACRequirement{
					Verbs:        []string{"get"},
					Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "tekton.dev", Resource: "taskruns"}},
					Namespace:    &api.RBACNamespace{Argument: "namespace"},
					ResourceName: &api.RBACResourceName{Argument: "name"},
				},
				api.RBACRequirement{
					Verbs:     []string{"create"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "tekton.dev", Resource: "taskruns"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
			),
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
						"step": {
							Type:        "string",
							Description: "Step name to include. If omitted, logs from all steps and sidecars are returned",
						},
						"tail": {
							Type:        "integer",
							Description: "Number of lines to retrieve from the end of the logs (Optional, default: 100)",
							Default:     api.ToRawMessage(kubernetes.DefaultTailLines),
							Minimum:     ptr.To(float64(0)),
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
			RBAC: api.RBACBounded(
				api.RBACRequirement{
					Verbs:        []string{"get"},
					Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "tekton.dev", Resource: "taskruns"}},
					Namespace:    &api.RBACNamespace{Argument: "namespace"},
					ResourceName: &api.RBACResourceName{Argument: "name"},
				},
				api.RBACRequirement{
					Verbs:     []string{"get"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "pods", Subresource: "log"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
			),
			Handler: getTaskRunLogs,
		},
	}
}

func restartTaskRun(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	namespace := p.OptionalString("namespace", params.NamespaceOrDefault(""))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to restart task run: %w", err)), nil
	}

	dynamicClient := params.DynamicClient()

	existingUnstructured, err := dynamicClient.Resource(taskRunGVR).Namespace(namespace).Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get TaskRun %s/%s: %w", namespace, name, err)), nil
	}

	// Convert to typed object to manipulate
	var existing tektonv1.TaskRun
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(existingUnstructured.Object, &existing); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to convert TaskRun from unstructured: %w", err)), nil
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

	// Convert to unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newTR)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to convert TaskRun to unstructured: %w", err)), nil
	}

	createdUnstructured, err := dynamicClient.Resource(taskRunGVR).Namespace(namespace).Create(params.Context, &unstructured.Unstructured{Object: unstructuredObj}, metav1.CreateOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create restart TaskRun for %s/%s: %w", namespace, name, err)), nil
	}

	createdName := createdUnstructured.GetName()
	return api.NewToolCallResult(fmt.Sprintf("TaskRun '%s' restarted as '%s' in namespace '%s'", name, createdName, namespace), nil), nil
}

func getTaskRunLogs(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	namespace := p.OptionalString("namespace", params.NamespaceOrDefault(""))
	stepName := p.OptionalString("step", "")
	tailInt := p.OptionalInt64("tail", kubernetes.DefaultTailLines)
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get task run logs: %w", err)), nil
	}

	dynamicClient := params.DynamicClient()

	trUnstructured, err := dynamicClient.Resource(taskRunGVR).Namespace(namespace).Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get TaskRun %s/%s: %w", namespace, name, err)), nil
	}

	// Convert to typed object to access status
	var tr tektonv1.TaskRun
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(trUnstructured.Object, &tr); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to convert TaskRun from unstructured: %w", err)), nil
	}

	b := api.NewBuilderWithBudget(api.ResultLimit(params.Context))
	collectTaskRunLogs(params, b, namespace, &tr, stepName, tailInt)
	if b.Len() == 0 {
		b.WriteString(fmt.Sprintf("No logs available for TaskRun '%s' in namespace '%s'", name, namespace))
	}
	return api.NewToolCallResult(b.String(), nil), nil
}

func collectTaskRunLogs(params api.ToolHandlerParams, sb *api.BuilderWithBudget, namespace string, tr *tektonv1.TaskRun, stepName string, tailLines int64) {
	collectTaskRunLogsWithClient(params.Context, params.KubernetesClient, sb, namespace, tr, stepName, tailLines)
}

func collectTaskRunLogsWithClient(ctx context.Context, client api.KubernetesClient, sb *api.BuilderWithBudget, namespace string, tr *tektonv1.TaskRun, stepName string, tailLines int64) {
	hasStep := stepName == ""
	for _, step := range tr.Status.Steps {
		if step.Name == stepName {
			hasStep = true
			break
		}
	}
	if !hasStep {
		return
	}
	if tr.Status.PodName == "" {
		sb.WriteString(fmt.Sprintf("TaskRun '%s' in namespace '%s' has not started a pod yet\n", tr.Name, namespace))
		return
	}
	for _, step := range tr.Status.Steps {
		if sb.Truncated() {
			return
		}
		if stepName == "" || step.Name == stepName {
			collectContainerLogs(ctx, client, sb, tr.Status.PodName, namespace, "step", step.Name, step.Container, tailLines)
		}
	}
	if stepName == "" {
		for _, sidecar := range tr.Status.Sidecars {
			if sb.Truncated() {
				return
			}
			collectContainerLogs(ctx, client, sb, tr.Status.PodName, namespace, "sidecar", sidecar.Name, sidecar.Container, tailLines)
		}
	}
}

func collectContainerLogs(ctx context.Context, client api.KubernetesClient, sb *api.BuilderWithBudget, podName, namespace, kind, name, container string, tailLines int64) {
	if sb.Truncated() {
		return
	}

	req := client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: container,
		TailLines: &tailLines,
	})
	stream, err := req.Stream(ctx)
	if err != nil {
		sb.WriteString(fmt.Sprintf("[%s: %s] error retrieving logs: %v\n", kind, name, err))
		return
	}
	defer func() {
		_ = stream.Close()
	}()

	logBytes, err := io.ReadAll(stream)
	if err != nil {
		sb.WriteString(fmt.Sprintf("[%s: %s] error reading logs: %v\n", kind, name, err))
		return
	}
	if len(logBytes) > 0 {
		sb.WriteString(fmt.Sprintf("[%s: %s]\n%s\n", kind, name, string(logBytes)))
	}
}

// appendTaskBlock collects one task into an uncapped builder, then writes that
// block once. An empty collection drops the header. suffix follows the logs
// (the prompt adds a blank line).
func appendTaskBlock(dst *api.BuilderWithBudget, header, suffix string, collect func(*api.BuilderWithBudget)) bool {
	if dst.Truncated() {
		return false
	}
	inner := api.NewBuilderWithBudget(0)
	collect(inner)
	body := inner.String()
	if strings.TrimSpace(body) == "" {
		return true
	}
	block := header + body
	if !strings.HasSuffix(block, "\n") {
		block += "\n"
	}
	block += suffix
	return !dst.WriteString(block)
}
