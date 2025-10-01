package mcp

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

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
		if tool.Tool.Name == "contexts_list" {
			if targetName != kubernetes.KubeConfigTargetParameterName {
				// let's not include contexts_list if we aren't targetting contexts in our ManagerProvider
				return false
			}

			if len(targets) <= maxTargetsInEnum {
				// all targets in enum, no need for contexts_list tool
				return false
			}
		}

		return true
	}
}
