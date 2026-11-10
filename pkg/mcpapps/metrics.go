package mcpapps

import "github.com/containers/kubernetes-mcp-server/pkg/api"

// Metrics creates a self-contained application for CPU and memory metrics.
// Tool results must provide structured content with an items array. CPU and
// Memory columns accept Kubernetes quantities and are sorted numerically.
func Metrics(uri, name string, options ...CustomOption) *api.ToolApp {
	return standardApp(uri, name, "metrics", options...)
}
