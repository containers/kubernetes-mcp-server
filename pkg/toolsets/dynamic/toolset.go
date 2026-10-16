package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	tools    lastListed[Tool]
	prompts  lastListed[Prompt]
	toolsets lastListed[ToolSet]
	// statusUnavailable remembers that status writes fail (no RBAC on the
	// status subresources, or an older CRD), so it is said once.
	statusUnavailable atomic.Bool
	// One poll loop per caller context: every enabled family asks for the
	// same watch (the fingerprint covers all Tool and Prompt resources,
	// not one family's), so the families of one server share a loop,
	// refcounted by stop. Keyed by context so servers (each with a
	// lifetime context of its own) never share or stop each other's loop.
	watchMu sync.Mutex
	watches map[context.Context]*watchLoop
}

// watchLoop is one poll loop, shared by every family watch of a server.
type watchLoop struct {
	stop func()
	refs int
}

// outcome is what the server did with one resource, written to its status.
type outcome struct {
	ready   bool
	reason  string
	message string
	// name is the MCP name the resource is exposed as, when it is.
	name string
}

// Reasons of the Ready condition.
const (
	ReasonRegistered     = "Registered"
	ReasonInvalid        = "Invalid"
	ReasonMalformed      = "Malformed"
	ReasonShadowsBuiltIn = "ShadowsBuiltIn"
	ReasonNameTaken      = "NameTaken"
	ReasonUnknownToolSet = "UnknownToolSet"
	ReasonMissingPrefix  = "MissingPrefix"
	ReasonWithheld       = "Withheld"
)

// reportStatus writes the Ready condition of every resource the collection
// decided about, through the server's own identity, when it changed. Status
// is best effort: a server without update on the status subresources keeps
// working and says so once.
func (t *Toolset) reportStatus(ctx context.Context, toolsetContext api.ToolsetContext, gvr schema.GroupVersionResource, kind string, items []listedStatus, outcomes map[string]outcome) {
	if toolsetContext.ServerClients == nil || len(outcomes) == 0 {
		return
	}
	logger := klogutil.FromContext(ctx)
	var client dynamic.Interface
	for _, item := range items {
		decided, ok := outcomes[item.name]
		if !ok {
			continue
		}
		status := metav1.ConditionFalse
		if decided.ready {
			status = metav1.ConditionTrue
		}
		if current := meta.FindStatusCondition(item.conditions, ConditionReady); current != nil &&
			current.Status == status && current.Reason == decided.reason && current.Message == decided.message &&
			current.ObservedGeneration == item.generation && item.exposed == decided.name {
			continue
		}
		if client == nil {
			server, err := toolsetContext.ServerClients(ctx, toolsetContext.DefaultTarget)
			if err != nil || server == nil || server.DynamicClient() == nil {
				logger.V(1).Info("dynamic toolset: not reporting resource status, no server client", "resource", gvr.Resource)
				return
			}
			client = server.DynamicClient()
		}
		conditions := append([]metav1.Condition{}, item.conditions...)
		meta.SetStatusCondition(&conditions, metav1.Condition{
			Type: ConditionReady, Status: status, Reason: decided.reason, Message: decided.message, ObservedGeneration: item.generation,
		})
		raw, err := json.Marshal(conditions)
		if err != nil {
			continue
		}
		var encoded []any
		_ = json.Unmarshal(raw, &encoded)
		reported := map[string]any{"conditions": encoded}
		if decided.name != "" {
			reported["name"] = decided.name
		}
		object := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": Group + "/" + Version, "kind": kind,
			"metadata": map[string]any{"name": item.name, "resourceVersion": item.resourceVersion},
			"status":   reported,
		}}
		if _, err := client.Resource(gvr).UpdateStatus(ctx, object, metav1.UpdateOptions{FieldManager: "kubernetes-mcp-server"}); err != nil {
			if api.IsNotFound(err) || apierrors.IsConflict(err) {
				// Changed or gone since the list: the next apply reports it.
				continue
			}
			if !t.statusUnavailable.Swap(true) {
				logger.Info("dynamic toolset: unable to write resource status; tools work regardless", "resource", gvr.Resource, "error", err.Error(), "hint", rbacHint)
			} else {
				logger.V(1).Info("dynamic toolset: unable to write resource status", "resource", gvr.Resource, "error", err.Error())
			}
			return
		}
		if t.statusUnavailable.Swap(false) {
			logger.Info("dynamic toolset: resource status can be written again", "resource", gvr.Resource)
		}
	}
}

