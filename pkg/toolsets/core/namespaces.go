package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/mcpapps"
)

func initNamespaces(p api.FilteringProvider) []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "namespaces_list",
			Description: "List all the Kubernetes namespaces in the current cluster",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"fieldSelector": {
						Type:        "string",
						Description: "Optional Kubernetes field selector to filter namespaces by field values (e.g. 'metadata.name=default', 'status.phase=Active'). Supported fields: metadata.name, status.phase. See https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/",
						Pattern:     REGEX_FIELDSELECTOR,
					},
				},
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
			Description: "List all the OpenShift projects in the current cluster",
			InputSchema: &jsonschema.Schema{
				Type: "object",
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
		TargetCompatibilityFilters: []func() bool{
			func() bool {
				return p.AnyTargetHasGVKs(context.TODO(), []schema.GroupVersionKind{
					{Group: "project.openshift.io", Version: "v1", Kind: "Project"},
				})
			},
		},
	})
	return ret
}

func namespacesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	options := api.ListOptions{AsTable: params.ListOutput.AsTable()}
	options.FieldSelector = p.OptionalString("fieldSelector", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list namespaces: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).NamespacesList(params, options)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list namespaces: %w", err)), nil
	}
	printed, err := params.ListOutput.PrintObjStructured(ret)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to render namespaces: %w", err)), nil
	}
	return api.NewToolCallResultFull(printed.Text, namespaceAppStructured(ret, printed.Structured), nil), nil
}

// namespaceAppStructured provides the Apps UI with stable flat rows regardless
// of the text output requested by the caller. YAML output contains full nested
// Kubernetes objects, while table output already supplies flat table cells.
func namespaceAppStructured(ret runtime.Unstructured, structured any) map[string]any {
	if list, ok := ret.(*unstructured.UnstructuredList); ok {
		rows := make([]map[string]any, 0, len(list.Items))
		for _, item := range list.Items {
			phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
			rows = append(rows, map[string]any{
				"Name":       item.GetName(),
				"Status":     phase,
				"Age":        namespaceAge(item.GetCreationTimestamp().Time),
				"Labels":     namespaceLabels(item.GetLabels()),
				"apiVersion": item.GetAPIVersion(),
				"kind":       item.GetKind(),
			})
		}
		return map[string]any{"items": rows}
	}
	if rows, ok := structured.([]map[string]any); ok {
		return map[string]any{"items": rows}
	}
	return map[string]any{"items": []map[string]any{}}
}

func namespaceLabels(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+labels[key])
	}
	return strings.Join(values, ",")
}

func namespaceAge(created time.Time) string {
	if created.IsZero() {
		return ""
	}
	d := time.Since(created)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func projectsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	ret, err := kubernetes.NewCore(params).ProjectsList(params, api.ListOptions{AsTable: params.ListOutput.AsTable()})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list projects: %w", err)), nil
	}
	return api.NewToolCallResult(params.ListOutput.PrintObj(ret)), nil
}
