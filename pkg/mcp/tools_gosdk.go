package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/confirmation"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/utils/ptr"
)

func ServerToolToGoSdkTool(s *Server, tool api.ServerTool) (*mcp.Tool, mcp.ToolHandler, error) {
	// Validate the input schema upfront to mirror the SDK's AddTool panic
	// surface. This keeps applyToolsets' two-phase model panic-free at commit
	// time even if a misconfigured tool slips through the toolset boundary.
	inputSchema := tool.Tool.InputSchema
	if inputSchema == nil {
		return nil, nil, fmt.Errorf("tool %q: missing input schema", tool.Tool.Name)
	}
	if inputSchema.Type != "object" {
		return nil, nil, fmt.Errorf("tool %q: input schema must have type %q (got %q)", tool.Tool.Name, "object", inputSchema.Type)
	}
	// Ensure InputSchema.Properties is initialized for OpenAI API compatibility
	// https://github.com/containers/kubernetes-mcp-server/issues/717
	if inputSchema.Properties == nil {
		inputSchema.Properties = make(map[string]*jsonschema.Schema)
	}
	goSdkTool := &mcp.Tool{
		Name:        tool.Tool.Name,
		Description: tool.Tool.Description,
		Title:       tool.Tool.Annotations.Title,
		Meta:        mcp.Meta(tool.Tool.Meta),
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
		// Snapshot the live configuration once so a concurrent reload
		// can't split BaseConfig and ListOutput across two configs.
		cfg := s.configuration.Load()
		// Check confirmation rules before executing the tool
		if confirmErr := confirmation.CheckToolRules(
			ctx, cfg, &sessionElicitor{},
			tool.Tool.Name, tool.Tool.Annotations.DestructiveHint,
		); confirmErr != nil {
			return NewTextResult("", confirmErr), nil
		}

		// get the correct derived Kubernetes client for the target specified in the request
		cluster := toolCallRequest.GetString(s.p.GetTargetParameterName(), s.p.GetDefaultTarget())
		k, err := s.p.GetDerivedKubernetes(ctx, cluster)
		if err != nil {
			return nil, err
		}

		result, err := tool.Handler(api.ToolHandlerParams{
			Context:          ctx,
			BaseConfig:       cfg,
			KubernetesClient: k,
			ToolCallRequest:  toolCallRequest,
			ListOutput:       cfg.ListOutput(),
			Elicitor:         &sessionElicitor{},
		})
		if err != nil {
			return nil, err
		}
		if result.Error != nil {
			mcplog.HandleK8sError(ctx, result.Error, tool.Tool.Name)
		}
		return NewStructuredResult(result.Content, result.StructuredContent, result.Error), nil
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
	if len(toolCallParams.Arguments) > 0 {
		if err := json.Unmarshal(toolCallParams.Arguments, &arguments); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool call arguments: %w", err)
		}
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
