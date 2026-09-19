package kiali

import (
	"context"
	"slices"
	"sync"

	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
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
			ctx, _, _ := registrationFromProvider(p)
			klogutil.LogWarn(
				klogutil.FromContext(ctx),
				"experimental_enable_target_compatibility_tool_filters is disabled; Kiali URL reachability is not validated",
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

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	return t.prompts()
}

// GetPromptsIfAvailable applies the same reachability gate as GetTools when the
// experimental target-compatibility flag is enabled. Called from the MCP server
// with the filtering provider wrapper that carries live config and credentials.
func (t *Toolset) GetPromptsIfAvailable(p api.FilteringProvider) []api.ServerPrompt {
	if p != nil && p.IsTargetCompatibilityToolFiltersEnabled() && !kialiAvailable(p) {
		return nil
	}
	return t.prompts()
}

func (t *Toolset) prompts() []api.ServerPrompt {
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
	for i := range prompts {
		prompts[i].ClusterAware = ptr.To(false)
	}
	return prompts
}

func kialiAvailable(p api.FilteringProvider) bool {
	ctx, baseCfg, token := registrationFromProvider(p)
	return kialiclient.HasKiali(ctx, baseCfg, token)
}

// registrationSource is satisfied by mcp.filteringProviderWithConfig today.
// TODO(registration-api): replace with a first-class registration context type.
type registrationSource interface {
	BaseConfig() api.BaseConfig
	BearerToken() string
	Context() context.Context
}

func registrationFromProvider(p api.FilteringProvider) (context.Context, api.BaseConfig, string) {
	if src, ok := p.(registrationSource); ok {
		return src.Context(), src.BaseConfig(), src.BearerToken()
	}
	return context.Background(), nil, ""
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
