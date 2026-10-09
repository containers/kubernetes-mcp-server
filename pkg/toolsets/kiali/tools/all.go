package tools

import (
	"context"
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// All returns every Kiali tool. Reachability filtering is handled by the toolset.
func All(ctx context.Context, inspector api.ClusterInspector) []api.ServerTool {
	return slices.Concat(
		InitGetMeshTrafficGraph(ctx, inspector),
		InitGetMeshStatus(),
		InitManageIstioConfigRead(),
		InitManageIstioConfig(),
		InitListMeshClusters(),
		InitListOrGetResources(ctx, inspector),
		InitListTraces(),
		InitGetTraceDetails(),
		InitGetPodPerformance(),
		InitGetLogs(),
		InitGetMetrics(),
	)
}
