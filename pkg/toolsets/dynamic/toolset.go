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
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// PollInterval is how often the toolset checks the cluster for changed Tool
// and Prompt resources. Exported so tests can shorten it.
var PollInterval = 15 * time.Second

// Toolset exposes Tool and Prompt custom resources as MCP tools and prompts.
type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)
var _ api.ToolsetWatcher = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "dynamic"
}

func (t *Toolset) GetDescription() string {
	return "Tools and prompts defined declaratively as Tool and Prompt custom resources (" + Group + ") installed in the cluster, see docs/specs/dynamic-tools.md"
}

func (t *Toolset) GetTools(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerTool {
	logger := klogutil.FromContext(ctx)
	definitions, err := listResources[Tool](ctx, toolsetContext.Inspector, GVR)
	if err != nil {
		logger.V(1).Info("dynamic toolset: unable to list Tool resources, exposing no dynamic tools", "error", err.Error())
		return nil
	}
	tools := make([]api.ServerTool, 0, len(definitions))
	for _, definition := range definitions {
		// One broken definition must not take the others down: skip it and say why.
		compiled, err := newCompiledTool(definition)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Tool resource", "tool", definition.Metadata.Name, "error", err.Error())
			continue
		}
		serverTool, err := compiled.serverTool(ctx, toolsetContext.Inspector)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Tool resource", "tool", definition.Metadata.Name, "error", err.Error())
			continue
		}
		tools = append(tools, serverTool)
	}
	return tools
}

func (t *Toolset) GetPrompts(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerPrompt {
	logger := klogutil.FromContext(ctx)
	definitions, err := listResources[Prompt](ctx, toolsetContext.Inspector, PromptGVR)
	if err != nil {
		logger.V(1).Info("dynamic toolset: unable to list Prompt resources, exposing no dynamic prompts", "error", err.Error())
		return nil
	}
	var discovery api.AggregateDiscovery
	if toolsetContext.Inspector != nil {
		discovery = toolsetContext.Inspector.Discovery()
	}
	prompts := make([]api.ServerPrompt, 0, len(definitions))
	for _, definition := range definitions {
		compiled, err := newCompiledPrompt(definition)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Prompt resource", "prompt", definition.Metadata.Name, "error", err.Error())
			continue
		}
		prompts = append(prompts, compiled.serverPrompt(ctx, discovery))
	}
	return prompts
}

func (t *Toolset) GetResources(_ context.Context, _ api.ToolsetContext) []api.ServerResource {
	return nil
}

func (t *Toolset) GetResourceTemplates(_ context.Context, _ api.ToolsetContext) []api.ServerResourceTemplate {
	return nil
}

// WatchToolset polls the Tool and Prompt resources and asks the server to
// re-apply the toolsets whenever their set or resourceVersions change.
// Polling mirrors the cluster-state watcher; an informer would be the
// production choice.
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
			logger.V(1).Info("dynamic toolset: Tool or Prompt resources changed, re-applying toolsets")
			if err := onChange(); err != nil {
				logger.Error(err, "dynamic toolset: failed to re-apply toolsets")
			}
		}
	}()
	return cancel
}

// named is implemented by Tool and Prompt for listing and fingerprinting.
type named interface {
	Tool | Prompt
}

// listResources returns the resources of the default target, sorted by name.
// A missing CRD is not an error: it simply means no definitions.
func listResources[T named](ctx context.Context, inspector api.ClusterInspector, gvr schema.GroupVersionResource) ([]T, error) {
	if inspector == nil || inspector.Unstructured() == nil {
		return nil, errors.New("no cluster inspector available")
	}
	list, err := inspector.Unstructured().Resource(gvr).List(ctx, metav1.ListOptions{}).Default()
	if err != nil {
		if api.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	items := make([]T, 0, len(list.Items))
	for _, item := range list.Items {
		raw, err := json.Marshal(item.Object)
		if err != nil {
			return nil, err
		}
		var decoded T
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, fmt.Errorf("%s %s: %w", gvr.Resource, item.GetName(), err)
		}
		items = append(items, decoded)
	}
	sort.Slice(items, func(i, j int) bool { return metadataOf(items[i]).Name < metadataOf(items[j]).Name })
	return items, nil
}

func metadataOf[T named](item T) Metadata {
	switch v := any(item).(type) {
	case Tool:
		return v.Metadata
	case Prompt:
		return v.Metadata
	}
	return Metadata{}
}

func fingerprint(ctx context.Context, inspector api.ClusterInspector) string {
	var parts []string
	tools, err := listResources[Tool](ctx, inspector, GVR)
	if err != nil {
		return ""
	}
	for _, tool := range tools {
		parts = append(parts, "tool/"+tool.Metadata.Name+"="+tool.Metadata.ResourceVersion)
	}
	prompts, err := listResources[Prompt](ctx, inspector, PromptGVR)
	if err != nil {
		return ""
	}
	for _, prompt := range prompts {
		parts = append(parts, "prompt/"+prompt.Metadata.Name+"="+prompt.Metadata.ResourceVersion)
	}
	return strings.Join(parts, "\n")
}

func init() {
	toolsets.Register(&Toolset{})
}
