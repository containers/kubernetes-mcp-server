package mcp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
)

type ToolMutator func(tool api.ServerTool) api.ServerTool

// ComposeMutators combines multiple mutators into a single mutator that applies them in order
func ComposeMutators(mutators ...ToolMutator) ToolMutator {
	return func(tool api.ServerTool) api.ServerTool {
		for _, m := range mutators {
			tool = m(tool)
		}
		return tool
	}
}

const maxTargetsInEnum = 5 // TODO: test and validate that this is a reasonable cutoff

// TargetsListToolName is the base name for the generic targets list tool before mutation
const TargetsListToolName = "targets_list"

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

// WithTargetListTool mutates the generic "targets_list" tool to have the correct name,
// description, and handler based on the provider's target parameter name.
// For example, with ACM provider (targetParameterName="cluster"), it becomes "cluster_list".
func WithTargetListTool(defaultTarget, targetParameterName string, targets []string) ToolMutator {
	return func(tool api.ServerTool) api.ServerTool {
		if tool.Tool.Name != TargetsListToolName {
			return tool
		}

		// Rename tool based on target parameter name
		tool.Tool.Name = fmt.Sprintf("%s_list", targetParameterName)
		tool.Tool.Description = fmt.Sprintf("List all available %ss that can be targeted by tools", targetParameterName)
		tool.Tool.Annotations.Title = fmt.Sprintf("%s List", capitalizeFirst(targetParameterName))

		// Set the handler with captured targets
		tool.Handler = createTargetListHandler(targets, targetParameterName, defaultTarget)

		return tool
	}
}

func createTargetListHandler(targets []string, targetParameterName, defaultTarget string) api.ToolHandlerFunc {
	return func(_ api.ToolHandlerParams) (*api.ToolCallResult, error) {
		if len(targets) == 0 {
			return api.NewToolCallResult(fmt.Sprintf("No %ss available", targetParameterName), nil), nil
		}

		// Sort targets for consistent output
		sortedTargets := make([]string, len(targets))
		copy(sortedTargets, targets)
		sort.Strings(sortedTargets)

		result := fmt.Sprintf("Available %ss (%d total, default: %s):\n\n", targetParameterName, len(sortedTargets), defaultTarget)
		result += fmt.Sprintf("Format: [*] %s_NAME\n", strings.ToUpper(targetParameterName))
		result += fmt.Sprintf(" (* indicates the default %s used in tools if %s is not set)\n\n", targetParameterName, targetParameterName)
		result += fmt.Sprintf("%ss:\n---------\n", capitalizeFirst(targetParameterName))
		for _, target := range sortedTargets {
			marker := " "
			if target == defaultTarget {
				marker = "*"
			}
			result += fmt.Sprintf("%s %s\n", marker, target)
		}
		result += "---------\n\n"
		result += fmt.Sprintf("To use a specific %s with any tool, set the '%s' parameter in the tool call arguments", targetParameterName, targetParameterName)

		return api.NewToolCallResult(result, nil), nil
	}
}

func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
