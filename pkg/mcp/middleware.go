package mcp

import (
	"bytes"
	"context"

	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/klog/v2"
)

// sessionInjectionMiddleware injects the MCP session into the context for logging support.
// This middleware should be added first so all subsequent middleware and handlers have access.
func sessionInjectionMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if session := req.GetSession(); session != nil {
			if serverSession, ok := session.(*mcp.ServerSession); ok {
				ctx = context.WithValue(ctx, mcplog.MCPSessionContextKey, serverSession)
			}
		}
		return next(ctx, method, req)
	}
}

func authHeaderPropagationMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if req.GetExtra() != nil && req.GetExtra().Header != nil {
			// Get the standard Authorization header (OAuth compliant)
			authHeader := req.GetExtra().Header.Get(string(internalk8s.OAuthAuthorizationHeader))
			if authHeader != "" {
				return next(context.WithValue(ctx, internalk8s.OAuthAuthorizationHeader, authHeader), method, req)
			}

			// Fallback to custom header for backward compatibility
			customAuthHeader := req.GetExtra().Header.Get(string(internalk8s.CustomAuthorizationHeader))
			if customAuthHeader != "" {
				return next(context.WithValue(ctx, internalk8s.OAuthAuthorizationHeader, customAuthHeader), method, req)
			}
		}
		return next(ctx, method, req)
	}
}

func toolCallLoggingMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		switch params := req.GetParams().(type) {
		case *mcp.CallToolParamsRaw:
			toolCallRequest, _ := GoSdkToolCallParamsToToolCallRequest(params)
			klog.V(5).Infof("mcp tool call: %s(%v)", toolCallRequest.Name, toolCallRequest.GetArguments())
			if req.GetExtra() != nil && req.GetExtra().Header != nil {
				buffer := bytes.NewBuffer(make([]byte, 0))
				if err := req.GetExtra().Header.WriteSubset(buffer, map[string]bool{"Authorization": true, "authorization": true}); err == nil {
					klog.V(7).Infof("mcp tool call headers: %s", buffer)
				}
			}
		}
		return next(ctx, method, req)
	}
}
