package toolsets

import (
	"fmt"
	"slices"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

var toolsetReg = &toolsetRegistry{toolsets: make(map[string]api.Toolset)}

// Clear removes all registered toolsets, TESTING PURPOSES ONLY.
func Clear() {
	toolsetReg.clear()
}

func Register(toolset api.Toolset) {
	toolsetReg.register(toolset)
}

func Toolsets() []api.Toolset {
	return toolsetReg.all()
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
	return toolsetReg.get(strings.TrimSpace(name))
}

func Validate(toolsets []string) error {
	for _, toolset := range toolsets {
		if ToolsetFromString(toolset) == nil {
			return fmt.Errorf("invalid toolset name: %s, valid names are: %s", toolset, strings.Join(ToolsetNames(), ", "))
		}
	}
	return nil
}

type toolsetRegistry struct {
	toolsets map[string]api.Toolset
}

func (r *toolsetRegistry) register(toolset api.Toolset) {
	name := toolset.GetName()
	if _, exists := r.toolsets[name]; exists {
		panic(fmt.Sprintf("toolset already registered for name '%s'", name))
	}
	r.toolsets[name] = toolset
}

func (r *toolsetRegistry) get(name string) api.Toolset {
	return r.toolsets[name]
}

func (r *toolsetRegistry) all() []api.Toolset {
	result := make([]api.Toolset, 0, len(r.toolsets))
	for _, toolset := range r.toolsets {
		result = append(result, toolset)
	}
	slices.SortFunc(result, func(a, b api.Toolset) int {
		return strings.Compare(a.GetName(), b.GetName())
	})
	return result
}

func (r *toolsetRegistry) clear() {
	r.toolsets = make(map[string]api.Toolset)
}
