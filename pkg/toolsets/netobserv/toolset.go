package netobserv

import (
	"context"
	"slices"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
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

func (t *Toolset) GetTools(_ context.Context, tc api.ToolsetContext) []api.ServerTool {
	listFlows := netobservTools.InitListFlows()
	getMetrics := netobservTools.InitGetFlowMetrics()
	exportFlows := netobservTools.InitExportFlows()
	manual := manualNetObservConfig(tc)
	if manual == nil || !hasBackendSettings(manual) {
		return slices.Concat(listFlows, getMetrics, exportFlows)
	}

	lokiEnabled := enabledSetting(manual.LokiEnabled, manual.LokiMode)
	prometheusEnabled := enabledSetting(manual.PrometheusEnabled, manual.PrometheusMode)
	var tools []api.ServerTool
	if lokiEnabled {
		setRBAC(listFlows, lokiFlowRBAC(manual))
		tools = append(tools, listFlows...)
	}
	if lokiEnabled || prometheusEnabled {
		setRBAC(getMetrics, prometheusMetricsRBAC(getMetrics[0].RBAC, manual))
		tools = append(tools, getMetrics...)
	}
	if lokiEnabled {
		setRBAC(exportFlows, lokiFlowRBAC(manual))
		tools = append(tools, exportFlows...)
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

func manualNetObservConfig(tc api.ToolsetContext) *netobservclient.Config {
	if tc.Config == nil {
		return nil
	}
	raw, ok := tc.Config.GetToolsetConfig(netobservclient.ToolsetConfigKey)
	if !ok || raw == nil {
		return nil
	}
	cfg, ok := raw.(*netobservclient.Config)
	if !ok {
		return nil
	}
	return cfg
}

func hasBackendSettings(cfg *netobservclient.Config) bool {
	return cfg.LokiEnabled != nil || cfg.LokiMode != "" || cfg.PrometheusEnabled != nil || cfg.PrometheusMode != ""
}

func enabledSetting(enabled *bool, mode string) bool {
	if enabled != nil {
		return *enabled
	}
	return mode != ""
}

func setRBAC(tools []api.ServerTool, rbac *api.RBACMetadata) {
	for i := range tools {
		tools[i].RBAC = rbac
	}
}

func lokiFlowRBAC(cfg *netobservclient.Config) *api.RBACMetadata {
	if !enabledSetting(cfg.LokiEnabled, cfg.LokiMode) || !strings.EqualFold(cfg.LokiMode, "LokiStack") {
		return nil
	}
	return api.RBACBounded(api.RBACRequirement{
		Verbs: []string{"get"},
		Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
			APIGroup: "loki.grafana.com",
			Resource: "network",
		}},
		ResourceName: &api.RBACResourceName{Name: "logs"},
	})
}

func prometheusMetricsRBAC(existing *api.RBACMetadata, cfg *netobservclient.Config) *api.RBACMetadata {
	if cfg.PrometheusEnabled == nil && cfg.PrometheusMode == "" {
		return existing
	}
	if enabledSetting(cfg.PrometheusEnabled, cfg.PrometheusMode) && strings.EqualFold(cfg.PrometheusMode, "Auto") {
		return existing
	}
	return nil
}
