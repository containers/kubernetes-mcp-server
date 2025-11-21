package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	authenticationapiv1 "k8s.io/api/authentication/v1"
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
	server        *mcp.Server
	enabledTools  []string
	p             internalk8s.Provider
}

func NewServer(configuration Configuration) (*Server, error) {
	s := &Server{
		configuration: &configuration,
		server: mcp.NewServer(
			&mcp.Implementation{
				Name: version.BinaryName, Title: version.BinaryName, Version: version.Version,
			},
			&mcp.ServerOptions{
				HasResources: false,
				HasPrompts:   false,
				HasTools:     true,
			}),
	}

	s.server.AddReceivingMiddleware(authHeaderPropagationMiddleware)
	s.server.AddReceivingMiddleware(toolCallLoggingMiddleware)
	if configuration.RequireOAuth && false { // TODO: Disabled scope auth validation for now
		s.server.AddReceivingMiddleware(toolScopedAuthorizationMiddleware)
	}

	if err := s.reloadKubernetesClusterProvider(); err != nil {
		return nil, err
	}
	s.p.WatchTargets(s.reloadKubernetesClusterProvider)

	return s, nil
}

func (s *Server) reloadKubernetesClusterProvider() error {
	ctx := context.Background()

	newProvider, err := internalk8s.NewProvider(s.configuration.StaticConfig)
	if err != nil {
		return err
	}

	targets, err := newProvider.GetTargets(ctx)
	if err != nil {
		newProvider.Close()
		return err
	}

	if s.p != nil {
		s.p.Close()
	}

	s.p = newProvider

	if err := s.rebuildTools(targets); err != nil {
		return err
	}

	s.p.WatchTargets(s.reloadKubernetesClusterProvider)

	return nil
}

// rebuildTools rebuilds the MCP tool registry based on the current provider and targets.
// This is called after the provider has been successfully validated and set.
func (s *Server) rebuildTools(targets []string) error {
	filter := CompositeFilter(
		s.configuration.isToolApplicable,
		ShouldIncludeTargetListTool(s.p.GetTargetParameterName(), targets),
	)

	mutator := WithTargetParameter(
		s.p.GetDefaultTarget(),
		s.p.GetTargetParameterName(),
		targets,
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

	// Add new tools
	for _, tool := range applicableTools {
		goSdkTool, goSdkToolHandler, err := ServerToolToGoSdkTool(s, tool)
		if err != nil {
			return fmt.Errorf("failed to convert tool %s: %v", tool.Tool.Name, err)
		}
		s.server.AddTool(goSdkTool, goSdkToolHandler)
	}

	return nil
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
		// For clients to be able to listen to tool changes, we need to set the server stateful
		// https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#listening-for-messages-from-the-server
		Stateless: false,
	})
}

// KubernetesApiVerifyToken verifies the given token with the audience by
// sending an TokenReview request to API Server for the specified cluster.
func (s *Server) KubernetesApiVerifyToken(ctx context.Context, cluster, token, audience string) (*authenticationapiv1.UserInfo, []string, error) {
	if s.p == nil {
		return nil, nil, fmt.Errorf("kubernetes cluster provider is not initialized")
	}
	return s.p.VerifyToken(ctx, cluster, token, audience)
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

func (s *Server) Close() {
	if s.p != nil {
		s.p.Close()
	}
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
