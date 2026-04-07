package tekton

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

func pipelineRunTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "tekton_pipelinerun_restart",
				Description: "Restart a Tekton PipelineRun by creating a new PipelineRun with the same spec",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the PipelineRun to restart",
						},
						"namespace": {
							Type:        "string",
							Description: "Namespace of the PipelineRun",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Tekton: Restart PipelineRun",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: restartPipelineRun,
		},
	}
}

func restartPipelineRun(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	namespace := api.OptionalString(params, "namespace", params.NamespaceOrDefault(""))

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
