package mcp

import (
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/dynamic"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kcp"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/logs"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/metrics"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/otelcol"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/traces"
)
