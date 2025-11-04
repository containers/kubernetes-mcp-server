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
			// TODO: temporary fix to append an empty properties object (some client have trouble parsing a schema without properties)
			// As opposed, Gemini had trouble for a while when properties was present but empty.
			// https://github.com/containers/kubernetes-mcp-server/issues/340
			if string(schema) == `{"type":"object"}` {
				schema = []byte(`{"type":"object","properties":{}}`)
			}
			m3labTool.RawInputSchema = schema
		}
		m3labHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// get the correct derived Kubernetes client for the target specified in the request
			cluster := request.GetString(s.p.GetTargetParameterName(), s.p.GetDefaultTarget())
			k, err := s.p.GetDerivedKubernetes(ctx, cluster)
			if err != nil {
				return nil, err
			}
			result, err := tool.Handler(api.ToolHandlerParams{
				Context:         ctx,
				Kubernetes:      k,
				ToolCallRequest: request,
				ListOutput:      s.configuration.ListOutput(),
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
