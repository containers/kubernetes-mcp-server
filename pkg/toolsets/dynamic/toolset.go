package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// pollInterval is how often the toolset checks the cluster for changed Tool
// and Prompt resources. Atomic so tests can shorten it while watchers from
// earlier servers may still be running.
var pollInterval atomic.Int64

// DefaultPollInterval is the poll interval unless SetPollInterval was called.
const DefaultPollInterval = 15 * time.Second

func init() {
	pollInterval.Store(int64(DefaultPollInterval))
}

// SetPollInterval changes how often Tool and Prompt resources are polled.
func SetPollInterval(d time.Duration) {
	pollInterval.Store(int64(d))
}

// Toolset exposes Tool and Prompt custom resources as MCP tools and prompts.
//
// The definitions of the last successful list are kept: a list failure while
// the toolsets are re-applied (a kubeconfig or cluster-state change, a
// configuration reload) must not deregister every dynamic tool until the
// resources next change, which the poller would not notice either.
type Toolset struct {
	tools   lastListed[Tool]
	prompts lastListed[Prompt]
}

// lastListed remembers the result of the last successful list of one kind.
type lastListed[T named] struct {
	mu     sync.Mutex
	items  []T
	listed bool
}

// listOrLast lists the resources of one kind, falling back to the last
// successful list when the cluster cannot be asked. The error is logged at
// default verbosity: the usual cause is a missing ClusterRole on the
// server's ServiceAccount, and the operator would otherwise only see
// missing tools.
func listOrLast[T named](ctx context.Context, inspector api.ClusterInspector, gvr schema.GroupVersionResource, last *lastListed[T]) []T {
	items, err := listResources[T](ctx, inspector, gvr)
	last.mu.Lock()
	defer last.mu.Unlock()
	if err == nil {
		last.items, last.listed = items, true
		return items
	}
	logger := klogutil.FromContext(ctx)
	if !last.listed {
		logger.Error(err, "dynamic toolset: unable to list resources, exposing none", "resource", gvr.Resource)
		return nil
	}
	logger.Error(err, "dynamic toolset: unable to list resources, keeping the last known ones", "resource", gvr.Resource, "count", len(last.items))
	return last.items
}

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
	definitions := listOrLast(ctx, toolsetContext.Inspector, GVR, &t.tools)
	builtIn := builtInNames(ctx, toolsetContext)
	// Two resources may resolve to the same MCP name (foo-bar and foo.bar,
	// or an explicit spec.name): the server refuses a duplicate, which would
	// take every toolset down, so only the first (by resource name) is kept.
	seen := map[string]string{}
	tools := make([]api.ServerTool, 0, len(definitions))
	for _, definition := range definitions {
		// One broken definition must not take the others down: skip it and say why.
		compiled, err := newCompiledTool(definition)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Tool resource", "tool", definition.Metadata.Name, "error", err.Error())
			continue
		}
		// A Tool must not replace a compiled-in tool (it could, for example,
		// declare a destructive operation read-only). The server would
		// otherwise register whichever came last.
		if owner, taken := builtIn.tools[compiled.name]; taken {
			logger.Error(nil, "dynamic toolset: skipping Tool resource whose name shadows a built-in tool",
				"tool", definition.Metadata.Name, "name", compiled.name, "toolset", owner)
			continue
		}
		if first, taken := seen[compiled.name]; taken {
			logger.Error(nil, "dynamic toolset: skipping Tool resource whose name is already taken by another Tool resource",
				"tool", definition.Metadata.Name, "name", compiled.name, "first", first)
			continue
		}
		serverTool, err := compiled.serverTool(ctx, toolsetContext.Inspector)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Tool resource", "tool", definition.Metadata.Name, "error", err.Error())
			continue
		}
		seen[compiled.name] = definition.Metadata.Name
		tools = append(tools, serverTool)
	}
	return tools
}

