package tools

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// All returns every Kiali tool. Reachability filtering is handled by the toolset.
func All() []api.ServerTool {
	return slices.Concat(
		InitGetMeshTrafficGraph(),
		InitGetMeshStatus(),
		InitManageIstioConfigRead(),
		InitManageIstioConfig(),
		InitListMeshClusters(),
		InitListOrGetResources(),
		InitListTraces(),
		InitGetTraceDetails(),
		InitGetPodPerformance(),
		InitGetLogs(),
		InitGetMetrics(),
	)
}
