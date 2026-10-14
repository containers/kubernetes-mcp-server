package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

// runEnv is what an action needs from the caller: a context, the derived
// Kubernetes client and (optionally) an elicitor. Both tool and prompt
// handler params provide it.
type runEnv struct {
	ctx      context.Context
	client   api.KubernetesClient
	elicitor api.Elicitor
	// requireTLS reports the server's require_tls setting; plain-HTTP
	// requests of http actions are refused while it is true.
	requireTLS func() bool
}

// binaryResult is the result of an action declared with response: binary.
type binaryResult struct {
	mimeType string
	data     []byte
}

// summary is how a binary result appears to CEL expressions.
func (b *binaryResult) summary() map[string]any {
	return map[string]any{"mimeType": b.mimeType, "size": len(b.data)}
}

// compiledAction is one kubernetes or http action with its expressions
// compiled.
type compiledAction struct {
	kubernetes *KubernetesAction
	http       *HTTPAction
	// transport is the shared HTTP transport of a url-mode http action,
	// built once so that calls reuse (and eventually reap) connections.
	transport *http.Transport
	bodyExpr  *expression
	queryExpr *expression
}

// compiledStep is one step of a composed Tool or Prompt.
type compiledStep struct {
	spec    Step
	action  *compiledAction
	expr    *expression
	fail    *expression
	elicit  *template
	when    *expression
	forEach *expression
}

// compiledTool is a validated Tool definition with every CEL expression
// compiled once at registration time, so a broken definition is rejected
// (and logged) when the toolset is applied rather than on first call.
type compiledTool struct {
	name       string
	spec       ToolSpec
	action     *compiledAction // single-action tools
	steps      []*compiledStep // composed tools
	outputExpr *expression
	// refExprs holds the compiled Ref expressions keyed by their source.
	refExprs map[string]*expression
}

func newCompiledTool(tool Tool) (*compiledTool, error) {
	if err := tool.Validate(); err != nil {
		return nil, err
	}
	c := &compiledTool{name: tool.ToolName(), spec: tool.Spec, refExprs: map[string]*expression{}}
	if err := c.compileRefs(c.spec.refs()); err != nil {
		return nil, err
	}
	var err error
	if c.spec.Output != "" {
		if c.outputExpr, err = compileExpression(c.spec.Output); err != nil {
			return nil, fmt.Errorf("spec.output: %w", err)
		}
	}
	if len(c.spec.Steps) == 0 {
		if c.action, err = compileAction(c.spec.Kubernetes, c.spec.HTTP); err != nil {
			return nil, err
		}
		return c, nil
	}
	if c.steps, err = compileSteps(c.spec.Steps); err != nil {
		return nil, err
	}
	return c, nil
}

// compileStepsOnly builds the step runner for a definition that is not a
// Tool (a Prompt). The caller has validated the steps.
func compileStepsOnly(steps []Step) (*compiledTool, error) {
	c := &compiledTool{spec: ToolSpec{Steps: steps}, refExprs: map[string]*expression{}}
	if err := c.compileRefs(c.spec.refs()); err != nil {
		return nil, err
	}
	var err error
	c.steps, err = compileSteps(steps)
	return c, err
}

func (c *compiledTool) compileRefs(refs []*Ref) error {
	for _, ref := range refs {
		if ref == nil || ref.Expression == "" {
			continue
		}
		if _, done := c.refExprs[ref.Expression]; done {
			continue
		}
		expr, err := compileExpression(ref.Expression)
		if err != nil {
			return err
		}
		c.refExprs[ref.Expression] = expr
	}
	return nil
}

