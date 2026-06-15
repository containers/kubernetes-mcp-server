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

func applicationTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "argocd_application_list",
				Description: "List ArgoCD Applications in the current cluster",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to list Applications from. If not provided, lists from all namespaces",
						},
						"labelSelector": {
							Type:        "string",
							Description: "Optional Kubernetes label selector to filter Applications (e.g. 'app=myapp')",
						},
					},
				},
				Annotations: api.ToolAnnotations{
					Title:           "ArgoCD: List Applications",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: listResources(applicationGVR, "Applications"),
		},
		{
			Tool: api.Tool{
				Name:        "argocd_application_get",
				Description: "Get an ArgoCD Application by name",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the Application",
						},
						"namespace": {
							Type:        "string",
							Description: "Namespace of the Application",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "ArgoCD: Get Application",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: getApplication,
		},
	}
}

func getApplication(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	namespace := p.OptionalString("namespace", params.NamespaceOrDefault(""))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get Application: %w", err)), nil
	}
	ret, err := params.DynamicClient().Resource(applicationGVR).Namespace(namespace).Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get Application %s/%s: %w", namespace, name, err)), nil
	}
	summary := formatApplicationSummary(name, ret)
	yamlStr, err := output.MarshalYaml(ret)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal Application %s/%s: %w", namespace, name, err)), nil
	}
	return api.NewToolCallResult(summary+yamlStr, nil), nil
}

func formatApplicationSummary(name string, obj *unstructured.Unstructured) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Application: %s\n", name)

	project, _, _ := unstructured.NestedString(obj.Object, "spec", "project")
	if project != "" {
		fmt.Fprintf(&sb, "- Project: %s\n", project)
	}

	syncStatus, _, _ := unstructured.NestedString(obj.Object, "status", "sync", "status")
	if syncStatus != "" {
		fmt.Fprintf(&sb, "- Sync Status: %s\n", syncStatus)
	}

	healthStatus, _, _ := unstructured.NestedString(obj.Object, "status", "health", "status")
	if healthStatus != "" {
		fmt.Fprintf(&sb, "- Health: %s\n", healthStatus)
	}

	repoURL, found, _ := unstructured.NestedString(obj.Object, "spec", "source", "repoURL")
	if found {
		path, _, _ := unstructured.NestedString(obj.Object, "spec", "source", "path")
		rev, _, _ := unstructured.NestedString(obj.Object, "spec", "source", "targetRevision")
		fmt.Fprintf(&sb, "- Source: %s (path: %s, revision: %s)\n", repoURL, path, rev)
	} else if sources, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "sources"); ok && len(sources) > 0 {
		for i, src := range sources {
			if srcMap, ok := src.(map[string]interface{}); ok {
				url, _ := srcMap["repoURL"].(string)
				path, _ := srcMap["path"].(string)
				rev, _ := srcMap["targetRevision"].(string)
				fmt.Fprintf(&sb, "- Source[%d]: %s (path: %s, revision: %s)\n", i, url, path, rev)
			}
		}
	}

	server, _, _ := unstructured.NestedString(obj.Object, "spec", "destination", "server")
	destNs, _, _ := unstructured.NestedString(obj.Object, "spec", "destination", "namespace")
	if server != "" || destNs != "" {
		fmt.Fprintf(&sb, "- Destination: %s / %s\n", server, destNs)
	}

	sb.WriteString("\n")
	return sb.String()
}
