package config

import (
	"fmt"
	"sort"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

// ContextInfo describes a single kubeconfig context entry in the
// configuration_contexts_list tool's structured content payload.
//
// This is part of the tool's public wire contract: the JSON field tags are
// observed by MCP clients and must not be renamed without a coordinated
// migration. See docs/specs/structured-output.md for the repo-wide
// structured-content conventions.
type ContextInfo struct {
	Name    string `json:"name"`
	Server  string `json:"server"`
	Default bool   `json:"default"`
}

// ContextsListResult is the structured-content payload returned by the
// configuration_contexts_list tool.
//
// This is part of the tool's public wire contract: the JSON field tags are
// observed by MCP clients and must not be renamed without a coordinated
// migration. See docs/specs/structured-output.md.
type ContextsListResult struct {
	DefaultContext string        `json:"defaultContext"`
	Contexts       []ContextInfo `json:"contexts"`
}

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
		// Structured content is intentionally omitted (nil) on this branch — see
		// docs/specs/structured-output.md "Empty / nil results".
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
	contextNames := make([]string, 0, len(contexts))
	for context := range contexts {
		contextNames = append(contextNames, context)
	}
	// Lexicographic order is the documented contract — see
	// docs/specs/structured-output.md "Ordering discipline". Note this places
	// numeric-looking names like "cluster-10" before "cluster-2".
	sort.Strings(contextNames)

	structured := ContextsListResult{
		DefaultContext: defaultContext,
		Contexts:       make([]ContextInfo, 0, len(contexts)),
	}
	for _, context := range contextNames {
		server := contexts[context]
		marker := " "
		if context == defaultContext {
			marker = "*"
		}

		result += fmt.Sprintf("%s%s -> %s\n", marker, context, server)
		structured.Contexts = append(structured.Contexts, ContextInfo{
			Name:    context,
			Server:  server,
			Default: context == defaultContext,
		})
	}
	result += "---------\n\n"

	result += "To use a specific context with any tool, set the 'context' parameter in the tool call arguments"

	// TODO: Review the unstructured text fallback above (see #1151 review).
	// Per the MCP spec the text block alongside structuredContent is SHOULD-
	// level, so its format is still worth revisiting independently (e.g. for
	// LLM legibility and parseability).
	return api.NewToolCallResultFull(result, structured, nil), nil
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
