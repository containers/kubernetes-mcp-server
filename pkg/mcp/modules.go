package mcp

import (
	// Cluster providers that register themselves on import. The kcp provider
	// used to be pulled in by the (now removed) kcp toolset.
	_ "github.com/containers/kubernetes-mcp-server/pkg/kcp"

	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/dynamic"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/otelcol"
)
