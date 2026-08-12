package tools

import (
	"slices"

	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
)

// All returns every Kiali tool with a shared reachability filter.
func All(p api.FilteringProvider) []api.ServerTool {
	hasKiali := kialiclient.HasKiali(p)
	tools := slices.Concat(
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
	// Kiali calls a single configured endpoint; mesh scope is selected via meshCluster,
	// not the provider-level context parameter injected for core Kubernetes tools.
	for i := range tools {
		tools[i].ClusterAware = ptr.To(false)
		tools[i].TargetCompatibilityFilters = []func() bool{hasKiali}
	}
	return tools
}
