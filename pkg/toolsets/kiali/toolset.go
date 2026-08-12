package kiali

import (
	"slices"

	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
	kialiPrompts "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/prompts"
	kialiTools "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return defaults.ToolsetName()
}

func (t *Toolset) GetDescription() string {
	return defaults.ToolsetDescription()
}

func (t *Toolset) GetTools(p api.FilteringProvider) []api.ServerTool {
	// Evaluate reachability once and share the filter across all Kiali tools.
	hasKiali := kialiclient.HasKiali(p)
	tools := slices.Concat(
		kialiTools.InitGetMeshTrafficGraph(p),
		kialiTools.InitGetMeshStatus(p),
		kialiTools.InitManageIstioConfigRead(p),
		kialiTools.InitManageIstioConfig(p),
		kialiTools.InitListMeshClusters(p),
		kialiTools.InitListOrGetResources(p),
		kialiTools.InitListTraces(p),
		kialiTools.InitGetTraceDetails(p),
		kialiTools.InitGetPodPerformance(p),
		kialiTools.InitGetLogs(p),
		kialiTools.InitGetMetrics(p),
	)
	// Kiali calls a single configured endpoint; mesh scope is selected via meshCluster,
	// not the provider-level context parameter injected for core Kubernetes tools.
	for i := range tools {
		tools[i].ClusterAware = ptr.To(false)
		tools[i].TargetCompatibilityFilters = []func() bool{hasKiali}
	}
	return tools
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	prompts := slices.Concat(
		kialiPrompts.InitListApplications(),
		kialiPrompts.InitListIstioConfig(),
		kialiPrompts.InitListNamespaces(),
		kialiPrompts.InitListServices(),
		kialiPrompts.InitListWorkloads(),
		kialiPrompts.InitMeshHealthCheck(),
		kialiPrompts.InitMeshTopology(),
		kialiPrompts.InitTrafficTopology(),
		kialiPrompts.InitServiceTroubleshoot(),
		kialiPrompts.InitTraceAnalysis(),
		kialiPrompts.InitIstioConfigReview(),
	)
	// Same as tools: mesh scope is not selected via provider context.
	for i := range prompts {
		prompts[i].ClusterAware = ptr.To(false)
	}
	return prompts
}

func (t *Toolset) GetResources() []api.ServerResource {
	return nil
}

func (t *Toolset) GetResourceTemplates() []api.ServerResourceTemplate {
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
