package toolsets

import (
	"fmt"
	"slices"
	"strings"

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
		if toolset.GetName() == strings.TrimSpace(name) {
			return toolset
		}
	}
	return nil
}

func Validate(toolsets []string) error {
	for _, toolset := range toolsets {
		if ToolsetFromString(toolset) == nil {
			return fmt.Errorf("invalid toolset name: %s, valid names are: %s", toolset, strings.Join(ToolsetNames(), ", "))
		}
	}
	return nil
}