// listedStatus is what reportStatus needs of a listed resource.
type listedStatus struct {
	name, resourceVersion string
	generation            int64
	conditions            []metav1.Condition
	// exposed is status.name as listed.
	exposed string
}

func statusOf[T named](items []T) []listedStatus {
	out := make([]listedStatus, 0, len(items))
	for _, item := range items {
		m := metadataOf(item)
		var conditions []metav1.Condition
		var exposed string
		switch v := any(item).(type) {
		case Tool:
			conditions = v.Status.Conditions
			exposed = v.Status.Name
		case Prompt:
			conditions = v.Status.Conditions
			exposed = v.Status.Name
		case ToolSet:
			conditions = v.Status.Conditions
			exposed = v.Status.Name
		}
		out = append(out, listedStatus{name: m.Name, resourceVersion: m.ResourceVersion, generation: m.Generation, conditions: conditions, exposed: exposed})
	}
	return out
}

// Family returns the toolset exposing only the Tools and Prompts of one
// ToolSet resource, enabled as "dynamic/<name>". The ToolSet need not exist
// yet (it lives in the cluster): the family is simply empty until it does.
func (t *Toolset) Family(name string) api.Toolset {
	family, ok := strings.CutPrefix(name, FamilyPrefix)
	if !ok || family == "" || strings.Contains(family, "/") {
		return nil
	}
	return &familyToolset{Toolset: t, family: family}
}

var _ api.ToolsetFamilies = (*Toolset)(nil)

// familyToolset is the dynamic toolset restricted to one ToolSet resource.
// It shares the base toolset's caches and watcher.
type familyToolset struct {
	*Toolset
	family string
}

var _ api.Toolset = (*familyToolset)(nil)
var _ api.ToolsetWatcher = (*familyToolset)(nil)
var _ api.ToolsetInstructions = (*familyToolset)(nil)

func (f *familyToolset) GetName() string { return FamilyPrefix + f.family }

func (f *familyToolset) GetDescription() string {
	return "Tools and prompts of the " + f.family + " ToolSet resource (" + Group + ")"
}