func compileSteps(specs []Step) ([]*compiledStep, error) {
	steps := make([]*compiledStep, 0, len(specs))
	for i := range specs {
		step, err := compileStep(specs[i])
		if err != nil {
			return nil, fmt.Errorf("spec.steps[%d] (%s): %w", i, specs[i].Name, err)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func compileAction(kubernetes *KubernetesAction, http *HTTPAction) (*compiledAction, error) {
	a := &compiledAction{kubernetes: kubernetes, http: http}
	var body *Body
	var query string
	switch {
	case kubernetes != nil:
		body, query = kubernetes.Body, kubernetes.QueryExpression
	case http != nil:
		body, query = http.Body, http.QueryExpression
	}
	var err error
	if http != nil && http.Service == nil {
		if a.transport, err = newHTTPTransport(http); err != nil {
			return nil, err
		}
	}
	if query != "" {
		if a.queryExpr, err = compileExpression(query); err != nil {
			return nil, fmt.Errorf("queryExpression: %w", err)
		}
	}
	if body != nil && body.Expression != "" {
		if a.bodyExpr, err = compileExpression(body.Expression); err != nil {
			return nil, fmt.Errorf("body.expression: %w", err)
		}
	}
	return a, nil
}

func compileStep(spec Step) (*compiledStep, error) {
	step := &compiledStep{spec: spec}
	var err error
	switch {
	case spec.Expression != "":
		if step.expr, err = compileExpression(spec.Expression); err != nil {
			return nil, fmt.Errorf("expression: %w", err)
		}
	case spec.Fail != "":
		if step.fail, err = compileExpression(spec.Fail); err != nil {
			return nil, fmt.Errorf("fail: %w", err)
		}
	case spec.Elicit != nil:
		if step.elicit, err = parseTemplate(spec.Elicit.Message); err != nil {
			return nil, fmt.Errorf("elicit.message: %w", err)
		}
	default:
		if step.action, err = compileAction(spec.Kubernetes, spec.HTTP); err != nil {
			return nil, err
		}
	}
	if spec.When != "" {
		if step.when, err = compileExpression(spec.When); err != nil {
			return nil, fmt.Errorf("when: %w", err)
		}
	}
	if spec.ForEach != "" {
		if step.forEach, err = compileExpression(spec.ForEach); err != nil {
			return nil, fmt.Errorf("forEach: %w", err)
		}
	}
	return step, nil
}

// serverTool converts the definition into the SDK-neutral ServerTool the MCP
// server registers. The inspector derives the RBAC metadata and the
// `requires` compatibility filters, and may be nil.
func (c *compiledTool) serverTool(ctx context.Context, inspector api.ClusterInspector) (api.ServerTool, error) {
	schema, err := c.inputSchema()
	if err != nil {
		return api.ServerTool{}, err
	}
	var discovery api.AggregateDiscovery
	if inspector != nil {
		discovery = inspector.Discovery()
	}
	tool := api.ServerTool{
		Tool: api.Tool{
			Name:        c.name,
			Description: c.spec.Description,
			InputSchema: schema,
			Annotations: api.ToolAnnotations{
				Title:           c.spec.Title,
				ReadOnlyHint:    ptr.To(c.spec.ReadOnly),
				DestructiveHint: ptr.To(c.spec.Destructive),
				IdempotentHint:  ptr.To(c.spec.Idempotent),
				OpenWorldHint:   ptr.To(true),
			},
		},
		RBAC:    c.rbac(ctx, discovery),
		Handler: c.handle,
	}
	if inspector != nil {
		for _, req := range c.spec.Requires {
			gvk := requiredGVK(req)
			tool.TargetCompatibilityFilters = append(tool.TargetCompatibilityFilters, func() bool {
				return api.AnyTargetHasGVK(ctx, inspector, gvk)
			})
		}
	}
	return tool, nil
}

func requiredGVK(req GVKRef) schema.GroupVersionKind {
	gv, _ := schema.ParseGroupVersion(req.APIVersion)
	return gv.WithKind(req.Kind)
}

func (c *compiledTool) inputSchema() (*jsonschema.Schema, error) {
	schema := &jsonschema.Schema{Type: "object"}
	if len(c.spec.InputSchema) == 0 {
		return schema, nil
	}
	if err := json.Unmarshal(c.spec.InputSchema, schema); err != nil {
		return nil, fmt.Errorf("spec.inputSchema is not a valid JSON schema: %w", err)
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	return schema, nil
}

func toolEnv(params api.ToolHandlerParams) runEnv {
	return runEnv{ctx: params, client: params.KubernetesClient, elicitor: params.Elicitor, requireTLS: requireTLS(params.Config)}
}

func requireTLS(cfg *config.Config) func() bool {
	if cfg == nil {
		return nil
	}
	return func() bool { return cfg.RequireTLS.Get() }
}

func (c *compiledTool) handle(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	env := toolEnv(params)
	vars := evalVars{args: params.GetArguments()}
	if vars.args == nil {
		vars.args = map[string]any{}
	}
	var response any
	var binary *binaryResult
	var err error
	if c.action != nil {
		if response, err = c.run(c.action, env, vars); err != nil {
			return api.NewToolCallResult("", fmt.Errorf("%s failed: %w", c.name, err)), nil
		}
		if b, ok := response.(*binaryResult); ok {
			binary, response = b, b.summary()
		}
	} else {
		vars.steps = map[string]any{}
		if response, binary, err = c.runSteps(env, &vars); err != nil {
			return api.NewToolCallResult("", fmt.Errorf("%s failed: %w", c.name, err)), nil
		}
	}
	vars.response = response
	result := response
	if c.outputExpr != nil {
		if result, err = c.outputExpr.eval(vars); err != nil {
			return api.NewToolCallResult("", fmt.Errorf("%s failed: %w", c.name, err)), nil
		}
	} else if c.steps != nil {
		result = vars.steps
	}
	if binary != nil {
		// A binary result becomes an image/blob content block, described by
		// the output text (or a default summary).
		text, ok := result.(string)
		if !ok {
			text = fmt.Sprintf("%s (%d bytes)", binary.mimeType, len(binary.data))
		}
		return api.NewToolCallResultContentBlocks(
			api.NewTextToolContent(text),
			api.NewImageToolContent(binary.data, binary.mimeType),
		), nil
	}
	// Plain text (logs, CSV exports, messages) is returned verbatim.
	if text, ok := result.(string); ok {
		return api.NewToolCallResult(text, nil), nil
	}
	text, err := output.MarshalYaml(result)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("%s failed to serialize result: %w", c.name, err)), nil
	}
	// structuredContent must be a JSON object per the MCP specification.
	if structured, ok := result.(map[string]any); ok {
		return api.NewToolCallResultFull(text, structured, nil), nil
	}
	return api.NewToolCallResult(text, nil), nil
}

// runSteps runs every step in order, recording results in vars.steps. It
// returns the last step's result and, when that result is binary, its bytes.
func (c *compiledTool) runSteps(env runEnv, vars *evalVars) (any, *binaryResult, error) {
	var last any
	var binary *binaryResult
	for _, step := range c.steps {
		result, err := c.runStep(step, env, *vars)
		if err != nil {
			return nil, nil, fmt.Errorf("step %s: %w", step.spec.Name, err)
		}
		binary = nil
		if b, ok := result.(*binaryResult); ok {
			binary, result = b, b.summary()
		}
		vars.steps[step.spec.Name] = result
		last = result
	}
	return last, binary, nil
}

func (c *compiledTool) run(action *compiledAction, env runEnv, vars evalVars) (any, error) {
	if action.kubernetes != nil {
		return c.runKubernetes(action, env, vars)
	}
	return c.runHTTP(action, env, vars)
}

// runStep evaluates when, then runs the step once or once per forEach item.
func (c *compiledTool) runStep(step *compiledStep, env runEnv, vars evalVars) (any, error) {
	if step.when != nil {
		value, err := step.when.eval(vars)
		if err != nil {
			return nil, err
		}
		proceed, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("when must evaluate to a boolean, got %T", value)
		}
		if !proceed {
			return nil, nil
		}
	}
	if step.forEach == nil {
		return c.runOnce(step, env, vars)
	}
	value, err := step.forEach.eval(vars)
	if err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("forEach must evaluate to a list, got %T", value)
	}
	if len(items) > MaxForEach {
		return nil, fmt.Errorf("forEach produced %d items, at most %d are allowed", len(items), MaxForEach)
	}
	results := make([]any, 0, len(items))
	for _, item := range items {
		vars.item = item
		result, err := c.runOnce(step, env, vars)
		if err != nil {
			return nil, err
		}
		if b, ok := result.(*binaryResult); ok {
			result = b.summary()
		}
		results = append(results, result)
	}
	return results, nil
}

