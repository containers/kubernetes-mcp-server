package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// PollInterval is how often the toolset checks the cluster for changed Tool
// resources. Exported so tests can shorten it.
var PollInterval = 15 * time.Second

// Toolset exposes Tool custom resources as MCP tools.
type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)
var _ api.ToolsetWatcher = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "dynamic"
}

func (t *Toolset) GetDescription() string {
	return "Tools defined declaratively as Tool custom resources (" + Group + ") installed in the cluster, see docs/specs/dynamic-tools.md"
}

func (t *Toolset) GetTools(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerTool {
	logger := klogutil.FromContext(ctx)
	definitions, err := listTools(ctx, toolsetContext.Inspector)
	if err != nil {
		logger.V(1).Info("dynamic toolset: unable to list Tool resources, exposing no dynamic tools", "error", err.Error())
		return nil
	}
	var discovery api.AggregateDiscovery
	if toolsetContext.Inspector != nil {
		discovery = toolsetContext.Inspector.Discovery()
	}
	tools := make([]api.ServerTool, 0, len(definitions))
	for _, definition := range definitions {
		// One broken definition must not take the others down: skip it and say why.
		compiled, err := newCompiledTool(definition)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Tool resource", "tool", definition.Metadata.Name, "error", err.Error())
			continue
		}
		serverTool, err := compiled.serverTool(ctx, discovery)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Tool resource", "tool", definition.Metadata.Name, "error", err.Error())
			continue
		}
		tools = append(tools, serverTool)
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

// WatchToolset polls the Tool resources and asks the server to re-apply the
// toolsets whenever their set or resourceVersions change. Polling mirrors the
// cluster-state watcher; an informer would be the production choice.
func (t *Toolset) WatchToolset(ctx context.Context, toolsetContext api.ToolsetContext, onChange func() error) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	logger := klogutil.FromContext(ctx)
	last := fingerprint(ctx, toolsetContext.Inspector)
	go func() {
		ticker := time.NewTicker(PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			current := fingerprint(ctx, toolsetContext.Inspector)
			if current == last {
				continue
			}
			last = current
			logger.V(1).Info("dynamic toolset: Tool resources changed, re-applying toolsets")
			if err := onChange(); err != nil {
				logger.Error(err, "dynamic toolset: failed to re-apply toolsets")
			}
		}
	}()
	return cancel
}

// listTools returns the Tool resources of the default target, sorted by name.
// A missing CRD is not an error: it simply means no dynamic tools.
func listTools(ctx context.Context, inspector api.ClusterInspector) ([]Tool, error) {
	if inspector == nil || inspector.Unstructured() == nil {
		return nil, errors.New("no cluster inspector available")
	}
	list, err := inspector.Unstructured().Resource(GVR).List(ctx, metav1.ListOptions{}).Default()
	if err != nil {
		if api.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	tools := make([]Tool, 0, len(list.Items))
	for _, item := range list.Items {
		raw, err := json.Marshal(item.Object)
		if err != nil {
			return nil, err
		}
		var tool Tool
		if err := json.Unmarshal(raw, &tool); err != nil {
			return nil, fmt.Errorf("tool %s: %w", item.GetName(), err)
		}
		tools = append(tools, tool)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Metadata.Name < tools[j].Metadata.Name })
	return tools, nil
}

func fingerprint(ctx context.Context, inspector api.ClusterInspector) string {
	tools, err := listTools(ctx, inspector)
	if err != nil {
		return ""
	}
	parts := make([]string, 0, len(tools))
	for _, tool := range tools {
		parts = append(parts, tool.Metadata.Name+"="+tool.Metadata.ResourceVersion)
	}
	return strings.Join(parts, "\n")
}

func init() {
	toolsets.Register(&Toolset{})
}
