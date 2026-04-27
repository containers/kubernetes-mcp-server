package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/metrics"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/prompts"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
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
	mu                       sync.RWMutex
	configuration            *Configuration
	server                   *mcp.Server
	enabledTools             []string
	enabledPrompts           []string
	enabledResources         []string
	enabledResourceTemplates []string
	p                        internalk8s.Provider
	metrics                  *metrics.Metrics // Metrics collection system
	rateLimitDone            chan struct{}    // Closed to stop the rate limiter reaper goroutine
	closeOnce                sync.Once
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
					Resources: &mcp.ResourceCapabilities{ListChanged: !configuration.Stateless},
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
	s.rateLimitDone = make(chan struct{})
	s.server.AddReceivingMiddleware(
		rateLimitingMiddleware(s.rateLimitDone, func() (rate.Limit, int) {
			s.mu.RLock()
			rps := s.configuration.HTTP.RateLimitRPS
			burst := s.configuration.HTTP.RateLimitBurst
			s.mu.RUnlock()
			if burst == 0 {
				burst = config.DefaultRateLimitBurst
			}
			return rate.Limit(rps), burst
		}),
	)
	s.server.AddReceivingMiddleware(tracingMiddleware(version.BinaryName + "/mcp"))
	s.server.AddReceivingMiddleware(authHeaderPropagationMiddleware)
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
	applicableResources := s.collectApplicableResources()
	applicableResourceTemplates := s.collectApplicableResourceTemplates()

	// Read the previous state with read lock - don't hold lock while calling external code
	s.mu.RLock()
	previousTools := s.enabledTools
	previousPrompts := s.enabledPrompts
	previousResources := s.enabledResources
	previousResourceTemplates := s.enabledResourceTemplates
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

	// Reload resources
	newResources, err := reloadItems(
		previousResources,
		applicableResources,
		func(r api.ServerResource) string { return r.Resource.URI },
		s.server.RemoveResources,
		s.registerResource,
	)
	if err != nil {
		return err
	}

	// Reload resource templates
	newResourceTemplates, err := reloadItems(
		previousResourceTemplates,
		applicableResourceTemplates,
		func(rt api.ServerResourceTemplate) string { return rt.ResourceTemplate.URITemplate },
		s.server.RemoveResourceTemplates,
		s.registerResourceTemplate,
	)
	if err != nil {
		return err
	}

	// Only hold write lock for the final assignment
	s.mu.Lock()
	s.enabledTools = newTools
	s.enabledPrompts = newPrompts
	s.enabledResources = newResources
	s.enabledResourceTemplates = newResourceTemplates
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
		WithToolOverrides(s.configuration.ToolOverrides),
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

// collectApplicablePrompts returns prompts after applying mutation and merging toolset and config prompts
func (s *Server) collectApplicablePrompts() []api.ServerPrompt {
	mutator := WithPromptTargetParameter(s.p.GetDefaultTarget(), s.p.GetTargetParameterName(), s.p.IsMultiTarget())

	toolsetPrompts := make([]api.ServerPrompt, 0)
	for _, toolset := range s.configuration.Toolsets() {
		for _, prompt := range toolset.GetPrompts() {
			toolsetPrompts = append(toolsetPrompts, mutator(prompt))
		}
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

// collectApplicableResources returns resources from all enabled toolsets after filtering and mutation
func (s *Server) collectApplicableResources() []api.ServerResource {
	filter := CompositeResourceFilter()
	mutator := ComposeResourceMutators()

	resources := make([]api.ServerResource, 0)
	for _, toolset := range s.configuration.Toolsets() {
		for _, resource := range toolset.GetResources() {
			resource = mutator(resource)
			if filter(resource) {
				resources = append(resources, resource)
			}
		}
	}
	return resources
}

// collectApplicableResourceTemplates returns resource templates from all enabled toolsets after filtering and mutation
func (s *Server) collectApplicableResourceTemplates() []api.ServerResourceTemplate {
	filter := CompositeResourceTemplateFilter()
	mutator := ComposeResourceTemplateMutators()

	templates := make([]api.ServerResourceTemplate, 0)
	for _, toolset := range s.configuration.Toolsets() {
		for _, template := range toolset.GetResourceTemplates() {
			template = mutator(template)
			if filter(template) {
				templates = append(templates, template)
			}
		}
	}
	return templates
}

// registerResource registers a resource with the MCP server
func (s *Server) registerResource(res api.ServerResource) error {
	addResource(s.server, res)
	return nil
}

// registerResourceTemplate registers a resource template with the MCP server
func (s *Server) registerResourceTemplate(rt api.ServerResourceTemplate) error {
	addResourceTemplate(s.server, rt)
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

// GetEnabledPrompts returns the names of the currently enabled prompts
func (s *Server) GetEnabledPrompts() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabledPrompts
}

// GetEnabledResources returns the URIs of the currently enabled resources
func (s *Server) GetEnabledResources() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabledResources
}

// GetEnabledResourceTemplates returns the URI templates of the currently enabled resource templates
func (s *Server) GetEnabledResourceTemplates() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabledResourceTemplates
}

