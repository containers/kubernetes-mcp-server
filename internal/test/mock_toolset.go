package test

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// MockToolset is a test helper for testing toolset functionality
type MockToolset struct {
	Name         string
	Description  string
	Instructions string
	Tools        []api.ServerTool
	Prompts      []api.ServerPrompt
}

var _ api.Toolset = (*MockToolset)(nil)

func (m *MockToolset) GetName() string {
	return m.Name
}

func (m *MockToolset) GetDescription() string {
	return m.Description
}

func (m *MockToolset) GetToolsetInstructions() string {
	return m.Instructions
}

func (m *MockToolset) GetTools(_ api.Openshift) []api.ServerTool {
	if m.Tools == nil {
		return []api.ServerTool{}
	}
	return m.Tools
}

func (m *MockToolset) GetPrompts() []api.ServerPrompt {
	return m.Prompts
}

// RegisterMockToolset registers a mock toolset for testing
func RegisterMockToolset(mockToolset *MockToolset) {
	toolsets.Register(mockToolset)
}

// UnregisterMockToolset removes a mock toolset from the registry
func UnregisterMockToolset(name string) {
	// Get all toolsets and rebuild the list without the mock
	allToolsets := toolsets.Toolsets()
	toolsets.Clear()
	for _, ts := range allToolsets {
		if ts.GetName() != name {
			toolsets.Register(ts)
		}
	}
}
