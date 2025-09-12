package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func ServerToolToM3LabsServerTool(s *Server, tools []api.ServerTool) ([]server.ServerTool, error) {
	m3labTools := make([]server.ServerTool, 0)
	for _, tool := range tools {
		m3labTool := mcp.Tool{
			Name:        tool.Tool.Name,
			Description: tool.Tool.Description,
			Annotations: mcp.ToolAnnotation{
				Title:           tool.Tool.Annotations.Title,
				ReadOnlyHint:    tool.Tool.Annotations.ReadOnlyHint,
				DestructiveHint: tool.Tool.Annotations.DestructiveHint,
				IdempotentHint:  tool.Tool.Annotations.IdempotentHint,
				OpenWorldHint:   tool.Tool.Annotations.OpenWorldHint,
			},
		}
		if tool.Tool.InputSchema != nil {
			schema, err := json.Marshal(tool.Tool.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool input schema for tool %s: %v", tool.Tool.Name, err)
			}
			m3labTool.RawInputSchema = schema
		}
		m3labHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			k, err := s.k.Derived(ctx)
			if err != nil {
				return nil, err
			}
			result, err := tool.Handler(api.ToolHandlerParams{
				Context:         ctx,
				Kubernetes:      k,
				ToolCallRequest: request,
				ListOutput:      s.configuration.ListOutput,
			})
			if err != nil {
				return nil, err
			}
			return NewTextResult(result.Content, result.Error), nil
		}
		m3labTools = append(m3labTools, server.ServerTool{Tool: m3labTool, Handler: m3labHandler})
	}
	return m3labTools, nil
}