// ReloadConfiguration reloads the configuration and reinitializes the server.
// This is intended to be called by the server lifecycle manager when
// configuration changes are detected.
func (s *Server) ReloadConfiguration(newConfig *config.StaticConfig) error {
	klog.V(1).Info("Reloading MCP server configuration...")

	// Validate config-level invariants (same checks as startup)
	if err := newConfig.
		WithProviderStrategies(internalk8s.GetRegisteredStrategies()).
		WithTokenExchangeStrategies(tokenexchange.GetRegisteredStrategies()).
		Validate(); err != nil {
		return fmt.Errorf("configuration reload rejected: %w", err)
	}

	// Update the configuration (protected by mu so concurrent readers see a
	// consistent snapshot, e.g. the rate-limit configFn closure).
	s.mu.Lock()
	s.configuration.StaticConfig = newConfig
	// Clear cached values so they get recomputed
	s.configuration.listOutput = nil
	s.configuration.toolsets = nil
	s.mu.Unlock()

	// Reload the Kubernetes provider (this will also rebuild tools)
	if err := s.reloadToolsets(); err != nil {
		return fmt.Errorf("failed to reload toolsets: %w", err)
	}

	klog.V(1).Info("MCP server configuration reloaded successfully")
	return nil
}

func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.rateLimitDone)
		if s.p != nil {
			s.p.Close()
		}
	})
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

// ResourceFilter is a function that takes a ServerResource and returns a boolean indicating whether to include it
type ResourceFilter func(resource api.ServerResource) bool

// CompositeResourceFilter combines multiple resource filters into a single filter using AND logic
func CompositeResourceFilter(filters ...ResourceFilter) ResourceFilter {
	return func(resource api.ServerResource) bool {
		for _, f := range filters {
			if !f(resource) {
				return false
			}
		}
		return true
	}
}

// ResourceMutator is a function that transforms a ServerResource
type ResourceMutator func(resource api.ServerResource) api.ServerResource

// ComposeResourceMutators combines multiple resource mutators into a pipeline
func ComposeResourceMutators(mutators ...ResourceMutator) ResourceMutator {
	return func(resource api.ServerResource) api.ServerResource {
		for _, m := range mutators {
			resource = m(resource)
		}
		return resource
	}
}

// ResourceTemplateFilter is a function that takes a ServerResourceTemplate and returns a boolean indicating whether to include it
type ResourceTemplateFilter func(template api.ServerResourceTemplate) bool

// CompositeResourceTemplateFilter combines multiple resource template filters into a single filter using AND logic
func CompositeResourceTemplateFilter(filters ...ResourceTemplateFilter) ResourceTemplateFilter {
	return func(template api.ServerResourceTemplate) bool {
		for _, f := range filters {
			if !f(template) {
				return false
			}
		}
		return true
	}
}

// ResourceTemplateMutator is a function that transforms a ServerResourceTemplate
type ResourceTemplateMutator func(template api.ServerResourceTemplate) api.ServerResourceTemplate

// ComposeResourceTemplateMutators combines multiple resource template mutators into a pipeline
func ComposeResourceTemplateMutators(mutators ...ResourceTemplateMutator) ResourceTemplateMutator {
	return func(template api.ServerResourceTemplate) api.ServerResourceTemplate {
		for _, m := range mutators {
			template = m(template)
		}
		return template
	}
}
