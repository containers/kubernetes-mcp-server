package mcp

import (
	"fmt"
	"sort"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
)

type ToolMutator func(tool api.ServerTool) api.ServerTool

const maxTargetsInEnum = 5 // TODO: test and validate that this is a reasonable cutoff

// WithTargetParameter adds a target selection parameter to the tool's input schema if the tool is cluster-aware
func WithTargetParameter(defaultCluster, targetParameterName string, targets []string) ToolMutator {
	return func(tool api.ServerTool) api.ServerTool {
		if !tool.IsClusterAware() {
			return tool
		}

		if tool.Tool.InputSchema == nil {
			tool.Tool.InputSchema = &jsonschema.Schema{Type: "object"}
		}

		if tool.Tool.InputSchema.Properties == nil {
			tool.Tool.InputSchema.Properties = make(map[string]*jsonschema.Schema)
		}

		if len(targets) > 1 {
			tool.Tool.InputSchema.Properties[targetParameterName] = createTargetProperty(
				defaultCluster,
				targetParameterName,
				targets,
			)
		}

		return tool
	}
}

func createTargetProperty(defaultCluster, targetName string, targets []string) *jsonschema.Schema {
	baseSchema := &jsonschema.Schema{
		Type: "string",
		Description: fmt.Sprintf(
			"Optional parameter selecting which %s to run the tool in. Defaults to %s if not set",
			targetName,
			defaultCluster,
		),
	}

	if len(targets) <= maxTargetsInEnum {
		// Sort clusters to ensure consistent enum ordering
		sort.Strings(targets)

		enumValues := make([]any, 0, len(targets))
		for _, c := range targets {
			enumValues = append(enumValues, c)
		}
		baseSchema.Enum = enumValues
	}

	return baseSchema
}
