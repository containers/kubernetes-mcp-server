package tools

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// All returns every Kiali tool. Reachability filtering is handled by the toolset.
func All(p api.FilteringProvider) []api.ServerTool {
	return slices.Concat(
		InitGetMeshTrafficGraph(p),
		InitGetMeshStatus(),
		InitManageIstioConfigRead(),
		InitManageIstioConfig(),
		InitListMeshClusters(),
		InitListOrGetResources(p),
		InitListTraces(),
		InitGetTraceDetails(),
		InitGetPodPerformance(),
		InitGetLogs(),
		InitGetMetrics(),
	)
}
