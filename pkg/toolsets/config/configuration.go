package config

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func initConfiguration() []api.ServerTool {
	tools := []api.ServerTool{
		// The target list tool of every provider. The WithTargetListTool
		// mutator names it "{targetParameterName}_list" (context_list for a
		// kubeconfig, cluster_list for ACM), describes it in the provider's
		// terms and answers it from the provider's targets.
		{
			Tool: api.Tool{
				Name:        "targets_list",
				Description: "List the targets this server can run tools against.",
				Examples:    []api.Example{{Summary: "Which targets are available"}},
				InputSchema: &jsonschema.Schema{
					Type: "object",
				},
				Annotations: api.ToolAnnotations{
					Title:           "Targets List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			ClusterAware:       ptr.To(false),
			TargetListProvider: ptr.To(true),
			RBAC:               api.RBACNone(),
			Handler:            nil,
		},
		{
			Tool: api.Tool{
				Name:        "configuration_view",
				Description: "Show the kubeconfig the server runs with, as YAML: the current context alone by default, every context with minified false. Credentials in it are returned as they are.",
				Examples:    []api.Example{{Summary: "The current context's kubeconfig"}, {Summary: "The whole kubeconfig", Arguments: map[string]any{"minified": false}}},
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
					OpenWorldHint:   ptr.To(true),
				},
			},
			ClusterAware: ptr.To(false),
			RBAC:         api.RBACNone(),
			Handler:      configurationView,
		},
	}
	return tools
}

func configurationView(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	minify := p.OptionalBool("minified", true)
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get configuration: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).ConfigurationView(minify)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get configuration: %w", err)), nil
	}
	configurationYaml, err := output.MarshalYaml(ret)
	if err != nil {
		err = fmt.Errorf("failed to get configuration: %w", err)
	}
	return api.NewToolCallResult(configurationYaml, err), nil
}
