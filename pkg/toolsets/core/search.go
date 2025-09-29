package core

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func initSearch(_ kubernetes.Openshift) []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "search.Resources",
				Description: "Search for a string in all resources.",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"query": {
							Type:        "string",
							Description: "The string to search for in the resources.",
						},
						"as_table": {
							Type:        "boolean",
							Description: "Return the results as a table.",
						},
						"api_version": {
							Type:        "string",
							Description: "Optional API version of the resource to search in.",
						},
						"kind": {
							Type:        "string",
							Description: "Optional kind of the resource to search in.",
						},
						"namespace_label_selector": {
							Type:        "string",
							Description: "Optional label selector to filter namespaces.",
						},
					},
					Required: []string{"query"},
				},
				Annotations: api.ToolAnnotations{
					ReadOnlyHint: ptr.To(true),
				},
			},
			Handler: searchResources,
		},
	}
}

func searchResources(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	query, ok := params.GetArguments()["query"].(string)
	if !ok {
		return api.NewToolCallResult("", fmt.Errorf("query is not a string")), nil
	}

	asTable := false
	if val, ok := params.GetArguments()["as_table"].(bool); ok {
		asTable = val
	}

	apiVersion := ""
	if val, ok := params.GetArguments()["api_version"].(string); ok {
		apiVersion = val
	}

	kind := ""
	if val, ok := params.GetArguments()["kind"].(string); ok {
		kind = val
	}

	namespaceLabelSelector := ""
	if val, ok := params.GetArguments()["namespace_label_selector"].(string); ok {
		namespaceLabelSelector = val
	}

	result, err := params.SearchResources(params, query, apiVersion, kind, namespaceLabelSelector, asTable)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to search resources: %v", err)), nil
	}

	return api.NewToolCallResult(params.ListOutput.PrintObj(result)), nil
}
