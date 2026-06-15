package argocd

import (
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
)

func appProjectTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "argocd_appproject_list",
				Description: "List ArgoCD AppProjects in the current cluster",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to list AppProjects from. If not provided, lists from all namespaces",
						},
						"labelSelector": {
							Type:        "string",
							Description: "Optional Kubernetes label selector to filter AppProjects (e.g. 'app=myapp')",
						},
					},
				},
				Annotations: api.ToolAnnotations{
					Title:           "ArgoCD: List AppProjects",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: listResources(appProjectGVR, "AppProjects"),
		},
		{
			Tool: api.Tool{
				Name:        "argocd_appproject_get",
				Description: "Get an ArgoCD AppProject by name",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the AppProject",
						},
						"namespace": {
							Type:        "string",
							Description: "Namespace of the AppProject",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "ArgoCD: Get AppProject",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: getAppProject,
		},
	}
}

func getAppProject(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	namespace := p.OptionalString("namespace", params.NamespaceOrDefault(""))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get AppProject: %w", err)), nil
	}
	ret, err := params.DynamicClient().Resource(appProjectGVR).Namespace(namespace).Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get AppProject %s/%s: %w", namespace, name, err)), nil
	}
	summary := formatAppProjectSummary(name, ret)
	yamlStr, err := output.MarshalYaml(ret)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal AppProject %s/%s: %w", namespace, name, err)), nil
	}
	return api.NewToolCallResult(summary+yamlStr, nil), nil
}

func formatAppProjectSummary(name string, obj *unstructured.Unstructured) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# AppProject: %s\n", name)

	desc, _, _ := unstructured.NestedString(obj.Object, "spec", "description")
	if desc != "" {
		fmt.Fprintf(&sb, "- Description: %s\n", desc)
	}

	repos, found, _ := unstructured.NestedStringSlice(obj.Object, "spec", "sourceRepos")
	if found && len(repos) > 0 {
		fmt.Fprintf(&sb, "- Source Repos: %s\n", strings.Join(repos, ", "))
	}

	dests, found, _ := unstructured.NestedSlice(obj.Object, "spec", "destinations")
	if found {
		fmt.Fprintf(&sb, "- Destinations: %d destination(s)\n", len(dests))
	}

	sb.WriteString("\n")
	return sb.String()
}
