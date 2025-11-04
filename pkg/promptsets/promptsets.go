package promptsets

import (
	"slices"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

var promptsets []api.PromptSet

// Clear removes all registered promptsets, TESTING PURPOSES ONLY.
func Clear() {
	promptsets = []api.PromptSet{}
}

// Register adds a promptset to the registry
func Register(promptset api.PromptSet) {
	promptsets = append(promptsets, promptset)
}

// PromptSets returns all registered promptsets
func PromptSets() []api.PromptSet {
	return promptsets
}

// PromptSetFromString returns a PromptSet by name, or nil if not found
func PromptSetFromString(name string) api.PromptSet {
	for _, ps := range PromptSets() {
		if ps.GetName() == strings.TrimSpace(name) {
			return ps
		}
	}
	return nil
}

// AllPromptSets returns all available promptsets
func AllPromptSets() []api.PromptSet {
	return PromptSets()
}

// GetPromptSetNames returns names of all registered promptsets
func GetPromptSetNames() []string {
	names := make([]string, 0, len(promptsets))
	for _, ps := range promptsets {
		names = append(names, ps.GetName())
	}
	slices.Sort(names)
	return names
}
