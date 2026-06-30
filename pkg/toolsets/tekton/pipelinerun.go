package tekton

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/google/jsonschema-go/jsonschema"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"
)

func pipelineRunTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "tekton_pipelinerun_restart",
				Description: "Restart a Tekton PipelineRun by creating a new PipelineRun with the same spec.",
				InputSchema: pipelineRunNameSchema("Name of the PipelineRun to restart", "Namespace of the PipelineRun"),
				Annotations: api.ToolAnnotations{
					Title:           "Tekton: Restart PipelineRun",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: restartPipelineRun,
		},
		{
			Tool: api.Tool{
				Name:        "tekton_pipelinerun_cancel",
				Description: "Cancel a running Tekton PipelineRun by setting spec.status to Cancelled. Use when a PipelineRun should stop executing.",
				InputSchema: pipelineRunNameSchema("Name of the PipelineRun to cancel", "Namespace of the PipelineRun"),
				Annotations: api.ToolAnnotations{
					Title:           "Tekton: Cancel PipelineRun",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: cancelPipelineRun,
		},
		{
			Tool: api.Tool{
				Name:        "tekton_pipelinerun_logs",
				Description: "Get logs for all TaskRuns owned by a Tekton PipelineRun. Use this to inspect PipelineRun execution output without locating pods manually.",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the PipelineRun to get logs from",
						},
						"namespace": {
							Type:        "string",
							Description: "Namespace of the PipelineRun",
						},
						"tail": {
							Type:        "integer",
							Description: "Number of lines to retrieve from the end of each container log (default: 100)",
							Default:     api.ToRawMessage(kubernetes.DefaultTailLines),
							Minimum:     ptr.To(float64(0)),
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Tekton: Get PipelineRun Logs",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: getPipelineRunLogs,
		},
	}
}

func pipelineRunNameSchema(nameDescription, namespaceDescription string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"name": {
				Type:        "string",
				Description: nameDescription,
			},
			"namespace": {
				Type:        "string",
				Description: namespaceDescription,
			},
		},
		Required: []string{"name"},
	}
}

func restartPipelineRun(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	namespace := p.OptionalString("namespace", params.NamespaceOrDefault(""))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to restart pipeline run: %w", err)), nil
	}

	dynamicClient := params.DynamicClient()

	existingUnstructured, err := dynamicClient.Resource(pipelineRunGVR).Namespace(namespace).Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get PipelineRun %s/%s: %w", namespace, name, err)), nil
	}

	// Convert to typed object to manipulate
	var existing tektonv1.PipelineRun
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(existingUnstructured.Object, &existing); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to convert PipelineRun from unstructured: %w", err)), nil
	}

	newPR := &tektonv1.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1",
			Kind:       "PipelineRun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: name + "-",
		},
		Spec: existing.Spec,
	}
	newPR.Spec.Status = ""
	if existing.GenerateName != "" {
		newPR.GenerateName = existing.GenerateName
	}

	// Convert to unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newPR)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to convert PipelineRun to unstructured: %w", err)), nil
	}

	createdUnstructured, err := dynamicClient.Resource(pipelineRunGVR).Namespace(namespace).Create(params.Context, &unstructured.Unstructured{Object: unstructuredObj}, metav1.CreateOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create restart PipelineRun for %s/%s: %w", namespace, name, err)), nil
	}

	createdName := createdUnstructured.GetName()
	return api.NewToolCallResult(fmt.Sprintf("PipelineRun '%s' restarted as '%s' in namespace '%s'", name, createdName, namespace), nil), nil
}

func cancelPipelineRun(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	namespace := p.OptionalString("namespace", params.NamespaceOrDefault(""))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to cancel pipeline run: %w", err)), nil
	}

	patch := []byte(fmt.Sprintf(`{"spec":{"status":%q}}`, tektonv1.PipelineRunSpecStatusCancelled))
	if _, err := params.DynamicClient().Resource(pipelineRunGVR).Namespace(namespace).Patch(params.Context, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to cancel PipelineRun %s/%s: %w", namespace, name, err)), nil
	}

	return api.NewToolCallResult(fmt.Sprintf("PipelineRun '%s' cancelled in namespace '%s'", name, namespace), nil), nil
}

func getPipelineRunLogs(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	namespace := p.OptionalString("namespace", params.NamespaceOrDefault(""))
	tailLines := p.OptionalInt64("tail", kubernetes.DefaultTailLines)
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get pipeline run logs: %w", err)), nil
	}

	if _, err := params.DynamicClient().Resource(pipelineRunGVR).Namespace(namespace).Get(params.Context, name, metav1.GetOptions{}); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get PipelineRun %s/%s: %w", namespace, name, err)), nil
	}

	taskRuns, err := listPipelineRunTaskRuns(params, namespace, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list TaskRuns for PipelineRun %s/%s: %w", namespace, name, err)), nil
	}
	if len(taskRuns) == 0 {
		return api.NewToolCallResult(fmt.Sprintf("No TaskRuns found for PipelineRun '%s' in namespace '%s'", name, namespace), nil), nil
	}

	var sb strings.Builder
	for _, taskRun := range taskRuns {
		fmt.Fprintf(&sb, "# TaskRun: %s\n", taskRun.Name)
		collectTaskRunLogs(params, &sb, namespace, &taskRun, tailLines)
	}
	if sb.Len() == 0 {
		return api.NewToolCallResult(fmt.Sprintf("No logs available for PipelineRun '%s' in namespace '%s'", name, namespace), nil), nil
	}
	return api.NewToolCallResult(sb.String(), nil), nil
}

func listPipelineRunTaskRuns(params api.ToolHandlerParams, namespace, pipelineRunName string) ([]tektonv1.TaskRun, error) {
	return pipelineRunTaskRuns(params.Context, params.DynamicClient(), namespace, pipelineRunName)
}

func pipelineRunTaskRuns(ctx context.Context, dynamicClient dynamic.Interface, namespace, pipelineRunName string) ([]tektonv1.TaskRun, error) {
	list, err := dynamicClient.Resource(taskRunGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "tekton.dev/pipelineRun=" + pipelineRunName,
	})
	if err != nil {
		return nil, err
	}

	taskRuns := make([]tektonv1.TaskRun, 0, len(list.Items))
	for _, item := range list.Items {
		var taskRun tektonv1.TaskRun
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &taskRun); err != nil {
			return nil, err
		}
		taskRuns = append(taskRuns, taskRun)
	}
	return taskRuns, nil
}
