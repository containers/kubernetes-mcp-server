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

func init() {
	// IBM Fusion extension integration point
	// This is the single hook for registering IBM Fusion tools
	// Tools are only registered if FUSION_TOOLS_ENABLED=true
	registerFusionTools()
}

// registerFusionTools is a placeholder that will be implemented by the fusion package
// This allows the fusion package to register itself without modifying upstream code
var registerFusionTools = func() {}

// SetFusionRegistration allows the fusion package to hook into the registration process
// This is the single integration point for IBM Fusion tools
func SetFusionRegistration(fn func()) {
	registerFusionTools = fn
}
}
