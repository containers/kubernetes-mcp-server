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

func (t *Toolset) GetTools(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerTool {
	if !toolsetContext.TargetCompatibilityFiltersEnabled {
		warnKialiValidationDisabledOnce.Do(func() {
			klogutil.LogWarn(
				klogutil.FromContext(ctx),
				"experimental_enable_target_compatibility_tool_filters is disabled; Kiali URL reachability is not validated",
			)
		})
	} else if !kialiAvailable(ctx, toolsetContext) {
		return nil
	}

	tools := kialiTools.All(ctx, toolsetContext.Inspector)
	// Kiali calls a single configured endpoint; mesh scope is selected via meshCluster,
	// not the provider-level context parameter injected for core Kubernetes tools.
	for i := range tools {
		tools[i].ClusterAware = ptr.To(false)
	}
	return tools
}

func kialiAvailable(ctx context.Context, toolsetContext api.ToolsetContext) bool {
	kp := kubernetes.ClusterProvider(toolsetContext.Inspector)
	return kialiclient.HasKiali(ctx, toolsetContext.Config, kp)
}

func (t *Toolset) GetPrompts(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerPrompt {
	if toolsetContext.TargetCompatibilityFiltersEnabled && !kialiAvailable(ctx, toolsetContext) {
		return nil
	}

	prompts := slices.Concat(
		kialiPrompts.InitListApplications(ctx, toolsetContext.Inspector),
		kialiPrompts.InitListIstioConfig(),
		kialiPrompts.InitListNamespaces(ctx, toolsetContext.Inspector),
		kialiPrompts.InitListServices(ctx, toolsetContext.Inspector),
		kialiPrompts.InitListWorkloads(ctx, toolsetContext.Inspector),
		kialiPrompts.InitMeshHealthCheck(),
		kialiPrompts.InitMeshTopology(),
		kialiPrompts.InitTrafficTopology(ctx, toolsetContext.Inspector),
		kialiPrompts.InitServiceTroubleshoot(),
		kialiPrompts.InitTraceAnalysis(),
		kialiPrompts.InitIstioConfigReview(ctx, toolsetContext.Inspector),
	)
	// Same as tools: mesh scope is not selected via provider context.
	for i := range prompts {
		prompts[i].ClusterAware = ptr.To(false)
	}
	return prompts
}

func (t *Toolset) GetResources(_ context.Context, _ api.ToolsetContext) []api.ServerResource {
	return nil
}

func (t *Toolset) GetResourceTemplates(_ context.Context, _ api.ToolsetContext) []api.ServerResourceTemplate {
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
