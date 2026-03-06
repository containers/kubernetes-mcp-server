package tekton

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
							AdditionalProperties: &jsonschema.Schema{},
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
	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	namespace := api.OptionalString(params, "namespace", params.KubernetesClient.NamespaceOrDefault(""))

	tektonClient, err := newTektonClient(params.KubernetesClient)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Tekton client: %w", err)), nil
	}

	if _, err := tektonClient.TektonV1().Tasks(namespace).Get(params.Context, name, metav1.GetOptions{}); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get Task %s/%s: %w", namespace, name, err)), nil
	}

	var tektonParams []tektonv1.Param
	if rawParams, ok := params.GetArguments()["params"].(map[string]interface{}); ok {
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

	created, err := tektonClient.TektonV1().TaskRuns(namespace).Create(params.Context, tr, metav1.CreateOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create TaskRun for Task %s/%s: %w", namespace, name, err)), nil
	}

	return api.NewToolCallResult(fmt.Sprintf("Task '%s' started as TaskRun '%s' in namespace '%s'", name, created.Name, namespace), nil), nil
}
