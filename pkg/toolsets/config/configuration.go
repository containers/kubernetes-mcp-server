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
		{
			Tool: api.Tool{
				Name:        "configuration_contexts_list",
				Description: "List all available context names and associated server urls from the kubeconfig file",
				InputSchema: &jsonschema.Schema{
					Type: "object",
				},
				Annotations: api.ToolAnnotations{
					Title:           "Configuration: Contexts List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			ClusterAware:       ptr.To(false),
			TargetListProvider: ptr.To(true),
			Handler:            contextsList,
		},
		// Generic targets list tool for non-kubeconfig providers (e.g., ACM).
		// The WithTargetListTool mutator will:
		// - Rename the tool to "{targetParameterName}_list" (e.g., "cluster_list")
		// - Update the description and title accordingly
		// - Set the handler with the actual targets
		{
			Tool: api.Tool{
				Name:        "targets_list",
				Description: "List all available targets",
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
			Handler:            nil,
		},
		{
			Tool: api.Tool{
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
					OpenWorldHint:   ptr.To(true),
				},
			},
			ClusterAware: ptr.To(false),
			Handler:      configurationView,
		},
	}
	return tools
}

func contextsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	contexts, err := kubernetes.NewCore(params).ConfigurationContextsList()
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list contexts: %w", err)), nil
	}

	if len(contexts) == 0 {
		return api.NewToolCallResult("No contexts found in kubeconfig", nil), nil
	}

	defaultContext, err := kubernetes.NewCore(params).ConfigurationContextsDefault()
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get default context: %w", err)), nil
	}

	result := fmt.Sprintf("Available Kubernetes contexts (%d total, default: %s):\n\n", len(contexts), defaultContext)
	result += "Format: [*] CONTEXT_NAME -> SERVER_URL\n"
	result += " (* indicates the default context used in tools if context is not set)\n\n"
	result += "Contexts:\n---------\n"
	for context, server := range contexts {
		marker := " "
		if context == defaultContext {
			marker = "*"
		}

		result += fmt.Sprintf("%s%s -> %s\n", marker, context, server)
	}
	result += "---------\n\n"

	result += "To use a specific context with any tool, set the 'context' parameter in the tool call arguments"

	// TODO: Review output format, current is not parseable and might not be ideal for LLM consumption
	return api.NewToolCallResult(result, nil), nil
}

func configurationView(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	minify := true
	minified := params.GetArguments()["minified"]
	if _, ok := minified.(bool); ok {
		minify = minified.(bool)
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
