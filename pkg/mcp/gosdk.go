package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

func toolCallLoggingMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		klog.V(5).Infof("mcp tool call: %s(%v)", method, req.GetParams())
		if req.GetExtra() != nil && req.GetExtra().Header != nil {
			buffer := bytes.NewBuffer(make([]byte, 0))
			if err := req.GetExtra().Header.WriteSubset(buffer, map[string]bool{"Authorization": true, "authorization": true}); err == nil {
				klog.V(7).Infof("mcp tool call headers: %s", buffer)
			}
		}
		return next(ctx, method, req)
	}
}

func toolScopedAuthorizationMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		scopes, ok := ctx.Value(TokenScopesContextKey).([]string)
		if !ok {
			return NewTextResult("", fmt.Errorf("authorization failed: Access denied: Tool '%s' requires scope 'mcp:%s' but no scope is available", method, method)), nil
		}
		if !slices.Contains(scopes, "mcp:"+method) && !slices.Contains(scopes, method) {
			return NewTextResult("", fmt.Errorf("authorization failed: Access denied: Tool '%s' requires scope 'mcp:%s' but only scopes %s are available", method, method, scopes)), nil
		}
		return next(ctx, method, req)
	}
}

func ServerToolToGoSdkTool(s *Server, tool api.ServerTool) (*mcp.Tool, mcp.ToolHandler, error) {
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
	}
	if tool.Tool.InputSchema != nil {
		schema, err := json.Marshal(tool.Tool.InputSchema)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal tool input schema for tool %s: %v", tool.Tool.Name, err)
		}
		// TODO: temporary fix to append an empty properties object (some client have trouble parsing a schema without properties)
		// As opposed, Gemini had trouble for a while when properties was present but empty.
		// https://github.com/containers/kubernetes-mcp-server/issues/340
		if string(schema) == `{"type":"object"}` {
			schema = []byte(`{"type":"object","properties":{}}`)
		}

		var fixedSchema map[string]interface{}
		if err := json.Unmarshal(schema, &fixedSchema); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal tool input schema for tool %s: %v", tool.Tool.Name, err)
		}

		goSdkTool.InputSchema = fixedSchema
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
			Context:         ctx,
			Kubernetes:      k,
			ToolCallRequest: toolCallRequest,
			ListOutput:      s.configuration.ListOutput(),
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
	var arguments map[string]any
	if err := json.Unmarshal(toolCallParams.Arguments, &arguments); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tool call arguments: %v", err)
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
