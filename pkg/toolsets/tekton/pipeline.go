package tekton

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func pipelineTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "tekton_pipeline_start",
				Description: "Start a Tekton Pipeline by creating a PipelineRun that references it",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the Pipeline to start",
						},
						"namespace": {
							Type:        "string",
							Description: "Namespace of the Pipeline",
						},
						"params": {
							Type:                 "object",
							Description:          "Parameter values to pass to the Pipeline. Keys are parameter names; values can be a string, an array of strings, or an object (map of string to string) depending on the parameter type defined in the Pipeline spec",
							Properties:           make(map[string]*jsonschema.Schema),
							AdditionalProperties: emptySchema,
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Tekton: Start Pipeline",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: startPipeline,
		},
	}
}

func startPipeline(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	namespace := api.OptionalString(params, "namespace", params.KubernetesClient.NamespaceOrDefault(""))

	tektonClient, err := newTektonClient(params.KubernetesClient)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Tekton client: %w", err)), nil
	}

	if _, err := tektonClient.TektonV1().Pipelines(namespace).Get(params.Context, name, metav1.GetOptions{}); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get Pipeline %s/%s: %w", namespace, name, err)), nil
	}

	var tektonParams []tektonv1.Param
	if rawParams, ok := params.GetArguments()["params"].(map[string]interface{}); ok {
		tektonParams, err = parseParams(rawParams)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to parse params: %w", err)), nil
		}
	}

	pr := &tektonv1.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1",
			Kind:       "PipelineRun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: name + "-",
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineRef: &tektonv1.PipelineRef{
				Name: name,
			},
			Params: tektonParams,
		},
	}

	created, err := tektonClient.TektonV1().PipelineRuns(namespace).Create(params.Context, pr, metav1.CreateOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create PipelineRun for Pipeline %s/%s: %w", namespace, name, err)), nil
	}

	return api.NewToolCallResult(fmt.Sprintf("Pipeline '%s' started as PipelineRun '%s' in namespace '%s'", name, created.Name, namespace), nil), nil
}
