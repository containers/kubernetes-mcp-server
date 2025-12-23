package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// promptCallRequestAdapter adapts MCP GetPromptRequest to api.PromptCallRequest
type promptCallRequestAdapter struct {
	request *mcp.GetPromptRequest
}

func (p *promptCallRequestAdapter) GetArguments() map[string]string {
	if p.request == nil || p.request.Params == nil || p.request.Params.Arguments == nil {
		return make(map[string]string)
	}
	return p.request.Params.Arguments
}

// ServerPromptToGoSdkPrompt converts an api.ServerPrompt to MCP SDK types
func ServerPromptToGoSdkPrompt(s *Server, serverPrompt api.ServerPrompt) (*mcp.Prompt, mcp.PromptHandler, error) {
	// Convert arguments
	var args []*mcp.PromptArgument
	for _, arg := range serverPrompt.Prompt.Arguments {
		args = append(args, &mcp.PromptArgument{
			Name:        arg.Name,
			Description: arg.Description,
			Required:    arg.Required,
		})
	}

	// Create the MCP SDK prompt
	mcpPrompt := &mcp.Prompt{
		Name:        serverPrompt.Prompt.Name,
		Description: serverPrompt.Prompt.Description,
		Arguments:   args,
	}

	// Create the handler that wraps the ServerPrompt handler
	handler := func(ctx context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		clusterParam := s.p.GetTargetParameterName()
		var cluster string
		if request.Params != nil && request.Params.Arguments != nil {
			if val, ok := request.Params.Arguments[clusterParam]; ok {
				cluster = val
			}
		}

		k8s, err := s.p.GetDerivedKubernetes(ctx, cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to get kubernetes client: %w", err)
		}

		params := api.PromptHandlerParams{
			Context:                ctx,
			ExtendedConfigProvider: s.configuration,
			KubernetesClient:       k8s,
			PromptCallRequest:      &promptCallRequestAdapter{request: request},
		}

		result, err := serverPrompt.Handler(params)
		if err != nil {
			return nil, err
		}

		if result.Error != nil {
			return nil, result.Error
		}

		var messages []*mcp.PromptMessage
		for _, msg := range result.Messages {
			mcpMsg := &mcp.PromptMessage{
				Role: mcp.Role(msg.Role),
			}

			switch msg.Content.Type {
			case "text":
				mcpMsg.Content = &mcp.TextContent{
					Text: msg.Content.Text,
				}
			default:
				mcpMsg.Content = &mcp.TextContent{
					Text: msg.Content.Text,
				}
			}

			messages = append(messages, mcpMsg)
		}

		return &mcp.GetPromptResult{
			Description: result.Description,
			Messages:    messages,
		}, nil
	}

	return mcpPrompt, handler, nil
}