func (c *compiledTool) runOnce(step *compiledStep, env runEnv, vars evalVars) (any, error) {
	switch {
	case step.expr != nil:
		return step.expr.eval(vars)
	case step.fail != nil:
		value, err := step.fail.eval(vars)
		if err != nil {
			return nil, err
		}
		if message := scalarString(value); message != "" {
			return nil, errors.New(message)
		}
		return nil, nil
	case step.elicit != nil:
		return c.runElicit(step, env, vars)
	}
	result, err := c.run(step.action, env, vars)
	if err != nil {
		if step.spec.IgnoreError {
			return map[string]any{"error": err.Error()}, nil
		}
		return nil, err
	}
	return result, nil
}

// runElicit asks the user through MCP elicitation. Clients without
// elicitation support yield action "unsupported" so the definition can
// fall back (for example by asking in the prompt text instead).
func (c *compiledTool) runElicit(step *compiledStep, env runEnv, vars evalVars) (any, error) {
	message, err := step.elicit.render(vars)
	if err != nil {
		return nil, err
	}
	if env.elicitor == nil {
		return map[string]any{"action": "unsupported"}, nil
	}
	result, err := env.elicitor.Elicit(env.ctx, &api.ElicitParams{Message: message})
	if err != nil {
		if errors.Is(err, api.ErrElicitationNotSupported) {
			return map[string]any{"action": "unsupported"}, nil
		}
		return nil, err
	}
	content := map[string]any{}
	for key, value := range result.Content {
		content[key] = value
	}
	return map[string]any{"action": result.Action, "content": content}, nil
}

