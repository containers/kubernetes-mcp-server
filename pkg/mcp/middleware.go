package mcp

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"k8s.io/klog/v2"
)

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

// traceContextPropagationMiddleware extracts distributed trace context from MCP request metadata
// and propagates it into the Go context. This enables distributed tracing across MCP protocol boundaries.
//
// The traceparent and tracestate headers should be propagated through the _meta field in MCP requests.
func traceContextPropagationMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		// Extract trace context from request params metadata
		if params := req.GetParams(); params != nil {
			if callParams, ok := params.(interface{ GetMeta() map[string]any }); ok {
				meta := callParams.GetMeta()
				if len(meta) > 0 {
					carrier := &metaCarrier{meta: meta}

					ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)

					if traceparent, ok := meta["traceparent"].(string); ok {
						klog.V(6).Infof("Extracted trace context from MCP request: traceparent=%s", traceparent)
					}
				}
			}
		}

		return next(ctx, method, req)
	}
}

// metaCarrier adapts an MCP Meta map to the OpenTelemetry TextMapCarrier interface
type metaCarrier struct {
	meta map[string]any
}

// Get retrieves a value from the metadata map
func (c *metaCarrier) Get(key string) string {
	if val, ok := c.meta[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// Set is a no-op for extraction (only used for injection)
func (c *metaCarrier) Set(key, value string) {
	// Not used during extraction
}

// Keys returns all keys in the metadata map
func (c *metaCarrier) Keys() []string {
	keys := make([]string, 0, len(c.meta))
	for k := range c.meta {
		keys = append(keys, k)
	}
	return keys
}
