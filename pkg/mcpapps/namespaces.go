// Package mcpapps provides embedded MCP Apps UI resources for server tools.
package mcpapps

import "github.com/containers/kubernetes-mcp-server/pkg/api"

const namespacesListURI = "ui://kubernetes-mcp-server/namespaces-list"

// NamespacesList returns the self-contained application associated with
// namespaces_list. It intentionally has no network dependencies so it also
// works in disconnected clusters and under the default restrictive CSP.
func NamespacesList() *api.ToolApp {
	return Table(
		namespacesListURI,
		"Namespaces list",
		WithDescription("Interactive table of Kubernetes namespaces"),
		WithMetadata(map[string]any{"ui": map[string]any{"prefersBorder": true}}),
	)
}
