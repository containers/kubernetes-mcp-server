package mcpapps

import "github.com/containers/kubernetes-mcp-server/pkg/api"

// Table creates a self-contained, sortable table application. Tool results
// must provide structured content with an items array of flat row objects or
// complete Kubernetes resources (flattened only for display). An optional
// columns array controls column order; otherwise common resource columns come
// first, followed by any remaining row keys.
func Table(uri, name string, options ...CustomOption) *api.ToolApp {
	return standardApp(uri, name, "table", options...)
}
