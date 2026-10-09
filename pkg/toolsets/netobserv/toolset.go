package netobserv

import (
	"context"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

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

func (t *Toolset) GetTools(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerTool {
	explicitURL := false
	if cfg := toolsetContext.Config; cfg != nil {
		if extended, ok := cfg.GetToolsetConfig("netobserv"); ok {
			if netobservConfig, ok := extended.(*netobservclient.Config); ok && netobservConfig != nil {
				explicitURL = strings.TrimSpace(netobservConfig.Url) != ""
			}
		}
	}
	compatible := func() bool {
		if explicitURL || toolsetContext.Inspector == nil {
			return true
		}
		return api.AnyTargetHasGVK(ctx, toolsetContext.Inspector, schema.GroupVersionKind{
			Group: "flows.netobserv.io", Kind: "FlowCollector",
		})
	}
	tools := slices.Concat(
		netobservTools.InitListFlows(),
		netobservTools.InitGetFlowMetrics(),
		netobservTools.InitExportFlows(),
	)
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
