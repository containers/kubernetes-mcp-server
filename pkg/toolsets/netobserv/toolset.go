package netobserv

import (
	"context"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobserv "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/internal/defaults"
	netobservTools "github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/tools"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return defaults.ToolsetName()
}

func (t *Toolset) GetDescription() string {
	return defaults.ToolsetDescription()
}

func (t *Toolset) GetTools(ctx context.Context, tc api.ToolsetContext) []api.ServerTool {
	explicitURL := false
	cfg, err := netobserv.DetectConfig(ctx, tc.Inspector)
	if err != nil {
		cfg.Unknown = true
	}
	if tc.Config != nil {
		if configValue, ok := tc.Config.GetToolsetConfig(netobserv.ToolsetConfigKey); ok {
			if netobservCfg, ok := configValue.(*netobserv.Config); ok && netobservCfg != nil {
				explicitURL = strings.TrimSpace(netobservCfg.Url) != ""
				cfg = netobserv.MergeEffectiveConfig(cfg, netobservCfg)
			}
		}
	}
	flowsEnabled := !cfg.Found || cfg.Unknown || cfg.LokiEnabled
	metricsEnabled := !cfg.Found || cfg.Unknown || cfg.LokiEnabled || cfg.PrometheusEnabled
	var tools []api.ServerTool
	if flowsEnabled {
		tools = slices.Concat(tools, netobservTools.InitListFlows(tc.Inspector, cfg), netobservTools.InitExportFlows(tc.Inspector, cfg))
	}
	if metricsEnabled {
		tools = slices.Concat(tools, netobservTools.InitGetFlowMetrics(tc.Inspector, cfg))
	}
	compatible := func(ctx context.Context) bool {
		if explicitURL || tc.Inspector == nil {
			return true
		}
		return api.AnyTargetHasGVK(ctx, tc.Inspector, schema.GroupVersionKind{
			Group: "flows.netobserv.io", Kind: "FlowCollector",
		})
	}
	for i := range tools {
		tools[i].TargetCompatibilityFilters = append(tools[i].TargetCompatibilityFilters, compatible)
	}
	return tools
}

func (t *Toolset) GetPrompts(_ context.Context, _ api.ToolsetContext) []api.ServerPrompt {
	return nil
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
