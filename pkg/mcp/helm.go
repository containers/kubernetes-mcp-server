package mcp

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/utils/ptr"
)

func (s *Server) initHelm() []ServerTool {
	return []ServerTool{
		{Tool: Tool{
			Name:        "helm_install",
			Description: "Install a Helm chart in the current or provided namespace",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"chart": {
						Type:        "string",
						Description: "Chart reference to install (for example: stable/grafana, oci://ghcr.io/nginxinc/charts/nginx-ingress)",
					},
					"values": {
						Type:        "object",
						Description: "Values to pass to the Helm chart (Optional)",
						Properties:  make(map[string]*jsonschema.Schema),
					},
					"name": {
						Type:        "string",
						Description: "Name of the Helm release (Optional, random name if not provided)",
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace to install the Helm chart in (Optional, current namespace if not provided)",
					},
				},
				Required: []string{"chart"},
			},
			Annotations: ToolAnnotations{
				Title:           "Helm: Install",
				ReadOnlyHint:    ptr.To(false),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false), // TODO: consider replacing implementation with equivalent to: helm upgrade --install
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.helmInstall},
		{Tool: Tool{
			Name:        "helm_list",
			Description: "List all the Helm releases in the current or provided namespace (or in all namespaces if specified)",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace to list Helm releases from (Optional, all namespaces if not provided)",
					},
					"all_namespaces": {
						Type:        "boolean",
						Description: "If true, lists all Helm releases in all namespaces ignoring the namespace argument (Optional)",
					},
				},
			},
			Annotations: ToolAnnotations{
				Title:           "Helm: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.helmList},
		{Tool: Tool{
			Name:        "helm_uninstall",
			Description: "Uninstall a Helm release in the current or provided namespace",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the Helm release to uninstall",
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace to uninstall the Helm release from (Optional, current namespace if not provided)",
					},
				},
				Required: []string{"name"},
			},
			Annotations: ToolAnnotations{
				Title:           "Helm: Uninstall",
				ReadOnlyHint:    ptr.To(false),
				DestructiveHint: ptr.To(true),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.helmUninstall},
	}
}

func (s *Server) helmInstall(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var chart string
	ok := false
	if chart, ok = ctr.GetArguments()["chart"].(string); !ok {
		return NewTextResult("", fmt.Errorf("failed to install helm chart, missing argument chart")), nil
	}
	values := map[string]interface{}{}
	if v, ok := ctr.GetArguments()["values"].(map[string]interface{}); ok {
		values = v
	}
	name := ""
	if v, ok := ctr.GetArguments()["name"].(string); ok {
		name = v
	}
	namespace := ""
	if v, ok := ctr.GetArguments()["namespace"].(string); ok {
		namespace = v
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.NewHelm().Install(ctx, chart, values, name, namespace)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to install helm chart '%s': %w", chart, err)), nil
	}
	return NewTextResult(ret, err), nil
}

func (s *Server) helmList(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	allNamespaces := false
	if v, ok := ctr.GetArguments()["all_namespaces"].(bool); ok {
		allNamespaces = v
	}
	namespace := ""
	if v, ok := ctr.GetArguments()["namespace"].(string); ok {
		namespace = v
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.NewHelm().List(namespace, allNamespaces)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to list helm releases in namespace '%s': %w", namespace, err)), nil
	}
	return NewTextResult(ret, err), nil
}

func (s *Server) helmUninstall(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var name string
	ok := false
	if name, ok = ctr.GetArguments()["name"].(string); !ok {
		return NewTextResult("", fmt.Errorf("failed to uninstall helm chart, missing argument name")), nil
	}
	namespace := ""
	if v, ok := ctr.GetArguments()["namespace"].(string); ok {
		namespace = v
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.NewHelm().Uninstall(name, namespace)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to uninstall helm chart '%s': %w", name, err)), nil
	}
	return NewTextResult(ret, err), nil
}
