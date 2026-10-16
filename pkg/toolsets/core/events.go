package core

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func initEvents() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "events_list",
			Description: "List the events of a namespace, or of the whole cluster when namespace is omitted: warnings, errors and state changes, each with the object it is about. Narrow with fieldSelector, e.g. type=Warning or involvedObject.name=<pod>.",
			Examples:    []api.Example{{Summary: "Warnings in a namespace", Arguments: map[string]any{"namespace": "payments", "fieldSelector": "type=Warning"}}, {Summary: "Events about one pod", Arguments: map[string]any{"namespace": "payments", "fieldSelector": "involvedObject.name=api-7d9f8b6c4-x2kqz"}}},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Optional Namespace to retrieve the events from. If not provided, will list events from all namespaces",
					},
					"fieldSelector": {
						Type:        "string",
						Description: "Optional Kubernetes field selector to filter events by field values (e.g. 'type=Warning', 'involvedObject.name=my-pod'). Supported fields: involvedObject.kind, involvedObject.name, involvedObject.namespace, involvedObject.uid, involvedObject.apiVersion, involvedObject.resourceVersion, involvedObject.fieldPath, reason, reportingComponent, source, type. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/",
						Pattern:     REGEX_FIELDSELECTOR,
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Events: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, RBAC: api.RBACBounded(api.RBACRequirement{
			Verbs:     []string{"list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "events"}},
			Namespace: &api.RBACNamespace{Argument: "namespace"},
		}), Handler: eventsList},
	}
}

func eventsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	namespace := p.OptionalString("namespace", "")
	options := api.ListOptions{}
	options.FieldSelector = p.OptionalString("fieldSelector", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list events in all namespaces: %w", err)), nil
	}
	eventMap, err := kubernetes.NewCore(params).EventsList(params, namespace, options)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list events in all namespaces: %w", err)), nil
	}
	if len(eventMap) == 0 {
		return api.NewToolCallResult("# No events found", nil), nil
	}
	yamlEvents, err := output.MarshalYaml(eventMap)
	if err != nil {
		err = fmt.Errorf("failed to list events in all namespaces: %w", err)
	}
	return api.NewToolCallResult(fmt.Sprintf("# The following events (YAML format) were found:\n%s", yamlEvents), err), nil
}
