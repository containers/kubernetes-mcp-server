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

func taskTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "tekton_task_start",
				Description: "Start a Tekton Task by creating a TaskRun that references it",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the Task to start",
						},
						"namespace": {
							Type:        "string",
							Description: "Namespace of the Task",
						},
						"params": {
							Type:                 "object",
							Description:          "Parameter values to pass to the Task. Keys are parameter names; values can be a string, an array of strings, or an object (map of string to string) depending on the parameter type defined in the Task spec",
							Properties:           make(map[string]*jsonschema.Schema),
							AdditionalProperties: emptySchema,
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Tekton: Start Task",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: startTask,
		},
	}
}

func startTask(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	namespace := p.OptionalString("namespace", params.NamespaceOrDefault(""))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to start task: %w", err)), nil
	}

	dynamicClient := params.DynamicClient()

	// Verify that the Task exists
	if _, err := dynamicClient.Resource(taskGVR).Namespace(namespace).Get(params.Context, name, metav1.GetOptions{}); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get Task %s/%s: %w", namespace, name, err)), nil
	}

	var tektonParams []tektonv1.Param
	if rawParams, ok := params.GetArguments()["params"].(map[string]interface{}); ok {
		var err error
		tektonParams, err = parseParams(rawParams)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to parse params: %w", err)), nil
		}
	}

	tr := &tektonv1.TaskRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1",
			Kind:       "TaskRun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: name + "-",
		},
		Spec: tektonv1.TaskRunSpec{
			TaskRef: &tektonv1.TaskRef{
				Name: name,
			},
			Params: tektonParams,
		},
	}

	// Convert to unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tr)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to convert TaskRun to unstructured: %w", err)), nil
	}

	createdUnstructured, err := dynamicClient.Resource(taskRunGVR).Namespace(namespace).Create(params.Context, &unstructured.Unstructured{Object: unstructuredObj}, metav1.CreateOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create TaskRun for Task %s/%s: %w", namespace, name, err)), nil
	}

	createdName := createdUnstructured.GetName()
	return api.NewToolCallResult(fmt.Sprintf("Task '%s' started as TaskRun '%s' in namespace '%s'", name, createdName, namespace), nil), nil
}
