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

		// TODO: this check should be removed or make more generic when we have other
		if tool.Tool.Name == "configuration_contexts_list" && targetName != kubernetes.KubeConfigTargetParameterName {
			// let's not include configuration_contexts_list if we aren't targeting contexts in our ManagerProvider
			return false
		}

		return true
	}
}
