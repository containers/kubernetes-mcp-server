package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
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

// validateScopeConfig validates scope configuration and logs warnings for potential issues.
// Returns an error only for critical misconfigurations.
func validateScopeConfig(cfg *config.StaticConfig) error {
	// Only validate scopes when OAuth is enabled
	if !cfg.IsRequireOAuth() {
		return nil
	}

	// Reject empty scope names when OAuth is enabled
	if cfg.ReadScope == "" {
		return fmt.Errorf("read_scope must not be empty when require_oauth is enabled")
	}
	if cfg.WriteScope == "" {
		return fmt.Errorf("write_scope must not be empty when require_oauth is enabled")
	}

	// Warn if scope names contain spaces (should be single tokens)
	if strings.Contains(cfg.ReadScope, " ") {
		klog.Warningf("read_scope %q contains spaces; scope names should be single tokens", cfg.ReadScope)
	}
	if strings.Contains(cfg.WriteScope, " ") {
		klog.Warningf("write_scope %q contains spaces; scope names should be single tokens", cfg.WriteScope)
	}

	// Warn if read and write scopes are the same (unusual configuration)
	if cfg.ReadScope == cfg.WriteScope {
		klog.Warningf("read_scope and write_scope are both %q; this means all tools require the same scope", cfg.ReadScope)
	}

	return nil
}

type Server struct {
	mu             sync.RWMutex
	configuration  *Configuration
	server         *mcp.Server
	enabledTools   []string
	toolsByName    map[string]*api.ServerTool // For scope validation lookups
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

	// Validate scope configuration (logs warnings for potential issues)
	if err := validateScopeConfig(configuration.StaticConfig); err != nil {
		return nil, err
	}

	s.server.AddReceivingMiddleware(sessionInjectionMiddleware)
	s.server.AddReceivingMiddleware(traceContextPropagationMiddleware)
	s.server.AddReceivingMiddleware(tracingMiddleware(version.BinaryName + "/mcp"))
	s.server.AddReceivingMiddleware(authHeaderPropagationMiddleware)
	s.server.AddReceivingMiddleware(scopeValidationMiddleware(
		func() (string, string) {
			return s.configuration.ReadScope, s.configuration.WriteScope
		},
		s.GetToolByName,
	))
	s.server.AddReceivingMiddleware(userAgentPropagationMiddleware(version.BinaryName, version.Version))
	s.server.AddReceivingMiddleware(toolCallLoggingMiddleware)
	s.server.AddReceivingMiddleware(s.metricsMiddleware())

	err = s.reloadToolsets()
	if err != nil {
		return nil, err
	}
	s.p.WatchTargets(s.reloadToolsets)

	return s, nil
}

func (s *Server) reloadToolsets() error {
	// TODO: No option to perform a full replacement of tools.
	// s.server.SetTools(tools...)

	// Collect applicable items
	applicableTools := s.collectApplicableTools()
	applicablePrompts := s.collectApplicablePrompts()

	// Read the previous state with read lock - don't hold lock while calling external code
	s.mu.RLock()
	previousTools := s.enabledTools
	previousPrompts := s.enabledPrompts
	s.mu.RUnlock()

	// Reload tools (calls s.server.AddTool/RemoveTools - external code, no lock held)
	newTools, err := reloadItems(
		previousTools,
		applicableTools,
		func(t api.ServerTool) string { return t.Tool.Name },
		s.server.RemoveTools,
		s.registerTool,
	)
	if err != nil {
		return err
	}

	// Reload prompts (calls s.server.AddPrompt/RemovePrompts - external code, no lock held)
	newPrompts, err := reloadItems(
		previousPrompts,
		applicablePrompts,
		func(p api.ServerPrompt) string { return p.Prompt.Name },
		s.server.RemovePrompts,
		s.registerPrompt,
	)
	if err != nil {
		return err
	}

	// Build tools lookup map for scope validation
	newToolsByName := make(map[string]*api.ServerTool, len(applicableTools))
	for i := range applicableTools {
		newToolsByName[applicableTools[i].Tool.Name] = &applicableTools[i]
	}

	// Only hold write lock for the final assignment
	s.mu.Lock()
	s.enabledTools = newTools
	s.toolsByName = newToolsByName
	s.enabledPrompts = newPrompts
	s.mu.Unlock()

	// Start new watch
	s.p.WatchTargets(s.reloadToolsets)
	return nil
}

// reloadItems handles the common pattern of reloading MCP server items.
// It removes items that are no longer applicable, registers new items,
// and returns the updated list of enabled item names.
func reloadItems[T any](
	previous []string,
	items []T,
	getName func(T) string,
	remove func(...string),
	register func(T) error,
) ([]string, error) {
	// Build new enabled list
	enabled := make([]string, 0, len(items))
	for _, item := range items {
		enabled = append(enabled, getName(item))
	}

	// Remove items that are no longer applicable
	toRemove := make([]string, 0)
	for _, old := range previous {
		if !slices.Contains(enabled, old) {
			toRemove = append(toRemove, old)
		}
	}
	remove(toRemove...)

	// Register all items
	for _, item := range items {
		if err := register(item); err != nil {
			return nil, err
		}
	}

	return enabled, nil
}

