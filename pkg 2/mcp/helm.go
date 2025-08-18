package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) initHelm() []server.ServerTool {
	return []server.ServerTool{
		{Tool: mcp.NewTool("helm_install",
			mcp.WithDescription("Install a Helm chart in the current or provided namespace"),
			mcp.WithString("chart", mcp.Description("Chart reference to install (for example: stable/grafana, oci://ghcr.io/nginxinc/charts/nginx-ingress)"), mcp.Required()),
			mcp.WithObject("values", mcp.Description("Values to pass to the Helm chart (Optional)")),
			mcp.WithString("name", mcp.Description("Name of the Helm release (Optional, random name if not provided)")),
			mcp.WithString("namespace", mcp.Description("Namespace to install the Helm chart in (Optional, current namespace if not provided)")),
			// Tool annotations
			mcp.WithTitleAnnotation("Helm: Install"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false), // TODO: consider replacing implementation with equivalent to: helm upgrade --install
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.helmInstall},
		{Tool: mcp.NewTool("helm_list",
			mcp.WithDescription("List all the Helm releases in the current or provided namespace (or in all namespaces if specified)"),
			mcp.WithString("namespace", mcp.Description("Namespace to list Helm releases from (Optional, all namespaces if not provided)")),
			mcp.WithBoolean("all_namespaces", mcp.Description("If true, lists all Helm releases in all namespaces ignoring the namespace argument (Optional)")),
			// Tool annotations
			mcp.WithTitleAnnotation("Helm: List"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.helmList},
		{Tool: mcp.NewTool("helm_uninstall",
			mcp.WithDescription("Uninstall a Helm release in the current or provided namespace"),
			mcp.WithString("name", mcp.Description("Name of the Helm release to uninstall"), mcp.Required()),
			mcp.WithString("namespace", mcp.Description("Namespace to uninstall the Helm release from (Optional, current namespace if not provided)")),
			// Tool annotations
			mcp.WithTitleAnnotation("Helm: Uninstall"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.helmUninstall},
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
