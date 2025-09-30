package mcp

import (
	"fmt"
	"sort"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
)

type ToolMutator func(tool api.ServerTool) api.ServerTool

const maxClustersInEnum = 15 // TODO: test and validate that this is a reasonable cutoff

func WithClusterParameter(defaultCluster string, clusters, skipToolNames []string) ToolMutator {
	skipNames := make(map[string]struct{}, len(skipToolNames))
	for _, n := range skipToolNames {
		skipNames[n] = struct{}{}
	}

	return func(tool api.ServerTool) api.ServerTool {
		if _, ok := skipNames[tool.Tool.Name]; ok {
			return tool
		}

		if tool.Tool.InputSchema == nil {
			tool.Tool.InputSchema = &jsonschema.Schema{Type: "object"}
		}

		if tool.Tool.InputSchema.Properties == nil {
			tool.Tool.InputSchema.Properties = make(map[string]*jsonschema.Schema)
		}

		if len(clusters) > 1 {
			tool.Tool.InputSchema.Properties["cluster"] = createClusterProperty(defaultCluster, clusters)
		}

		return tool
	}
}

func createClusterProperty(defaultCluster string, clusters []string) *jsonschema.Schema {
	baseSchema := &jsonschema.Schema{
		Type: "string",
		Description: fmt.Sprintf(
			"Optional parameter selecting which cluster to run the tool in. Defaults to %s if not set", defaultCluster,
		),
	}

	if len(clusters) <= maxClustersInEnum {
		// Sort clusters to ensure consistent enum ordering
		sort.Strings(clusters)

		enumValues := make([]any, 0, len(clusters))
		for _, c := range clusters {
			enumValues = append(enumValues, c)
		}
		baseSchema.Enum = enumValues
	}

	return baseSchema
}
