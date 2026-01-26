package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/metrics"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/prompts"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/version"
)

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
	configuration  *Configuration
	server         *mcp.Server
	enabledTools   []string
	enabledPrompts []string
	p              internalk8s.Provider
	metrics        *metrics.Metrics // Metrics collection system
}

func NewServer(configuration Configuration, targetProvider internalk8s.Provider) (*Server, error) {
	s := &Server{
		configuration: &configuration,
		server: mcp.NewServer(
			&mcp.Implementation{
				Name:       version.BinaryName,
				Title:      version.BinaryName,
				Version:    version.Version,
				WebsiteURL: version.WebsiteURL,
			},
			&mcp.ServerOptions{
				Capabilities: &mcp.ServerCapabilities{
					Resources: nil,
					Prompts:   &mcp.PromptCapabilities{ListChanged: !configuration.Stateless},
					Tools:     &mcp.ToolCapabilities{ListChanged: !configuration.Stateless},
					Logging:   &mcp.LoggingCapabilities{},
				},
				Instructions: configuration.ServerInstructions,
			}),
		p: targetProvider,
	}

	// Initialize metrics system
	metricsInstance, err := metrics.New(metrics.Config{
		TracerName:     version.BinaryName + "/mcp",
		ServiceName:    version.BinaryName,
		ServiceVersion: version.Version,
		Telemetry:      &configuration.Telemetry,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}
	s.metrics = metricsInstance

	s.server.AddReceivingMiddleware(sessionInjectionMiddleware)
	s.server.AddReceivingMiddleware(traceContextPropagationMiddleware)
	s.server.AddReceivingMiddleware(tracingMiddleware(version.BinaryName + "/mcp"))
	s.server.AddReceivingMiddleware(authHeaderPropagationMiddleware)
	s.server.AddReceivingMiddleware(toolCallLoggingMiddleware)
	s.server.AddReceivingMiddleware(s.metricsMiddleware())
	if configuration.RequireOAuth && false { // TODO: Disabled scope auth validation for now
		s.server.AddReceivingMiddleware(toolScopedAuthorizationMiddleware)
	}
	err = s.reloadToolsets()
	if err != nil {
		return nil, err
	}
	s.p.WatchTargets(s.reloadToolsets)

	return s, nil
}

func (s *Server) reloadToolsets() error {
	ctx := context.Background()

	targets, err := s.p.GetTargets(ctx)
	if err != nil {
		return err
	}

	filter := CompositeFilter(
		s.configuration.isToolApplicable,
		ShouldIncludeTargetListTool(s.p.GetTargetParameterName(), targets),
	)

	mutator := ComposeMutators(
		WithTargetParameter(s.p.GetDefaultTarget(), s.p.GetTargetParameterName(), targets),
		WithTargetListTool(s.p.GetDefaultTarget(), s.p.GetTargetParameterName(), targets),
	)

	// TODO: No option to perform a full replacement of tools.
	// s.server.SetTools(m3labsServerTools...)

	// Track previously enabled tools
	previousTools := s.enabledTools

	// Build new list of applicable tools
	applicableTools := make([]api.ServerTool, 0)
	s.enabledTools = make([]string, 0)
	for _, toolset := range s.configuration.Toolsets() {
		for _, tool := range toolset.GetTools(s.p) {
			tool := mutator(tool)
			if !filter(tool) {
				continue
			}

			applicableTools = append(applicableTools, tool)
			s.enabledTools = append(s.enabledTools, tool.Tool.Name)
		}
	}

	// TODO: No option to perform a full replacement of tools.
	// Remove tools that are no longer applicable
	toolsToRemove := make([]string, 0)
	for _, oldTool := range previousTools {
		if !slices.Contains(s.enabledTools, oldTool) {
			toolsToRemove = append(toolsToRemove, oldTool)
		}
	}
	s.server.RemoveTools(toolsToRemove...)

	for _, tool := range applicableTools {
		goSdkTool, goSdkToolHandler, err := ServerToolToGoSdkTool(s, tool)
		if err != nil {
			return fmt.Errorf("failed to convert tool %s: %w", tool.Tool.Name, err)
		}
		s.server.AddTool(goSdkTool, goSdkToolHandler)
	}

	// Track previously enabled prompts
	previousPrompts := s.enabledPrompts

	// Build and register prompts from all toolsets
	toolsetPrompts := make([]api.ServerPrompt, 0)
	// Load embedded toolset prompts
	for _, toolset := range s.configuration.Toolsets() {
		toolsetPrompts = append(toolsetPrompts, toolset.GetPrompts()...)
	}

	configPrompts := prompts.ToServerPrompts(s.configuration.Prompts)

	// Merge: config prompts override embedded prompts with same name
	applicablePrompts := prompts.MergePrompts(toolsetPrompts, configPrompts)

	// Update enabled prompts list
	s.enabledPrompts = make([]string, 0)
	for _, prompt := range applicablePrompts {
		s.enabledPrompts = append(s.enabledPrompts, prompt.Prompt.Name)
	}

	// Remove prompts that are no longer applicable
	promptsToRemove := make([]string, 0)
	for _, oldPrompt := range previousPrompts {
		if !slices.Contains(s.enabledPrompts, oldPrompt) {
			promptsToRemove = append(promptsToRemove, oldPrompt)
		}
	}
	s.server.RemovePrompts(promptsToRemove...)

	// Register all applicable prompts
	for _, prompt := range applicablePrompts {
		mcpPrompt, promptHandler, err := ServerPromptToGoSdkPrompt(s, prompt)
		if err != nil {
			return fmt.Errorf("failed to convert prompt %s: %w", prompt.Prompt.Name, err)
		}
		s.server.AddPrompt(mcpPrompt, promptHandler)
	}

	// start new watch
	s.p.WatchTargets(s.reloadToolsets)
	return nil
}

// metricsMiddleware returns a metrics middleware with access to the server's metrics system
func (s *Server) metricsMiddleware() func(mcp.MethodHandler) mcp.MethodHandler {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			start := time.Now()
			result, err := next(ctx, method, req)
			duration := time.Since(start)

			toolName := method
			if method == "tools/call" {
				if params, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
					if toolReq, _ := GoSdkToolCallParamsToToolCallRequest(params); toolReq != nil {
						toolName = toolReq.Name
					}
				}
			}

			// Record to all collectors
			s.metrics.RecordToolCall(ctx, toolName, duration, err)

			return result, err
		}
	}
}