// rbac derives the RBAC metadata from the declaration: the union of the
// requirements of every action. Kubernetes actions are bounded by
// construction (one verb, one resource, argument-derived scope) unless the
// subresource comes from the arguments; the resource plural is resolved
// through discovery when only the kind is declared.
func (c *compiledTool) rbac(ctx context.Context, discovery api.AggregateDiscovery) *api.RBACMetadata {
	actions := []*compiledAction{c.action}
	if c.action == nil {
		actions = actions[:0]
		for _, step := range c.steps {
			if step.action != nil {
				actions = append(actions, step.action)
			}
		}
	}
	var requirements []api.RBACRequirement
	var urls []string
	for _, action := range actions {
		switch {
		case action.http != nil && action.http.Service != nil:
			requirements = append(requirements, api.RBACRequirement{
				Verbs:        []string{proxyVerb(action.http.Method)},
				Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "services", Subresource: "proxy"}},
				Namespace:    &api.RBACNamespace{Name: action.http.Service.Namespace},
				ResourceName: &api.RBACResourceName{Name: action.http.Service.Name},
			})
		case action.http != nil:
			urls = append(urls, action.http.URL)
		default:
			requirement, unbounded := action.kubernetes.rbacRequirement(ctx, discovery)
			if unbounded != "" {
				return api.RBACUnbounded(unbounded)
			}
			requirements = append(requirements, requirement)
		}
	}
	var reason string
	if len(urls) > 0 {
		reason = "performs HTTP requests to " + strings.Join(urls, ", ") + " authenticated with the caller's bearer token"
	}
	if len(requirements) == 0 {
		if reason == "" {
			reason = "does not access the Kubernetes API"
		}
		return &api.RBACMetadata{Version: api.RBACVersionV1Alpha1, None: &api.NoRBAC{Reason: reason}}
	}
	// The Kubernetes requirements are complete, but a composed tool that also
	// calls an external URL must not advertise itself as Kubernetes-only.
	metadata := api.RBACBounded(requirements...)
	metadata.Bounded.Reason = reason
	return metadata
}