// collectApplicableTools returns tools after applying filtering and mutation
func (s *Server) collectApplicableTools() []api.ServerTool {
	filter := CompositeFilter(
		s.configuration.isToolApplicable,
		ShouldIncludeTargetListTool(s.p.GetTargetParameterName(), s.p.IsMultiTarget()),
	)
	mutator := ComposeMutators(
		WithTargetParameter(s.p.GetDefaultTarget(), s.p.GetTargetParameterName(), s.p.IsMultiTarget()),
		WithTargetListTool(s.p.GetDefaultTarget(), s.p.GetTargetParameterName(), s.p),
	)

	tools := make([]api.ServerTool, 0)
	for _, toolset := range s.configuration.Toolsets() {
		for _, tool := range toolset.GetTools(s.p) {
			tool = mutator(tool)
			if filter(tool) {
				tools = append(tools, tool)
			}
		}
	}
	return tools
}

// collectApplicablePrompts returns prompts after merging toolset and config prompts
func (s *Server) collectApplicablePrompts() []api.ServerPrompt {
	toolsetPrompts := make([]api.ServerPrompt, 0)
	for _, toolset := range s.configuration.Toolsets() {
		toolsetPrompts = append(toolsetPrompts, toolset.GetPrompts()...)
	}
	configPrompts := prompts.ToServerPrompts(s.configuration.Prompts)
	return prompts.MergePrompts(toolsetPrompts, configPrompts)
}

// registerTool converts and registers a tool with the MCP server
func (s *Server) registerTool(tool api.ServerTool) error {
	goSdkTool, goSdkToolHandler, err := ServerToolToGoSdkTool(s, tool)
	if err != nil {
		return fmt.Errorf("failed to convert tool %s: %w", tool.Tool.Name, err)
	}
	s.server.AddTool(goSdkTool, goSdkToolHandler)
	return nil
}

// registerPrompt converts and registers a prompt with the MCP server
func (s *Server) registerPrompt(prompt api.ServerPrompt) error {
	mcpPrompt, promptHandler, err := ServerPromptToGoSdkPrompt(s, prompt)
	if err != nil {
		return fmt.Errorf("failed to convert prompt %s: %w", prompt.Prompt.Name, err)
	}
	s.server.AddPrompt(mcpPrompt, promptHandler)
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabledTools
}

// GetToolByName returns a tool by name for scope validation.
// Returns nil if the tool is not found.
func (s *Server) GetToolByName(name string) *api.ServerTool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.toolsByName[name]
}

// GetEnabledPrompts returns the names of the currently enabled prompts
func (s *Server) GetEnabledPrompts() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabledPrompts
}

// ReloadConfiguration reloads the configuration and reinitializes the server.
// This is intended to be called by the server lifecycle manager when
// configuration changes are detected.
func (s *Server) ReloadConfiguration(newConfig *config.StaticConfig) error {
	klog.V(1).Info("Reloading MCP server configuration...")

	// Validate scope configuration before applying
	if err := validateScopeConfig(newConfig); err != nil {
		return fmt.Errorf("invalid scope configuration: %w", err)
	}

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

// NewTextResult creates an MCP CallToolResult with text content only.
// Use this for tools that return human-readable text output.
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

// NewStructuredResult creates an MCP CallToolResult with structured content.
// The Content field contains the JSON-serialized form of structuredContent
// for backward compatibility with MCP clients that don't support structuredContent.
//
// Per the MCP specification, structuredContent must marshal to a JSON object.
// If structuredContent is a slice/array, it is automatically wrapped in
// {"items": [...]} to satisfy this requirement.
//
// Per the MCP specification:
// "For backwards compatibility, a tool that returns structured content SHOULD
// also return the serialized JSON in a TextContent block."
// https://modelcontextprotocol.io/specification/2025-11-25/server/tools#structured-content
//
// Use this for tools that return typed/structured data that MCP clients can
// parse programmatically.
func NewStructuredResult(content string, structuredContent any, err error) *mcp.CallToolResult {
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
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: content,
			},
		},
	}
	if structuredContent != nil {
		result.StructuredContent = ensureStructuredObject(structuredContent)
	}
	return result
}

// ensureStructuredObject wraps slice/array values in a {"items": ...} object
// because the MCP specification requires structuredContent to be a JSON object.
// A typed nil slice (e.g. []string(nil)) returns nil to avoid {"items": null}.
// Note: this checks the top-level reflect.Kind, so a pointer-to-slice (*[]T)
// would not be wrapped. All current callers pass value types.
func ensureStructuredObject(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		if rv.IsNil() {
			return nil
		}
		return map[string]any{"items": v}
	}
	if rv.Kind() == reflect.Array {
		return map[string]any{"items": v}
	}
	return v
}
