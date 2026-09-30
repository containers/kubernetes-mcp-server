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

func (t *Toolset) GetTools(p api.FilteringProvider) []api.ServerTool {
	cfg := netobserv.EffectiveConfig{}
	if provider, ok := p.(netobserv.NetObservConfigProvider); ok {
		cfg = provider.NetObservConfig(context.Background())
	}
	if provider, ok := p.(interface {
		GetToolsetConfig(string) (config.ExtendedConfig, bool)
	}); ok {
		if configValue, ok := provider.GetToolsetConfig(defaults.ToolsetName()); ok {
			if netobservCfg, ok := configValue.(*netobserv.Config); ok {
				cfg = mergeManualConfig(cfg, netobservCfg)
			}
		}
	}
	flowsEnabled := !cfg.Found || cfg.Unknown || cfg.LokiEnabled
	metricsEnabled := !cfg.Found || cfg.Unknown || cfg.LokiEnabled || cfg.PrometheusEnabled
	var tools []api.ServerTool
	if flowsEnabled {
		tools = slices.Concat(tools, netobservTools.InitListFlows(p, cfg), netobservTools.InitExportFlows(p, cfg))
	}
	if metricsEnabled {
		tools = slices.Concat(tools, netobservTools.InitGetFlowMetrics(p, cfg))
	}
	return tools
}

func mergeManualConfig(cfg netobserv.EffectiveConfig, manual *netobserv.Config) netobserv.EffectiveConfig {
	if manual == nil {
		return cfg
	}
	if manual.LokiEnabled != nil {
		cfg.Found = true
		cfg.LokiEnabled = *manual.LokiEnabled
	}
	if manual.LokiMode != "" {
		cfg.Found = true
		cfg.LokiMode = manual.LokiMode
		if manual.LokiEnabled == nil {
			cfg.LokiEnabled = true
		}
	}
	if manual.PrometheusEnabled != nil {
		cfg.Found = true
		cfg.PrometheusEnabled = *manual.PrometheusEnabled
	}
	if manual.PrometheusMode != "" {
		cfg.Found = true
		cfg.PrometheusMode = manual.PrometheusMode
		if manual.PrometheusEnabled == nil {
			cfg.PrometheusEnabled = true
		}
	}
	return cfg
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	return nil
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