// proxyVerb is the RBAC verb the API server checks for a services/proxy
// request: it is derived from the HTTP method of the proxied request.
func proxyVerb(method string) string {
	switch strings.ToUpper(method) {
	case "", http.MethodGet, http.MethodHead:
		return "get"
	case http.MethodPost:
		return "create"
	case http.MethodPut:
		return "update"
	case http.MethodPatch:
		return "patch"
	case http.MethodDelete:
		return "delete"
	}
	return strings.ToLower(method)
}

// rbacRequirement returns the requirement of one Kubernetes action, or the
// reason it cannot be bounded.
func (a *KubernetesAction) rbacRequirement(ctx context.Context, discovery api.AggregateDiscovery) (api.RBACRequirement, string) {
	plural := a.Resource
	if plural == "" {
		var err error
		if plural, err = resolvePlural(ctx, discovery, a.APIVersion, a.Kind); err != nil {
			return api.RBACRequirement{}, fmt.Sprintf("unable to resolve the resource for %s/%s: %v", a.APIVersion, a.Kind, err)
		}
	}
	group := ""
	if i := strings.Index(a.APIVersion, "/"); i >= 0 {
		group = a.APIVersion[:i]
	}
	if sub := a.Subresource; sub != nil && (sub.Argument != "" || sub.Expression != "" || strings.Contains(sub.Value, "{")) {
		return api.RBACRequirement{}, "the subresource is derived from the tool arguments"
	}
	requirement := api.RBACRequirement{
		Verbs:  []string{a.Verb},
		Target: api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: group, Resource: plural, Subresource: rbacSubresource(a.Subresource)}},
	}
	// A CEL-derived namespace or name cannot be mapped to an argument, but the
	// verb and resource are still fixed: advertise them for any namespace /
	// any name rather than giving up on the whole declaration.
	if ns := a.Namespace; ns != nil {
		switch {
		case ns.Argument != "":
			requirement.Namespace = &api.RBACNamespace{Argument: ns.Argument}
		case ns.Expression != "" || ns.All:
			requirement.Namespace = &api.RBACNamespace{AllNamespaces: true}
		case ns.Value != "":
			requirement.Namespace = &api.RBACNamespace{Name: ns.Value}
		}
	}
	if name := a.Name; name != nil {
		switch {
		case name.Argument != "":
			requirement.ResourceName = &api.RBACResourceName{Argument: name.Argument}
		case name.Expression != "":
			// any resource name
		case name.Value != "":
			requirement.ResourceName = &api.RBACResourceName{Name: name.Value}
		}
	}
	return requirement, ""
}

func resolvePlural(ctx context.Context, discovery api.AggregateDiscovery, apiVersion, kind string) (string, error) {
	if discovery == nil {
		return "", fmt.Errorf("no discovery client available")
	}
	resources, err := discovery.ServerResourcesForGroupVersion(ctx, apiVersion).Default()
	if err != nil {
		return "", err
	}
	if resources == nil {
		return "", fmt.Errorf("group version %s not found", apiVersion)
	}
	for _, resource := range resources.APIResources {
		if resource.Kind == kind && !strings.Contains(resource.Name, "/") {
			return resource.Name, nil
		}
	}
	return "", fmt.Errorf("kind %s not found in %s", kind, apiVersion)
}

// rbacSubresource reduces a fixed subresource path to what RBAC sees: its
// first segment (`proxy/stats/summary` needs `nodes/proxy`).
func rbacSubresource(subresource *Ref) string {
	if subresource == nil {
		return ""
	}
	first, _, _ := strings.Cut(strings.Trim(subresource.Value, "/"), "/")
	return first
}

// Check validates a Tool definition the way the toolset does at registration
// time: static validation plus compilation of every CEL expression. It is
// meant for generators and tests that produce Tool manifests.
func Check(tool Tool) error {
	compiled, err := newCompiledTool(tool)
	if err != nil {
		return err
	}
	_, err = compiled.serverTool(context.Background(), nil)
	return err
}

// CheckPrompt validates a Prompt definition the way the toolset does at
// registration time.
func CheckPrompt(prompt Prompt) error {
	_, err := newCompiledPrompt(prompt)
	return err
}
