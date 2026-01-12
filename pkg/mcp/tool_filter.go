package mcp

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

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

func ShouldIncludeTargetListTool(targetName string, targets []string) ToolFilter {
	return func(tool api.ServerTool) bool {
		if !tool.IsTargetListProvider() {
			return true
		}
		if len(targets) <= 1 {
			// there is no need to provide a tool to list the single available target
			return false
		}

		// Mutual exclusivity between configuration_contexts_list and targets_list:
		// - configuration_contexts_list: only for kubeconfig provider (targetName == "context")
		// - targets_list: only for non-kubeconfig providers (targetName != "context")
		// Note: targets_list gets mutated to "{targetName}_list" before this filter runs,
		// so we check for the mutated name pattern
		if tool.Tool.Name == "configuration_contexts_list" && targetName != kubernetes.KubeConfigTargetParameterName {
			return false
		}
		mutatedTargetsListName := targetName + "_list"
		if tool.Tool.Name == mutatedTargetsListName && targetName == kubernetes.KubeConfigTargetParameterName {
			return false
		}

		return true
	}
}
