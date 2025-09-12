package full

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func initConfiguration() []api.ServerTool {
	tools := []api.ServerTool{
		{Tool: api.Tool{
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
			Annotations: api.ToolAnnotations{
				Title:           "Configuration: View",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: configurationView},
	}
	return tools
}

func configurationView(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	minify := true
	minified := params.GetArguments()["minified"]
	if _, ok := minified.(bool); ok {
		minify = minified.(bool)
	}
	ret, err := params.ConfigurationView(minify)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get configuration: %v", err)), nil
	}
	configurationYaml, err := output.MarshalYaml(ret)
	if err != nil {
		err = fmt.Errorf("failed to get configuration: %v", err)
	}
	return api.NewToolCallResult(configurationYaml, err), nil
}
