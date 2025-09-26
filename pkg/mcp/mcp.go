package mcp

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	authenticationapiv1 "k8s.io/api/authentication/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/version"
)

type ContextKey string

const TokenScopesContextKey = ContextKey("TokenScopesContextKey")

type Configuration struct {
	*config.StaticConfig
	listOutput output.Output
	toolsets   []api.Toolset
}

func (c *Configuration) Toolsets() []api.Toolset {
	if c.toolsets == nil {
		for _, toolset := range c.StaticConfig.Toolsets {
			c.toolsets = append(c.toolsets, toolsets.ToolsetFromString(toolset))
		}
	}
	return c.toolsets
}

func (c *Configuration) ListOutput() output.Output {
	if c.listOutput == nil {
		c.listOutput = output.FromString(c.StaticConfig.ListOutput)
	}
	return c.listOutput
}

func (c *Configuration) isToolApplicable(tool api.ServerTool) bool {
	if c.ReadOnly && !ptr.Deref(tool.Tool.Annotations.ReadOnlyHint, false) {
		return false
	}
	if c.DisableDestructive && ptr.Deref(tool.Tool.Annotations.DestructiveHint, false) {
		return false
	}
	if c.EnabledTools != nil && !slices.Contains(c.EnabledTools, tool.Tool.Name) {
		return false
	}
	if c.DisabledTools != nil && slices.Contains(c.DisabledTools, tool.Tool.Name) {
		return false
	}
	return true
}

type Server struct {
	configuration *Configuration
	server        *server.MCPServer
	enabledTools  []string
	k             *internalk8s.Manager
}

func NewServer(configuration Configuration) (*Server, error) {
	var serverOptions []server.ServerOption
	serverOptions = append(serverOptions,
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithToolCapabilities(true),
		server.WithLogging(),
		server.WithToolHandlerMiddleware(toolCallLoggingMiddleware),
	)
	if configuration.RequireOAuth && false { // TODO: Disabled scope auth validation for now
		serverOptions = append(serverOptions, server.WithToolHandlerMiddleware(toolScopedAuthorizationMiddleware))
	}

	s := &Server{
		configuration: &configuration,
		server: server.NewMCPServer(
			version.BinaryName,
			version.Version,
			serverOptions...,
		),
	}
	if err := s.reloadKubernetesClient(); err != nil {
		return nil, err
	}
	s.k.WatchKubeConfig(s.reloadKubernetesClient)

	return s, nil
}

func (s *Server) reloadKubernetesClient() error {
	k, err := internalk8s.NewManager(s.configuration.StaticConfig)
	if err != nil {
		return err
	}
	s.k = k
	applicableTools := make([]api.ServerTool, 0)
	for _, toolset := range s.configuration.Toolsets() {
		for _, tool := range toolset.GetTools(s.k) {
			if !s.configuration.isToolApplicable(tool) {
				continue
			}
			applicableTools = append(applicableTools, tool)
			s.enabledTools = append(s.enabledTools, tool.Tool.Name)
		}
	}
	m3labsServerTools, err := ServerToolToM3LabsServerTool(s, applicableTools)
	if err != nil {
		return fmt.Errorf("failed to convert tools: %v", err)
	}
	s.server.SetTools(m3labsServerTools...)
	return nil
}

func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.server)
}

func (s *Server) ServeSse(baseUrl string, httpServer *http.Server) *server.SSEServer {
	options := make([]server.SSEOption, 0)
	options = append(options, server.WithSSEContextFunc(contextFunc), server.WithHTTPServer(httpServer))
	if baseUrl != "" {
		options = append(options, server.WithBaseURL(baseUrl))
	}
	return server.NewSSEServer(s.server, options...)
}

func (s *Server) ServeHTTP(httpServer *http.Server) *server.StreamableHTTPServer {
	options := []server.StreamableHTTPOption{
		server.WithHTTPContextFunc(contextFunc),
		server.WithStreamableHTTPServer(httpServer),
		server.WithStateLess(true),
	}
	return server.NewStreamableHTTPServer(s.server, options...)
}

// KubernetesApiVerifyToken verifies the given token with the audience by
// sending an TokenReview request to API Server.
func (s *Server) KubernetesApiVerifyToken(ctx context.Context, token string, audience string) (*authenticationapiv1.UserInfo, []string, error) {
	if s.k == nil {
		return nil, nil, fmt.Errorf("kubernetes manager is not initialized")
	}
	return s.k.VerifyToken(ctx, token, audience)
}

// GetKubernetesAPIServerHost returns the Kubernetes API server host from the configuration.
func (s *Server) GetKubernetesAPIServerHost() string {
	if s.k == nil {
		return ""
	}
	return s.k.GetAPIServerHost()
}

func (s *Server) GetEnabledTools() []string {
	return s.enabledTools
}

func (s *Server) Close() {
	if s.k != nil {
		s.k.Close()
	}
}

func NewTextResult(content string, err error) *mcp.CallToolResult {
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: err.Error(),
				},
			},
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: content,
			},
		},
	}
}

func contextFunc(ctx context.Context, r *http.Request) context.Context {
	// Get the standard Authorization header (OAuth compliant)
	authHeader := r.Header.Get(string(internalk8s.OAuthAuthorizationHeader))
	if authHeader != "" {
		return context.WithValue(ctx, internalk8s.OAuthAuthorizationHeader, authHeader)
	}

	// Fallback to custom header for backward compatibility
	customAuthHeader := r.Header.Get(string(internalk8s.CustomAuthorizationHeader))
	if customAuthHeader != "" {
		return context.WithValue(ctx, internalk8s.OAuthAuthorizationHeader, customAuthHeader)
	}

	return ctx
}

func toolCallLoggingMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		klog.V(5).Infof("mcp tool call: %s(%v)", ctr.Params.Name, ctr.Params.Arguments)
		if ctr.Header != nil {
			buffer := bytes.NewBuffer(make([]byte, 0))
			if err := ctr.Header.WriteSubset(buffer, map[string]bool{"Authorization": true, "authorization": true}); err == nil {
				klog.V(7).Infof("mcp tool call headers: %s", buffer)
			}
		}
		return next(ctx, ctr)
	}
}

func toolScopedAuthorizationMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		scopes, ok := ctx.Value(TokenScopesContextKey).([]string)
		if !ok {
			return NewTextResult("", fmt.Errorf("authorization failed: Access denied: Tool '%s' requires scope 'mcp:%s' but no scope is available", ctr.Params.Name, ctr.Params.Name)), nil
		}
		if !slices.Contains(scopes, "mcp:"+ctr.Params.Name) && !slices.Contains(scopes, ctr.Params.Name) {
			return NewTextResult("", fmt.Errorf("authorization failed: Access denied: Tool '%s' requires scope 'mcp:%s' but only scopes %s are available", ctr.Params.Name, ctr.Params.Name, scopes)), nil
		}
		return next(ctx, ctr)
	}
}
