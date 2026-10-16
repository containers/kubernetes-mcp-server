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
	var unknown []string
	enabled := map[string]bool{}
	for _, toolset := range toolsets {
		if ToolsetFromString(toolset) == nil {
			unknown = append(unknown, toolset)
		}
		enabled[strings.TrimSpace(toolset)] = true
	}
	if len(unknown) > 0 {
		return fmt.Errorf("invalid toolset name: %s, valid names are: %s (or <name>/<family> for a toolset with families)", strings.Join(unknown, ", "), strings.Join(ToolsetNames(), ", "))
	}
	// A toolset and one of its families would expose the family twice.
	for name := range enabled {
		if owner, _, ok := strings.Cut(name, "/"); ok && enabled[owner] {
			return fmt.Errorf("toolset %s already includes %s: enable one or the other", owner, name)
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
	if toolset, ok := r.toolsets[name]; ok {
		return toolset
	}
	// "<toolset>/<family>" resolves to a part of a toolset that supports it.
	if owner, _, ok := strings.Cut(name, "/"); ok {
		if families, ok := r.toolsets[owner].(api.ToolsetFamilies); ok {
			return families.Family(name)
		}
	}
	return nil
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
