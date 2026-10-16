package mcp

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

const configurationViewToolName = "configuration_view"

// ToolFilter is a function that takes a ServerTool and returns a boolean indicating whether to include the tool
type ToolFilter func(tool api.ServerTool) bool

func CompositeFilter(filters ...ToolFilter) ToolFilter {
	return func(tool api.ServerTool) bool {
		for _, f := range filters {
			if !f(tool) {
				return false
			}
		}

		return true
	}
}

func ShouldIncludeTargetListTool(targetName string, isMultiTarget bool) ToolFilter {
	return func(tool api.ServerTool) bool {
		if !tool.IsTargetListProvider() {
			return true
		}
		if !isMultiTarget {
			// there is no need to provide a tool to list the single available target
			return false
		}

		return true
	}
}

// ShouldIncludeConfigurationViewTool prevents kubeconfig contents from being
// exposed over HTTP unless the tool was explicitly allowlisted.
func ShouldIncludeConfigurationViewTool(isHTTP bool, enabledTools []string) ToolFilter {
	return func(tool api.ServerTool) bool {
		return !isHTTP ||
			tool.Tool.Name != configurationViewToolName ||
			slices.Contains(enabledTools, tool.Tool.Name)
	}
}
