package mcp

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func (s *Server) initNamespaces() []ServerTool {
	ret := make([]ServerTool, 0)
	ret = append(ret, ServerTool{
		Tool: Tool{
			Name:        "namespaces_list",
			Description: "List all the Kubernetes namespaces in the current cluster",
			InputSchema: &jsonschema.Schema{
				Type: "object",
			},
			Annotations: ToolAnnotations{
				Title:           "Namespaces: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.namespacesList,
	})
	if s.k.IsOpenShift(context.Background()) {
		ret = append(ret, ServerTool{
			Tool: Tool{
				Name:        "projects_list",
				Description: "List all the OpenShift projects in the current cluster",
				InputSchema: &jsonschema.Schema{
					Type: "object",
				},
				Annotations: ToolAnnotations{
					Title:           "Projects: List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			}, Handler: s.projectsList,
		})
	}
	return ret
}

func (s *Server) namespacesList(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.NamespacesList(ctx, kubernetes.ResourceListOptions{AsTable: s.configuration.ListOutput.AsTable()})
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to list namespaces: %v", err)), nil
	}
	return NewTextResult(s.configuration.ListOutput.PrintObj(ret)), nil
}

func (s *Server) projectsList(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.ProjectsList(ctx, kubernetes.ResourceListOptions{AsTable: s.configuration.ListOutput.AsTable()})
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to list projects: %v", err)), nil
	}
	return NewTextResult(s.configuration.ListOutput.PrintObj(ret)), nil
}
