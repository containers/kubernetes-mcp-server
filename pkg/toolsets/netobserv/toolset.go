package netobserv

import (
	"context"
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
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
	cfg, err := netobserv.DetectConfig(ctx, tc.Inspector)
	if err != nil {
		cfg.Unknown = true
	}
	if tc.Config != nil {
		if configValue, ok := tc.Config.GetToolsetConfig(defaults.ToolsetName()); ok {
			if netobservCfg, ok := configValue.(*netobserv.Config); ok {
				cfg = mergeManualConfig(cfg, netobservCfg)
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
