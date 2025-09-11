package mcp

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func (s *Server) initConfiguration() []ServerTool {
	tools := []ServerTool{
		{Tool: Tool{
			Name:        "configuration_view",
			Description: "Get the current Kubernetes configuration content as a kubeconfig YAML",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"minified": {
						Type: "boolean",
						Description: "Return a minified version of the configuration. " +
							"If set to true, keeps only the current-context and the relevant pieces of the configuration for that context. " +
							"If set to false, all contexts, clusters, auth-infos, and users are returned in the configuration. " +
							"(Optional, default true)",
					},
				},
			},
			Annotations: ToolAnnotations{
				Title:           "Configuration: View",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: s.configurationView},
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
