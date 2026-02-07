package fusion

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/fusion/storage"
)

// Toolset implements the IBM Fusion toolset
type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

// GetName returns the name of the IBM Fusion toolset
func (t *Toolset) GetName() string {
	return "fusion"
}

// GetDescription returns a description of the IBM Fusion toolset
func (t *Toolset) GetDescription() string {
	return "IBM Fusion capabilities for OpenShift (storage, network, compute management)"
}

// GetTools returns all tools provided by the IBM Fusion toolset
func (t *Toolset) GetTools(o api.Openshift) []api.ServerTool {
	return []api.ServerTool{
		storage.InitStorageSummary(),
	}
}

// GetPrompts returns prompts provided by the IBM Fusion toolset
func (t *Toolset) GetPrompts() []api.ServerPrompt {
	// No prompts for now
	return nil
}

// Made with Bob
