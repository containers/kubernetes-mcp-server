package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/utils/ptr"
)

func ServerToolToGoSdkTool(s *Server, tool api.ServerTool) (*mcp.Tool, mcp.ToolHandler, error) {
	// Ensure InputSchema.Properties is initialized for OpenAI API compatibility
	// https://github.com/containers/kubernetes-mcp-server/issues/717
	inputSchema := tool.Tool.InputSchema
	if inputSchema != nil && inputSchema.Properties == nil {
		inputSchema.Properties = make(map[string]*jsonschema.Schema)
	}
	goSdkTool := &mcp.Tool{
		Name:        tool.Tool.Name,
		Description: tool.Tool.Description,
		Title:       tool.Tool.Annotations.Title,
		Annotations: &mcp.ToolAnnotations{
			Title:           tool.Tool.Annotations.Title,
			ReadOnlyHint:    ptr.Deref(tool.Tool.Annotations.ReadOnlyHint, false),
			DestructiveHint: tool.Tool.Annotations.DestructiveHint,
			IdempotentHint:  ptr.Deref(tool.Tool.Annotations.IdempotentHint, false),
			OpenWorldHint:   tool.Tool.Annotations.OpenWorldHint,
		},
		InputSchema: inputSchema,
	}
	goSdkHandler := func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		toolCallRequest, err := GoSdkToolCallRequestToToolCallRequest(request)
		if err != nil {
			return nil, fmt.Errorf("%v for tool %s", err, tool.Tool.Name)
		}
		// get the correct derived Kubernetes client for the target specified in the request
		cluster := toolCallRequest.GetString(s.p.GetTargetParameterName(), s.p.GetDefaultTarget())
		k, err := s.p.GetDerivedKubernetes(ctx, cluster)
		if err != nil {
			return nil, err
		}

		result, err := tool.Handler(api.ToolHandlerParams{
			Context:                ctx,
			ExtendedConfigProvider: s.configuration,
			KubernetesClient:       k,
			ToolCallRequest:        toolCallRequest,
			ListOutput:             s.configuration.ListOutput(),
		})
		if err != nil {
			return nil, err
		}
		return NewTextResult(result.Content, result.Error), nil
	}
	return goSdkTool, goSdkHandler, nil
}

type ToolCallRequest struct {
	Name      string
	arguments map[string]any
}

var _ api.ToolCallRequest = (*ToolCallRequest)(nil)

func GoSdkToolCallRequestToToolCallRequest(request *mcp.CallToolRequest) (*ToolCallRequest, error) {
	toolCallParams, ok := request.GetParams().(*mcp.CallToolParamsRaw)
	if !ok {
		return nil, errors.New("invalid tool call parameters for tool call request")
	}
	return GoSdkToolCallParamsToToolCallRequest(toolCallParams)
}

func GoSdkToolCallParamsToToolCallRequest(toolCallParams *mcp.CallToolParamsRaw) (*ToolCallRequest, error) {
	var arguments map[string]any
	if err := json.Unmarshal(toolCallParams.Arguments, &arguments); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tool call arguments: %w", err)
	}
	return &ToolCallRequest{
		Name:      toolCallParams.Name,
		arguments: arguments,
	}, nil
}

func (ToolCallRequest *ToolCallRequest) GetArguments() map[string]any {
	return ToolCallRequest.arguments
}

func (ToolCallRequest *ToolCallRequest) GetString(key, defaultValue string) string {
	if value, ok := ToolCallRequest.arguments[key]; ok {
		if strValue, ok := value.(string); ok {
			return strValue
		}
	}
	return defaultValue
}
