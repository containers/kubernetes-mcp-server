package kiali

import (
	"slices"

	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
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

func (t *Toolset) GetTools(_ api.FilteringProvider) []api.ServerTool {
	tools := slices.Concat(
		kialiTools.InitGetMeshTrafficGraph(),
		kialiTools.InitGetMeshStatus(),
		kialiTools.InitManageIstioConfigRead(),
		kialiTools.InitManageIstioConfig(),
		kialiTools.InitListMeshClusters(),
		kialiTools.InitListOrGetResources(),
		kialiTools.InitListTraces(),
		kialiTools.InitGetTraceDetails(),
		kialiTools.InitGetPodPerformance(),
		kialiTools.InitGetLogs(),
		kialiTools.InitGetMetrics(),
	)
	// Kiali calls a single configured endpoint; mesh scope is selected via meshCluster,
	// not the provider-level context parameter injected for core Kubernetes tools.
	for i := range tools {
		tools[i].ClusterAware = ptr.To(false)
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
