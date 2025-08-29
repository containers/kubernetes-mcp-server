package mcp

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) initConfiguration() []server.ServerTool {
	tools := []server.ServerTool{
		{Tool: mcp.NewTool("configuration_view",
			mcp.WithDescription("Get the current Kubernetes configuration content as a kubeconfig YAML"),
			mcp.WithBoolean("minified", mcp.Description("Return a minified version of the configuration. "+
				"If set to true, keeps only the current-context and the relevant pieces of the configuration for that context. "+
				"If set to false, all contexts, clusters, auth-infos, and users are returned in the configuration. "+
				"(Optional, default true)")),
			// Tool annotations
			mcp.WithTitleAnnotation("Configuration: View"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		), Handler: s.configurationView},
		{Tool: mcp.NewTool("configuration_switch_context",
			mcp.WithDescription("Switch the current Kubernetes context to a different context"),
			mcp.WithString("context", mcp.Description("The name of the context to switch to. Use configuration_view to see available contexts.")),
			// Tool annotations
			mcp.WithTitleAnnotation("Configuration: Switch Context"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(false),
		), Handler: s.configurationSwitchContext},
	}
	return tools
}

func (s *Server) configurationView(_ context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	minify := true
	minified := ctr.GetArguments()["minified"]
	if _, ok := minified.(bool); ok {
		minify = minified.(bool)
	}
	ret, err := s.k.ConfigurationView(minify)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to get configuration: %v", err)), nil
	}
	configurationYaml, err := output.MarshalYaml(ret)
	if err != nil {
		err = fmt.Errorf("failed to get configuration: %v", err)
	}
	return NewTextResult(configurationYaml, err), nil
}

func (s *Server) configurationSwitchContext(_ context.Context, ctr mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contextName, ok := ctr.GetArguments()["context"].(string)
	if !ok || contextName == "" {
		return NewTextResult("", fmt.Errorf("context parameter is required and must be a string")), nil
	}

	err := s.k.SwitchContext(contextName)
	if err != nil {
		return NewTextResult("", fmt.Errorf("failed to switch context: %v", err)), nil
	}

	return NewTextResult(fmt.Sprintf("Successfully switched to context: %s", contextName), nil), nil
}
