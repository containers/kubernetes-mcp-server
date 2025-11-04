package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// ServerPromptToGoSdkPrompt converts our internal ServerPrompt to go-sdk Prompt format
func ServerPromptToGoSdkPrompt(s *Server, prompts []api.ServerPrompt) (map[string]*mcp.Prompt, map[string]mcp.PromptHandler, error) {
	goSdkPrompts := make(map[string]*mcp.Prompt)
	goSdkHandlers := make(map[string]mcp.PromptHandler)

	for _, prompt := range prompts {
		// Convert arguments to PromptArgument pointers
		var arguments []*mcp.PromptArgument
		for _, arg := range prompt.Arguments {
			arguments = append(arguments, &mcp.PromptArgument{
				Name:        arg.Name,
				Description: arg.Description,
				Required:    arg.Required,
			})
		}

		goSdkPrompt := &mcp.Prompt{
			Name:        prompt.Name,
			Description: prompt.Description,
			Arguments:   arguments,
		}

		// Create the prompt handler
		handler := createPromptHandler(s, prompt)

		goSdkPrompts[prompt.Name] = goSdkPrompt
		goSdkHandlers[prompt.Name] = handler
	}

	return goSdkPrompts, goSdkHandlers, nil
}

// createPromptHandler creates a handler function for a prompt
func createPromptHandler(s *Server, prompt api.ServerPrompt) mcp.PromptHandler {
	return func(ctx context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		// Get arguments from the request
		params := request.GetParams()
		arguments := make(map[string]string)
		if params != nil {
			// Cast to concrete type to access Arguments field
			if getPromptParams, ok := params.(*mcp.GetPromptParams); ok && getPromptParams.Arguments != nil {
				arguments = getPromptParams.Arguments
			}
		}

		// Get messages from the prompt
		promptMessages := prompt.GetMessages(arguments)

		// Convert to mcp-go format - need to use pointers
		messages := make([]*mcp.PromptMessage, 0, len(promptMessages))
		for _, msg := range promptMessages {
			messages = append(messages, &mcp.PromptMessage{
				Role: mcp.Role(msg.Role),
				Content: &mcp.TextContent{
					Text: msg.Content,
				},
			})
		}

		return &mcp.GetPromptResult{
			Description: prompt.Description,
			Messages:    messages,
		}, nil
	}
}
