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

func instanceTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "argocd_instance_list",
				Description: "List ArgoCD instances (ArgoCD operator CRs) in the current cluster",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to list ArgoCD instances from. If not provided, lists from all namespaces",
						},
						"labelSelector": {
							Type:        "string",
							Description: "Optional Kubernetes label selector to filter ArgoCD instances (e.g. 'app=myapp')",
						},
					},
				},
				Annotations: api.ToolAnnotations{
					Title:           "ArgoCD: List Instances",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: listResources(argocdGVR, "ArgoCD instances"),
		},
		{
			Tool: api.Tool{
				Name:        "argocd_instance_get",
				Description: "Get an ArgoCD instance (ArgoCD operator CR) by name",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the ArgoCD instance",
						},
						"namespace": {
							Type:        "string",
							Description: "Namespace of the ArgoCD instance",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "ArgoCD: Get Instance",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: getInstance,
		},
	}
}

func getInstance(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	namespace := p.OptionalString("namespace", params.NamespaceOrDefault(""))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get ArgoCD instance: %w", err)), nil
	}
	ret, err := params.DynamicClient().Resource(argocdGVR).Namespace(namespace).Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get ArgoCD instance %s/%s: %w", namespace, name, err)), nil
	}
	summary := formatInstanceSummary(name, ret)
	yamlStr, err := output.MarshalYaml(ret)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal ArgoCD instance %s/%s: %w", namespace, name, err)), nil
	}
	return api.NewToolCallResult(summary+yamlStr, nil), nil
}

func formatInstanceSummary(name string, obj *unstructured.Unstructured) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# ArgoCD: %s\n", name)
	fmt.Fprintf(&sb, "- Namespace: %s\n", obj.GetNamespace())

	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	if phase != "" {
		fmt.Fprintf(&sb, "- Phase: %s\n", phase)
	}

	if _, found, _ := unstructured.NestedMap(obj.Object, "spec", "server"); found {
		fmt.Fprintf(&sb, "- Server: configured\n")
	} else {
		fmt.Fprintf(&sb, "- Server: not configured\n")
	}

	if _, found, _ := unstructured.NestedMap(obj.Object, "spec", "ha"); found {
		fmt.Fprintf(&sb, "- HA: enabled\n")
	} else {
		fmt.Fprintf(&sb, "- HA: not configured\n")
	}

	sb.WriteString("\n")
	return sb.String()
}
