package full

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func initEvents() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "events_list",
			Description: "List all the Kubernetes events in the cluster (current or provided context) from all namespaces",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"context": api.ContextParameterSchema,
					"namespace": {
						Type:        "string",
						Description: "Optional Namespace to retrieve the events from. If not provided, will list events from all namespaces",
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Events: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: eventsList},
	}
}

func eventsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Get Kubernetes client for the specified context (or default)
	k8sClient, err := api.GetKubernetesWithContext(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	namespace := params.GetArguments()["namespace"]
	if namespace == nil {
		namespace = ""
	}
	eventMap, err := k8sClient.EventsList(params.Context, namespace.(string))
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list events in all namespaces: %v", err)), nil
	}
	if len(eventMap) == 0 {
		return api.NewToolCallResult("No events found", nil), nil
	}
	yamlEvents, err := output.MarshalYaml(eventMap)
	if err != nil {
		err = fmt.Errorf("failed to list events in all namespaces: %v", err)
	}
	return api.NewToolCallResult(fmt.Sprintf("The following events (YAML format) were found:\n%s", yamlEvents), err), nil
}
