package mcpapps

import "github.com/containers/kubernetes-mcp-server/pkg/api"

// Resource creates a self-contained details view for one Kubernetes resource,
// with metadata, conditions, Pod containers, and expandable specification,
// status, and raw resource sections.
func Resource(uri, name string, options ...CustomOption) *api.ToolApp {
	return standardApp(uri, name, "resource", options...)
}