func (t *Toolset) GetPrompts(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerPrompt {
	logger := klogutil.FromContext(ctx)
	definitions := listOrLast(ctx, toolsetContext.Inspector, PromptGVR, &t.prompts)
	var discovery api.AggregateDiscovery
	if toolsetContext.Inspector != nil {
		discovery = toolsetContext.Inspector.Discovery()
	}
	builtIn := builtInNames(ctx, toolsetContext)
	seen := map[string]string{}
	prompts := make([]api.ServerPrompt, 0, len(definitions))
	for _, definition := range definitions {
		compiled, err := newCompiledPrompt(definition)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Prompt resource", "prompt", definition.Metadata.Name, "error", err.Error())
			continue
		}
		if owner, taken := builtIn.prompts[compiled.name]; taken {
			logger.Error(nil, "dynamic toolset: skipping Prompt resource whose name shadows a built-in prompt",
				"prompt", definition.Metadata.Name, "name", compiled.name, "toolset", owner)
			continue
		}
		if first, taken := seen[compiled.name]; taken {
			logger.Error(nil, "dynamic toolset: skipping Prompt resource whose name is already taken by another Prompt resource",
				"prompt", definition.Metadata.Name, "name", compiled.name, "first", first)
			continue
		}
		seen[compiled.name] = definition.Metadata.Name
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

// builtIns indexes the tool and prompt names of the compiled-in toolsets by
// the toolset that owns them.
type builtIns struct {
	tools   map[string]string
	prompts map[string]string
}

// builtInNames collects the names every other enabled toolset exposes, so
// that a Tool or Prompt resource cannot take one over. Only enabled toolsets
// count: with the core toolset disabled, a Tool may legitimately provide a
// pods_list of its own (examples/dynamic-tools/converted does), and once
// core is enabled again that Tool is skipped on the re-apply.
func builtInNames(ctx context.Context, toolsetContext api.ToolsetContext) builtIns {
	names := builtIns{tools: map[string]string{}, prompts: map[string]string{}}
	for _, toolset := range toolsetContext.Toolsets {
		if _, self := toolset.(*Toolset); self {
			continue
		}
		for _, tool := range toolset.GetTools(ctx, toolsetContext) {
			names.tools[tool.Tool.Name] = toolset.GetName()
		}
		for _, prompt := range toolset.GetPrompts(ctx, toolsetContext) {
			names.prompts[prompt.Prompt.Name] = toolset.GetName()
		}
	}
	return names
}

// WatchToolset polls the Tool and Prompt resources and asks the server to
// re-apply the toolsets whenever their set or resourceVersions change.
// Polling mirrors the cluster-state watcher; an informer would be the
// production choice.
//
// A list failure is logged once at default verbosity when it starts and once
// when it ends; the failures in between are logged at verbosity 1. A missing
// ClusterRole would otherwise produce one error line per poll, forever.
func (t *Toolset) WatchToolset(ctx context.Context, toolsetContext api.ToolsetContext, onChange func() error) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	logger := klogutil.FromContext(ctx)
	last, err := fingerprint(ctx, toolsetContext.Inspector)
	failing := err != nil
	if failing {
		logger.Error(err, "dynamic toolset: unable to list Tool or Prompt resources, will keep polling (further failures are logged at verbosity 1)")
	}
	go func() {
		ticker := time.NewTicker(time.Duration(pollInterval.Load()))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			// A failed poll says nothing about the resources: skip it rather
			// than mistake it for "every Tool was deleted", which would drop
			// all dynamic tools and flap them back on the next success.
			current, err := fingerprint(ctx, toolsetContext.Inspector)
			if err != nil {
				if failing {
					logger.V(1).Info("dynamic toolset: unable to list Tool or Prompt resources, keeping the current tools", "error", err.Error())
				} else {
					logger.Error(err, "dynamic toolset: unable to list Tool or Prompt resources, keeping the current tools (further failures are logged at verbosity 1)")
				}
				failing = true
				continue
			}
			if failing {
				logger.Info("dynamic toolset: Tool and Prompt resources can be listed again")
				failing = false
			}
			if current == last {
				continue
			}
			logger.V(1).Info("dynamic toolset: Tool or Prompt resources changed, re-applying toolsets")
			// The change counts as applied only once it is: a failed re-apply
			// is retried on the next poll rather than forgotten until the
			// resources change again.
			if err := onChange(); err != nil {
				logger.Error(err, "dynamic toolset: failed to re-apply toolsets, will retry on the next poll")
				continue
			}
			last = current
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

// fingerprint summarizes the names and resourceVersions of the Tool and
// Prompt resources. A list failure is returned as such: an empty fingerprint
// means there are no resources, never that the cluster could not be asked.
func fingerprint(ctx context.Context, inspector api.ClusterInspector) (string, error) {
	var parts []string
	tools, err := listResources[Tool](ctx, inspector, GVR)
	if err != nil {
		return "", err
	}
	for _, tool := range tools {
		parts = append(parts, "tool/"+tool.Metadata.Name+"="+tool.Metadata.ResourceVersion)
	}
	prompts, err := listResources[Prompt](ctx, inspector, PromptGVR)
	if err != nil {
		return "", err
	}
	for _, prompt := range prompts {
		parts = append(parts, "prompt/"+prompt.Metadata.Name+"="+prompt.Metadata.ResourceVersion)
	}
	return strings.Join(parts, "\n"), nil
}

func init() {
	toolsets.Register(&Toolset{})
}
