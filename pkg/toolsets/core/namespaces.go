package core

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/mcpapps"
)

func initNamespaces(inspector api.ClusterInspector) []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "namespaces_list",
			Description: "List all the Kubernetes namespaces in the current cluster." + ListPageSentence,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: PageProperties(map[string]*jsonschema.Schema{
					"fieldSelector": {
						Type:        "string",
						Description: "Optional Kubernetes field selector to filter namespaces by field values (e.g. 'metadata.name=default', 'status.phase=Active'). Supported fields: metadata.name, status.phase. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/",
						Pattern:     REGEX_FIELDSELECTOR,
					},
				}),
			},
			Annotations: api.ToolAnnotations{
				Title:           "Namespaces: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		},
		RBAC: api.RBACBounded(api.RBACRequirement{
			Verbs:  []string{"list"},
			Target: api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "namespaces"}},
		}),
		Handler: namespacesList,
		App:     mcpapps.NamespacesList(),
	})
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "projects_list",
			Description: "List all the OpenShift projects in the current cluster." + ListPageSentence,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: PageProperties(nil),
			},
			Annotations: api.ToolAnnotations{
				Title:           "Projects: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		},
		RBAC: api.RBACBounded(api.RBACRequirement{
			Verbs: []string{"list"},
			Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
				APIGroup: "project.openshift.io",
				Resource: "projects",
			}},
		}),
		Handler: projectsList,
		TargetCompatibilityFilters: []api.TargetCompatibilityFilter{
			func(ctx context.Context) bool {
				return api.AnyTargetHasGVK(ctx, inspector, schema.GroupVersionKind{Group: "project.openshift.io", Version: "v1", Kind: "Project"})
			},
		},
		App: mcpapps.Table(
			"ui://kubernetes-mcp-server/projects-list",
			"Projects list",
			mcpapps.WithDescription("Interactive table of OpenShift projects"),
		),
	})
	return ret
}

func namespacesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	options := api.ListOptions{AsTable: params.ListOutput.AsTable()}
	options.FieldSelector = p.OptionalString("fieldSelector", "")
	if err := ApplyListPage(p, &options.ListOptions); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list namespaces: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).NamespacesList(params, options)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list namespaces: %w", ListErr(err))), nil
	}
	printed, err := params.ListOutput.PrintObjStructured(ret)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to render namespaces: %w", err)), nil
	}
	text, structured := WithListPage(printed.Text, tableAppStructured(ret, printed.Structured), ret)
	return api.NewToolCallResultFull(text, structured, nil), nil
}

func projectsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	options := api.ListOptions{AsTable: params.ListOutput.AsTable()}
	if err := ApplyListPage(api.WrapParams(params), &options.ListOptions); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list projects: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).ProjectsList(params, options)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list projects: %w", ListErr(err))), nil
	}
	printed, err := params.ListOutput.PrintObjStructured(ret)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to render projects: %w", err)), nil
	}
	text, structured := WithListPage(printed.Text, tableAppStructured(ret, printed.Structured), ret)
	return api.NewToolCallResultFull(text, structured, nil), nil
}
