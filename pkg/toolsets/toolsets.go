package toolsets

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

var toolsets []api.Toolset

// Clear removes all registered toolsets, TESTING PURPOSES ONLY.
func Clear() {
	toolsets = []api.Toolset{}
}

func Register(toolset api.Toolset) {
	toolsets = append(toolsets, toolset)
}

func Toolsets() []api.Toolset {
	return toolsets
}

func ToolsetNames() []string {
	names := make([]string, 0)
	for _, toolset := range Toolsets() {
		names = append(names, toolset.GetName())
	}
	slices.Sort(names)
	return names
}

func ToolsetFromString(name string) api.Toolset {
	for _, toolset := range Toolsets() {
		if toolset.GetName() == name {
			return toolset
		}
	}
	return nil
}
