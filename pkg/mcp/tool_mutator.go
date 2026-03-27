package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
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

// TargetsListToolName is the base name for the generic targets list tool before mutation
const TargetsListToolName = "targets_list"

// WithTargetParameter adds a target selection parameter to the tool's input schema if the tool is cluster-aware
func WithTargetParameter(defaultCluster, targetParameterName string, isMultiTarget bool) ToolMutator {
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

		if isMultiTarget {
			tool.Tool.InputSchema.Properties[targetParameterName] = createTargetProperty(
				defaultCluster,
				targetParameterName,
			)
		}

		return tool
	}
}

func createTargetProperty(defaultCluster, targetName string) *jsonschema.Schema {
	baseSchema := &jsonschema.Schema{
		Type: "string",
		Description: fmt.Sprintf(
			"Optional parameter selecting which %s to run the tool in. Defaults to %s if not set",
			targetName,
			defaultCluster,
		),
	}

	return baseSchema
}

// targetLister is a minimal interface for listing available targets.
// This reduces coupling with the kubernetes package.
type targetLister interface {
	GetTargets(ctx context.Context) ([]string, error)
}

// WithTargetListTool mutates the generic "targets_list" tool to have the correct name,
// description, and handler based on the provider's target parameter name.
// For example, with ACM provider (targetParameterName="cluster"), it becomes "cluster_list".
func WithTargetListTool(defaultTarget, targetParameterName string, p targetLister) ToolMutator {
	return func(tool api.ServerTool) api.ServerTool {
		if tool.Tool.Name != TargetsListToolName {
			return tool
		}

		// Rename tool based on target parameter name
		tool.Tool.Name = fmt.Sprintf("%s_list", targetParameterName)
		tool.Tool.Description = fmt.Sprintf("List all available %ss that can be targeted by tools", targetParameterName)
		tool.Tool.Annotations.Title = fmt.Sprintf("%s List", capitalizeFirst(targetParameterName))

		// Set the handler with captured targets
		tool.Handler = createTargetListHandler(p, targetParameterName, defaultTarget)

		return tool
	}
}

func createTargetListHandler(p targetLister, targetParameterName, defaultTarget string) api.ToolHandlerFunc {
	return func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		targets, err := p.GetTargets(params.Context)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to find any targets: %w", err)), nil
		}

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

// WithToolOverrides returns a mutator that applies per-tool configuration overrides
// (such as custom descriptions) from the user's config file.
func WithToolOverrides(overrides map[string]config.ToolOverride) ToolMutator {
	return func(tool api.ServerTool) api.ServerTool {
		if overrides == nil {
			return tool
		}
		if o, ok := overrides[tool.Tool.Name]; ok {
			if o.Description != "" {
				tool.Tool.Description = o.Description
			}
		}
		return tool
	}
}
