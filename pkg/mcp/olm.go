package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) initOlm() []server.ServerTool {
	return []server.ServerTool{
		{Tool: mcp.NewTool("olm_install",
			mcp.WithDescription("Install an OLMv1 ClusterExtension resource from a manifest (YAML or JSON)"),
			mcp.WithString("manifest", mcp.Description("ClusterExtension manifest to create or update (YAML or JSON)"), mcp.Required()),
			// Tool annotations
			mcp.WithTitleAnnotation("OLM: Install"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.olmInstall},
		{Tool: mcp.NewTool("olm_list",
			mcp.WithDescription("List OLMv1 ClusterExtension resources in the cluster"),
			// Tool annotations
			mcp.WithTitleAnnotation("OLM: List"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.olmList},
		{Tool: mcp.NewTool("olm_uninstall",
			mcp.WithDescription("Uninstall (delete) an OLMv1 ClusterExtension resource by name"),
			mcp.WithString("name", mcp.Description("Name of the ClusterExtension to delete"), mcp.Required()),
			// Tool annotations
			mcp.WithTitleAnnotation("OLM: Uninstall"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.olmUninstall},
		{Tool: mcp.NewTool("olm_upgrade",
			mcp.WithDescription("Upgrade (update) an existing OLMv1 ClusterExtension resource by name using a manifest"),
			mcp.WithString("name", mcp.Description("Name of the ClusterExtension to upgrade"), mcp.Required()),
			mcp.WithString("manifest", mcp.Description("Manifest to apply to the ClusterExtension (YAML or JSON)"), mcp.Required()),
			// Tool annotations
			mcp.WithTitleAnnotation("OLM: Upgrade"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.olmUpgrade},
	}
}

func (s *Server) olmInstall(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	manifest, ok := ctr.GetArguments()["manifest"].(string)
	if !ok || manifest == "" {
		return NewTextResult("", fmt.Errorf("missing argument manifest")), nil
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.NewOlm().Install(ctx, manifest)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to install ClusterExtension: %w", err)), nil
	}
	return NewTextResult(ret, nil), nil
}

func (s *Server) olmList(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.NewOlm().List(ctx)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to list ClusterExtensions: %w", err)), nil
	}
	return NewTextResult(ret, nil), nil
}

func (s *Server) olmUninstall(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := ctr.GetArguments()["name"].(string)
	if !ok || name == "" {
		return NewTextResult("", fmt.Errorf("missing argument name")), nil
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.NewOlm().Uninstall(ctx, name)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to uninstall ClusterExtension: %w", err)), nil
	}
	return NewTextResult(ret, nil), nil
}

func (s *Server) olmUpgrade(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := ctr.GetArguments()["name"].(string)
	if !ok || name == "" {
		return NewTextResult("", fmt.Errorf("missing argument name")), nil
	}
	manifest, ok := ctr.GetArguments()["manifest"].(string)
	if !ok || manifest == "" {
		return NewTextResult("", fmt.Errorf("missing argument manifest")), nil
	}
	derived, err := s.k.Derived(ctx)
	if err != nil {
		return nil, err
	}
	ret, err := derived.NewOlm().Upgrade(ctx, name, manifest)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to upgrade ClusterExtension: %w", err)), nil
	}
	return NewTextResult(ret, nil), nil
}