// GetMetrics returns the metrics system for use by the HTTP server.
func (s *Server) GetMetrics() *metrics.Metrics {
	return s.metrics
}

func (s *Server) ServeStdio(ctx context.Context) error {
	return s.server.Run(ctx, &mcp.LoggingTransport{Transport: &mcp.StdioTransport{}, Writer: os.Stderr})
}

func (s *Server) ServeSse() *mcp.SSEHandler {
	return mcp.NewSSEHandler(func(request *http.Request) *mcp.Server {
		return s.server
	}, &mcp.SSEOptions{})
}

func (s *Server) ServeHTTP() *mcp.StreamableHTTPHandler {
	return mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server {
		return s.server
	}, &mcp.StreamableHTTPOptions{
		// Stateless mode configuration from server settings.
		// When Stateless is true, the server will not send notifications to clients
		// (e.g., tools/list_changed, prompts/list_changed). This disables dynamic
		// tool and prompt updates but is useful for container deployments, load
		// balancing, and serverless environments where maintaining client state
		// is not desired or possible.
		// https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#listening-for-messages-from-the-server
		Stateless: s.configuration.Stateless,
	})
}

// GetTargetParameterName returns the parameter name used for target identification in MCP requests
func (s *Server) GetTargetParameterName() string {
	if s.p == nil {
		return "" // fallback for uninitialized provider
	}
	return s.p.GetTargetParameterName()
}

func (s *Server) GetEnabledTools() []string {
	return s.enabledTools
}

// GetEnabledPrompts returns the names of the currently enabled prompts
func (s *Server) GetEnabledPrompts() []string {
	return s.enabledPrompts
}

// ReloadConfiguration reloads the configuration and reinitializes the server.
// This is intended to be called by the server lifecycle manager when
// configuration changes are detected.
func (s *Server) ReloadConfiguration(newConfig *config.StaticConfig) error {
	klog.V(1).Info("Reloading MCP server configuration...")

	// Update the configuration
	s.configuration.StaticConfig = newConfig
	// Clear cached values so they get recomputed
	s.configuration.listOutput = nil
	s.configuration.toolsets = nil

	// Reload the Kubernetes provider (this will also rebuild tools)
	if err := s.reloadToolsets(); err != nil {
		return fmt.Errorf("failed to reload toolsets: %w", err)
	}

	klog.V(1).Info("MCP server configuration reloaded successfully")
	return nil
}

func (s *Server) Close() {
	if s.p != nil {
		s.p.Close()
	}
}

// Shutdown gracefully shuts down the server, flushing any pending metrics.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.metrics != nil {
		if err := s.metrics.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown metrics: %w", err)
		}
	}
	s.Close()
	return nil
}

func NewTextResult(content string, err error) *mcp.CallToolResult {
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: err.Error(),
				},
			},
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: content,
			},
		},
	}
}
