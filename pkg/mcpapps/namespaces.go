// Package mcpapps provides embedded MCP Apps UI resources for server tools.
package mcpapps

import (
	_ "embed"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

const namespacesListURI = "ui://kubernetes-mcp-server/namespaces-list"

// NamespacesList returns the self-contained application associated with
// namespaces_list. It intentionally has no network dependencies so it also
// works in disconnected clusters and under the default restrictive CSP.
func NamespacesList() *api.ToolApp {
	return Custom(
		namespacesListURI,
		"Namespaces list",
		StaticHTML(namespacesListHTML),
		WithDescription("Interactive table of Kubernetes namespaces"),
		WithMetadata(map[string]any{"ui": map[string]any{"prefersBorder": true}}),
	)
}

//go:embed namespaces.html
var namespacesListHTML string
