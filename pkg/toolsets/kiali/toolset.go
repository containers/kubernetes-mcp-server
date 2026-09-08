package kiali

import (
	"context"
	"slices"
	"sync"

	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
	kialiPrompts "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/prompts"
	kialiTools "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

var warnKialiValidationDisabledOnce sync.Once

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return defaults.ToolsetName()
}

func (t *Toolset) GetDescription() string {
	return defaults.ToolsetDescription()
}

func (t *Toolset) GetTools(p api.FilteringProvider) []api.ServerTool {
	if p != nil && !p.IsTargetCompatibilityToolFiltersEnabled() {
		warnKialiValidationDisabledOnce.Do(func() {
			klogutil.LogWarn(
				klogutil.FromContext(context.Background()),
				"experimental_enable_target_compatibility_tool_filters is disabled; Kiali URL reachability and in-cluster discovery are not validated",
			)
		})
	} else if p != nil && p.IsTargetCompatibilityToolFiltersEnabled() && !kialiAvailable(p) {
		return nil
	}

	tools := kialiTools.All()
	for i := range tools {
		tools[i].ClusterAware = ptr.To(false)
	}
	return tools
}

func kialiAvailable(p api.FilteringProvider) bool {
	var kp kubernetes.Provider
	if provider, ok := p.(kubernetes.Provider); ok {
		kp = provider
	}
	return kialiclient.HasKiali(context.Background(), p, kp)
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
