package mcp

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
	"github.com/containers/kubernetes-mcp-server/pkg/telemetry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

// tokenScopesContextKeyType is the type for the token scopes context key
type tokenScopesContextKeyType string

// TokenScopesContextKey is the context key for storing OAuth token scopes
const TokenScopesContextKey tokenScopesContextKeyType = "tokenScopes"

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
		// Skip trace context extraction if telemetry is not enabled
		if !telemetry.Enabled() {
			return next(ctx, method, req)
		}

		// Extract trace context from request params metadata
		if params := req.GetParams(); params != nil {
			if callParams, ok := params.(interface{ GetMeta() map[string]any }); ok {
				// GetMeta() can panic on some MCP message types when metadata is not set
				// (e.g., InitializedParams with nil Meta field). Recover gracefully.
				var meta map[string]any
				func() {
					defer func() {
						if r := recover(); r != nil {
							klog.V(7).Infof("GetMeta() panicked (metadata not set): %v", r)
						}
					}()
					meta = callParams.GetMeta()
				}()

				if len(meta) > 0 {
					carrier := &metaCarrier{meta: meta}

					// Compare span context before and after extraction to verify it worked
					scBefore := trace.SpanContextFromContext(ctx)
					ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
					scAfter := trace.SpanContextFromContext(ctx)

					if scAfter.IsValid() && !scAfter.Equal(scBefore) {
						klog.V(6).Infof("Extracted trace context from MCP request: trace_id=%s span_id=%s", scAfter.TraceID(), scAfter.SpanID())
					}
				}
			}
		}

		return next(ctx, method, req)
	}
}

// tracingMiddleware creates OpenTelemetry spans for MCP method calls.
// This wraps the method execution so that child spans created during execution
// are properly parented to this span.
func tracingMiddleware(tracerName string) func(mcp.MethodHandler) mcp.MethodHandler {
	tracer := otel.Tracer(tracerName)

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// Skip all tracing work if telemetry is not enabled
			if !telemetry.Enabled() {
				return next(ctx, method, req)
			}

			transport := "pipe"
			if req.GetExtra() != nil && req.GetExtra().Header != nil {
				transport = "tcp"
			}

			spanName := method
			attrs := []attribute.KeyValue{
				attribute.String("mcp.method.name", method),
				attribute.String("rpc.jsonrpc.version", "2.0"),
				attribute.String("network.transport", transport),
			}

			if method == "tools/call" {
				if params, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
					if toolReq, _ := GoSdkToolCallParamsToToolCallRequest(params); toolReq != nil {
						spanName = method + " " + toolReq.Name
						attrs = append(attrs, attribute.String("gen_ai.tool.name", toolReq.Name))
						attrs = append(attrs, attribute.String("gen_ai.operation.name", "execute_tool"))
					}
				}
			}

			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attrs...),
			)
			defer span.End()

			result, err := next(ctx, method, req)

			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				// Conditional: Error type classification
				span.SetAttributes(attribute.String("error.type", "_OTHER"))
			} else {
				if callResult, ok := result.(*mcp.CallToolResult); ok && callResult.IsError {
					span.SetStatus(codes.Error, "tool execution failed")
					span.SetAttributes(attribute.String("error.type", "tool_error"))
				} else {
					span.SetStatus(codes.Ok, "")
				}
			}

			return result, err
		}
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
