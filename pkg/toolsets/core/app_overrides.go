package core

import "github.com/containers/kubernetes-mcp-server/pkg/api"

// withAppOverrides is the customization point for core tool app presentations.
// The default implementation returns the tools unchanged. Keep presentation
// overrides in this file so toolset assembly does not need to change.
func withAppOverrides(tools []api.ServerTool) []api.ServerTool {
	return tools
}