func (f *familyToolset) GetTools(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerTool {
	return f.toolsOf(ctx, toolsetContext, f.family)
}

func (f *familyToolset) GetPrompts(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerPrompt {
	return f.promptsOf(ctx, toolsetContext, f.family)
}

func (f *familyToolset) GetInstructions(ctx context.Context, toolsetContext api.ToolsetContext) string {
	return f.instructions(ctx, toolsetContext, f.family)
}

// isDynamic reports whether a toolset is the dynamic toolset or one of its
// families, which share one set of resources and must not count each other
// as built-ins.
func isDynamic(toolset api.Toolset) bool {
	switch toolset.(type) {
	case *Toolset, *familyToolset:
		return true
	}
	return false
}

// families indexes the ToolSet resources by name. unavailable says the
// ToolSet list failed and nothing is known, so a tool naming a family is
// told that rather than that its family does not exist.
type families struct {
	sets        map[string]ToolSet
	unavailable bool
}

// familyOf resolves the ToolSet a definition belongs to. It reports whether
// the definition is part of the family being collected ("" collects every
// definition) and, when it names an unknown ToolSet or lacks the family's
// prefix, why it is skipped. Tools are named "<prefix>_...", prompts
// "<prefix>-..." (MCP prompt names are dashed); separator says which.
func (f families) familyOf(toolset, name, separator string, family string) (ToolSet, bool, string) {
	if family != "" && toolset != family {
		return ToolSet{}, false, ""
	}
	if toolset == "" {
		return ToolSet{}, true, ""
	}
	owner, ok := f.sets[toolset]
	if !ok {
		if f.unavailable {
			return ToolSet{}, false, "names a ToolSet resource, but ToolSet resources could not be listed: " + toolset + " (" + rbacHint + ")"
		}
		return ToolSet{}, false, "names a ToolSet resource that does not exist: " + toolset
	}
	if owner.Spec.Prefix != "" && !strings.HasPrefix(name, owner.Spec.Prefix+separator) {
		return ToolSet{}, false, fmt.Sprintf("name %q does not carry the %q prefix of its ToolSet %s", name, owner.Spec.Prefix+separator, toolset)
	}
	return owner, true, ""
}

// listFamilies indexes the ToolSet resources. Tools and prompts are
// collected by separate calls of one apply; the second lists quietly so a
// failing list is reported once per kind, as for Tools and Prompts.
func (t *Toolset) listFamilies(ctx context.Context, toolsetContext api.ToolsetContext, family string, quiet bool) families {
	logger := klogutil.FromContext(ctx)
	index := families{sets: map[string]ToolSet{}}
	listed, ok := listOrLast(ctx, toolsetContext.Inspector, ToolSetGVR, &t.toolsets, quiet, toolsetContext.ToolSelector)
	index.unavailable = !ok && listed == nil
	outcomes := map[string]outcome{}
	for _, toolset := range listed {
		if err := toolset.Validate(); err != nil {
			logger.Info("dynamic toolset: skipping invalid ToolSet resource", "toolset", toolset.Metadata.Name, "error", err.Error())
			outcomes[toolset.Metadata.Name] = outcome{reason: ReasonInvalid, message: err.Error()}
			continue
		}
		index.sets[toolset.Metadata.Name] = toolset
		// A family that is not enabled is not written to, as for Tools and
		// Prompts: with only dynamic/kiali enabled, the prometheus ToolSet
		// must not be told it is enabled.
		if family != "" && toolset.Metadata.Name != family {
			continue
		}
		outcomes[toolset.Metadata.Name] = outcome{ready: true, reason: ReasonRegistered, message: "enabled as " + toolset.ToolsetName()}
	}
	if !quiet {
		t.reportStatus(ctx, toolsetContext, ToolSetGVR, ToolSetKind, statusOf(listed), outcomes)
	}
	return index
}

// GetInstructions contributes the instructions of every ToolSet resource,
// each under a heading with its name.
func (t *Toolset) GetInstructions(ctx context.Context, toolsetContext api.ToolsetContext) string {
	return t.instructions(ctx, toolsetContext, "")
}

func (t *Toolset) instructions(ctx context.Context, toolsetContext api.ToolsetContext, family string) string {
	index := t.listFamilies(ctx, toolsetContext, family, true)
	names := make([]string, 0, len(index.sets))
	for name := range index.sets {
		if family == "" || name == family {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var sections []string
	for _, name := range names {
		if text := strings.TrimSpace(index.sets[name].Spec.Instructions); text != "" {
			sections = append(sections, "## "+name+"\n\n"+text)
		}
	}
	return strings.Join(sections, "\n\n")
}

// lastListed remembers the result of the last successful list of one kind.
type lastListed[T named] struct {
	mu     sync.Mutex
	items  []T
	listed bool
	// selectorKey is the dynamic_tools_selector the items were listed
	// under, so a cache filled under one policy is never served under
	// another: a tightened selector must not fail open through a list
	// failure.
	selectorKey string
}

// listOrLast lists the resources of one kind, falling back to the last
// successful list when the cluster cannot be asked. The error is logged at
// default verbosity: the usual cause is a missing ClusterRole on the
// server's ServiceAccount, and the operator would otherwise only see
// missing tools.
//
// With quiet set, a failure is logged at verbosity 1 only: the ToolSet
// resources are listed by both the tool and the prompt collection of one
// apply, and the failure is reported once.
func listOrLast[T named](ctx context.Context, inspector api.ClusterInspector, gvr schema.GroupVersionResource, last *lastListed[T], quiet bool, selector labels.Selector) ([]T, bool) {
	items, err := listResources[T](ctx, inspector, gvr, selector)
	key := ""
	if selector != nil {
		key = selector.String()
	}
	last.mu.Lock()
	defer last.mu.Unlock()
	if err == nil {
		last.items, last.listed, last.selectorKey = items, true, key
		return items, true
	}
	logger := klogutil.FromContext(ctx)
	report := func(message string, keysAndValues ...any) {
		if apierrors.IsForbidden(err) {
			keysAndValues = append(keysAndValues, "hint", rbacHint)
		}
		if quiet {
			logger.V(1).Info(message, append([]any{"error", err.Error()}, keysAndValues...)...)
			return
		}
		logger.Error(err, message, keysAndValues...)
	}
	if !last.listed || last.selectorKey != key {
		// Never cached, or cached under a different dynamic_tools_selector:
		// serving it would apply the wrong policy.
		report("dynamic toolset: unable to list resources, exposing none", "resource", gvr.Resource)
		return nil, false
	}
	report("dynamic toolset: unable to list resources, keeping the last known ones", "resource", gvr.Resource, "count", len(last.items))
	return last.items, false
}

// rbacHint is what the server's identity needs for the dynamic toolset,
// appended to every forbidden list or status write.
const rbacHint = "the server's identity needs get, list and watch on tools, prompts and toolsets in tools.kubernetes-mcp-server.io" +
	" and update on their status subresources; the server Helm chart's rbac.dynamicTools: true grants it"

var _ api.Toolset = (*Toolset)(nil)
var _ api.ToolsetWatcher = (*Toolset)(nil)
var _ api.ToolsetInstructions = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "dynamic"
}

func (t *Toolset) GetDescription() string {
	return "Tools and prompts defined declaratively as Tool and Prompt custom resources (" + Group + ") installed in the cluster, see docs/specs/dynamic-tools.md"
}

func (t *Toolset) GetTools(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerTool {
	return t.toolsOf(ctx, toolsetContext, "")
}

// toolsOf collects the Tool resources of one family ("" for all of them).
func (t *Toolset) toolsOf(ctx context.Context, toolsetContext api.ToolsetContext, family string) []api.ServerTool {
	logger := klogutil.FromContext(ctx)
	definitions, _ := listOrLast(ctx, toolsetContext.Inspector, GVR, &t.tools, false, toolsetContext.ToolSelector)
	index := t.listFamilies(ctx, toolsetContext, family, false)
	builtIn := builtInNames(ctx, toolsetContext)
	// Two resources may resolve to the same MCP name (foo-bar and foo.bar,
	// or an explicit spec.name): the server refuses a duplicate, which would
	// take every toolset down, so only the first (by resource name) is kept.
	seen := map[string]string{}
	outcomes := map[string]outcome{}
	tools := make([]api.ServerTool, 0, len(definitions))
	for _, definition := range definitions {
		owner, mine, reason := index.familyOf(definition.Spec.ToolSet, definition.ToolName(), "_", family)
		if reason != "" {
			logger.Info("dynamic toolset: skipping Tool resource", "tool", definition.Metadata.Name, "reason", reason)
			outcomes[definition.Metadata.Name] = outcome{reason: familyReason(reason), message: reason}
			continue
		}
		if !mine {
			continue
		}
		definition.Spec.Requires = append(append([]GVKRef{}, owner.Spec.Requires...), definition.Spec.Requires...)
		// One broken definition must not take the others down: skip it and say why.
		compiled, err := newCompiledTool(definition)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Tool resource", "tool", definition.Metadata.Name, "error", err.Error())
			outcomes[definition.Metadata.Name] = outcome{reason: ReasonInvalid, message: err.Error()}
			continue
		}
		compiled.setIdentity(owner.Spec.Identity, toolsetContext.ServerClients)
		// A Tool must not replace a compiled-in tool (it could, for example,
		// declare a destructive operation read-only). The server would
		// otherwise register whichever came last.
		if owner, taken := builtIn.tools[compiled.name]; taken {
			logger.Error(nil, "dynamic toolset: skipping Tool resource whose name shadows a built-in tool",
				"tool", definition.Metadata.Name, "name", compiled.name, "toolset", owner)
			outcomes[definition.Metadata.Name] = outcome{reason: ReasonShadowsBuiltIn, message: "name " + compiled.name + " is a built-in tool of the " + owner + " toolset"}
			continue
		}
		// A fixed http url is operator policy and visible at registration;
		// urlExpression hosts are checked at call time, when they are known.
		withheld := ""
		for _, url := range fixedHTTPURLs(definition.Spec) {
			if host := hostOf(url); !httpHostAllowed(toolsetContext.AllowedHTTPHosts, host) {
				logger.Error(nil, "dynamic toolset: skipping Tool resource whose http url host is not allowed",
					"tool", definition.Metadata.Name, "host", host)
				withheld = fmt.Sprintf("http url host %q is not in the server's allowed_http_hosts", host)
				break
			}
		}
		if withheld != "" {
			outcomes[definition.Metadata.Name] = outcome{reason: ReasonWithheld, message: withheld}
			continue
		}
		if first, taken := seen[compiled.name]; taken {
			logger.Error(nil, "dynamic toolset: skipping Tool resource whose name is already taken by another Tool resource",
				"tool", definition.Metadata.Name, "name", compiled.name, "first", first)
			outcomes[definition.Metadata.Name] = outcome{reason: ReasonNameTaken, message: "name " + compiled.name + " is already exposed by Tool " + first}
			continue
		}
		serverTool, err := compiled.serverTool(ctx, toolsetContext.Inspector)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Tool resource", "tool", definition.Metadata.Name, "error", err.Error())
			outcomes[definition.Metadata.Name] = outcome{reason: ReasonInvalid, message: err.Error()}
			continue
		}
		seen[compiled.name] = definition.Metadata.Name
		message := "exposed as tool " + compiled.name
		if lints := definition.Lint(); len(lints) > 0 {
			logger.V(1).Info("dynamic toolset: Tool resource does not follow the tools-for-models guidance", "tool", definition.Metadata.Name, "lints", lints)
			message += "; lint: " + strings.Join(lints, "; ")
		}
		outcomes[definition.Metadata.Name] = outcome{ready: true, reason: ReasonRegistered, message: message, name: compiled.name}
		tools = append(tools, serverTool)
	}
	if len(tools) > MaxToolsPerToolSet {
		logger.Info("dynamic toolset: a ToolSet with more than twelve tools degrades the model's selection accuracy; split it or merge tools",
			"toolset", family, "tools", len(tools))
	}
	t.reportStatus(ctx, toolsetContext, GVR, Kind, statusOf(definitions), outcomes)
	return tools
}

// familyReason maps a familyOf skip reason to a condition reason.
func familyReason(reason string) string {
	if strings.HasPrefix(reason, "names a ToolSet") {
		return ReasonUnknownToolSet
	}
	return ReasonMissingPrefix
}

func (t *Toolset) GetPrompts(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerPrompt {
	return t.promptsOf(ctx, toolsetContext, "")
}

// promptsOf collects the Prompt resources of one family ("" for all of them).
func (t *Toolset) promptsOf(ctx context.Context, toolsetContext api.ToolsetContext, family string) []api.ServerPrompt {
	logger := klogutil.FromContext(ctx)
	definitions, _ := listOrLast(ctx, toolsetContext.Inspector, PromptGVR, &t.prompts, false, toolsetContext.ToolSelector)
	index := t.listFamilies(ctx, toolsetContext, family, true)
	var discovery api.AggregateDiscovery
	if toolsetContext.Inspector != nil {
		discovery = toolsetContext.Inspector.Discovery()
	}
	builtIn := builtInNames(ctx, toolsetContext)
	seen := map[string]string{}
	outcomes := map[string]outcome{}
	prompts := make([]api.ServerPrompt, 0, len(definitions))
	for _, definition := range definitions {
		owner, mine, reason := index.familyOf(definition.Spec.ToolSet, definition.PromptName(), "-", family)
		if reason != "" || !mine {
			if reason != "" {
				logger.Info("dynamic toolset: skipping Prompt resource", "prompt", definition.Metadata.Name, "reason", reason)
				outcomes[definition.Metadata.Name] = outcome{reason: familyReason(reason), message: reason}
			}
			continue
		}
		if toolsetContext.ReadOnly && definition.Mutates() {
			logger.V(1).Info("dynamic toolset: withholding Prompt resource that changes cluster state in read-only mode", "prompt", definition.Metadata.Name)
			outcomes[definition.Metadata.Name] = outcome{reason: ReasonWithheld, message: "changes cluster state; withheld while the server is read-only"}
			continue
		}
		compiled, err := newCompiledPrompt(definition)
		if err != nil {
			logger.Info("dynamic toolset: skipping invalid Prompt resource", "prompt", definition.Metadata.Name, "error", err.Error())
			outcomes[definition.Metadata.Name] = outcome{reason: ReasonInvalid, message: err.Error()}
			continue
		}
		if owner, taken := builtIn.prompts[compiled.name]; taken {
			logger.Error(nil, "dynamic toolset: skipping Prompt resource whose name shadows a built-in prompt",
				"prompt", definition.Metadata.Name, "name", compiled.name, "toolset", owner)
			outcomes[definition.Metadata.Name] = outcome{reason: ReasonShadowsBuiltIn, message: "name " + compiled.name + " is a built-in prompt of the " + owner + " toolset"}
			continue
		}
		// As for Tools: a fixed http url is operator policy and visible at
		// registration; urlExpression hosts are checked at call time.
		withheld := ""
		for _, url := range stepsHTTPURLs(definition.Spec.Steps) {
			if host := hostOf(url); !httpHostAllowed(toolsetContext.AllowedHTTPHosts, host) {
				logger.Error(nil, "dynamic toolset: skipping Prompt resource whose http url host is not allowed",
					"prompt", definition.Metadata.Name, "host", host)
				withheld = fmt.Sprintf("http url host %q is not in the server's allowed_http_hosts", host)
				break
			}
		}
		if withheld != "" {
			outcomes[definition.Metadata.Name] = outcome{reason: ReasonWithheld, message: withheld}
			continue
		}
		if first, taken := seen[compiled.name]; taken {
			logger.Error(nil, "dynamic toolset: skipping Prompt resource whose name is already taken by another Prompt resource",
				"prompt", definition.Metadata.Name, "name", compiled.name, "first", first)
			outcomes[definition.Metadata.Name] = outcome{reason: ReasonNameTaken, message: "name " + compiled.name + " is already exposed by Prompt " + first}
			continue
		}
		compiled.runner.setIdentity(owner.Spec.Identity, toolsetContext.ServerClients)
		seen[compiled.name] = definition.Metadata.Name
		outcomes[definition.Metadata.Name] = outcome{ready: true, reason: ReasonRegistered, message: "exposed as prompt " + compiled.name, name: compiled.name}
		prompts = append(prompts, compiled.serverPrompt(ctx, discovery))
	}
	t.reportStatus(ctx, toolsetContext, PromptGVR, PromptKind, statusOf(definitions), outcomes)
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

// builtInNames collects the names every other registered toolset exposes —
// enabled or not — so that a Tool or Prompt resource cannot take one over:
// a resource named like the tool of a toolset that happens to be disabled
// today must not quietly become the tool clients call once that toolset is
// enabled again.
func builtInNames(ctx context.Context, toolsetContext api.ToolsetContext) builtIns {
	names := builtIns{tools: map[string]string{}, prompts: map[string]string{}}
	for _, toolset := range toolsets.Toolsets() {
		if isDynamic(toolset) {
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
	t.watchMu.Lock()
	if t.watches == nil {
		t.watches = map[context.Context]*watchLoop{}
	}
	loop := t.watches[ctx]
	if loop == nil {
		loop = &watchLoop{stop: t.pollResources(ctx, toolsetContext, onChange)}
		t.watches[ctx] = loop
	}
	loop.refs++
	t.watchMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			t.watchMu.Lock()
			defer t.watchMu.Unlock()
			if loop.refs--; loop.refs == 0 {
				loop.stop()
				delete(t.watches, ctx)
			}
		})
	}
}

// pollResources is the single poll loop behind WatchToolset.
func (t *Toolset) pollResources(ctx context.Context, toolsetContext api.ToolsetContext, onChange func() error) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	logger := klogutil.FromContext(ctx)
	last, err := fingerprint(ctx, toolsetContext.Inspector, toolsetContext.ToolSelector)
	failing := err != nil
	if failing {
		logger.Error(err, "dynamic toolset: unable to list Tool or Prompt resources, will keep polling (further failures are logged at verbosity 1)", "hint", rbacHint)
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
			current, err := fingerprint(ctx, toolsetContext.Inspector, toolsetContext.ToolSelector)
			if err != nil {
				if failing {
					logger.V(1).Info("dynamic toolset: unable to list Tool or Prompt resources, keeping the current tools", "error", err.Error())
				} else {
					logger.Error(err, "dynamic toolset: unable to list Tool or Prompt resources, keeping the current tools (further failures are logged at verbosity 1)", "hint", rbacHint)
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
	Tool | Prompt | ToolSet
}

// listResources returns the resources of the default target, sorted by name.
// A missing CRD is not an error: it simply means no definitions.
func listResources[T named](ctx context.Context, inspector api.ClusterInspector, gvr schema.GroupVersionResource, selector labels.Selector) ([]T, error) {
	if inspector == nil || inspector.Unstructured() == nil {
		return nil, errors.New("no cluster inspector available")
	}
	options := metav1.ListOptions{}
	if selector != nil {
		// dynamic_tools_selector: the API server does the filtering, so a
		// resource outside the policy is not read at all.
		options.LabelSelector = selector.String()
	}
	list, err := inspector.Unstructured().Resource(gvr).List(ctx, options).Default()
	if err != nil {
		if api.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	// A resource whose shape does not fit the definition (the CRD leaves the
	// reference fields open) is skipped like an invalid one: one bad
	// `kubectl apply` must not take every other tool down with it.
	logger := klogutil.FromContext(ctx)
	items := make([]T, 0, len(list.Items))
	for _, item := range list.Items {
		raw, err := json.Marshal(item.Object)
		if err != nil {
			return nil, err
		}
		var decoded T
		if err := json.Unmarshal(raw, &decoded); err != nil {
			logger.Info("dynamic toolset: skipping malformed resource", "resource", gvr.Resource, "name", item.GetName(), "error", err.Error())
			continue
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
	case ToolSet:
		return v.Metadata
	}
	return Metadata{}
}

// fingerprint summarizes the names and resourceVersions of the Tool and
// Prompt resources. A list failure is returned as such: an empty fingerprint
// means there are no resources, never that the cluster could not be asked.
func fingerprint(ctx context.Context, inspector api.ClusterInspector, selector labels.Selector) (string, error) {
	var parts []string
	tools, err := listResources[Tool](ctx, inspector, GVR, selector)
	if err != nil {
		return "", err
	}
	for _, tool := range tools {
		parts = append(parts, "tool/"+tool.Metadata.Name+"="+tool.Metadata.ResourceVersion)
	}
	prompts, err := listResources[Prompt](ctx, inspector, PromptGVR, selector)
	if err != nil {
		return "", err
	}
	for _, prompt := range prompts {
		parts = append(parts, "prompt/"+prompt.Metadata.Name+"="+prompt.Metadata.ResourceVersion)
	}
	toolsets, err := listResources[ToolSet](ctx, inspector, ToolSetGVR, selector)
	if err != nil {
		return "", err
	}
	for _, toolset := range toolsets {
		parts = append(parts, "toolset/"+toolset.Metadata.Name+"="+toolset.Metadata.ResourceVersion)
	}
	return strings.Join(parts, "\n"), nil
}

func init() {
	toolsets.Register(&Toolset{})
}
