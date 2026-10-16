package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"k8s.io/client-go/rest"

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
	// serverConfig, when set, is the rest.Config whose credentials url-mode
	// http actions send instead of the caller's (ToolSet identity: server).
	serverConfig func() (*rest.Config, error)
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
	urlExpr   *expression
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
	// outputFullExpr is the projection behind `detail: full`.
	outputFullExpr *expression
	// refExprs holds the compiled Ref expressions keyed by their source.
	refExprs map[string]*expression
	// identity is the ToolSet's: IdentityServer makes http actions send the
	// server's own credentials, obtained through serverClients for the
	// call's target, instead of the caller's.
	identity      string
	serverClients func(ctx context.Context, target string) (api.KubernetesClient, error)
	// variants are the tool's modes as tools of their own, by option name;
	// the call is dispatched to one by the selecting argument.
	variants map[string]*compiledTool
}

// setIdentity records the family's identity on the tool and its variants.
func (c *compiledTool) setIdentity(identity string, serverClients func(ctx context.Context, target string) (api.KubernetesClient, error)) {
	c.identity, c.serverClients = identity, serverClients
	for _, variant := range c.variants {
		variant.setIdentity(identity, serverClients)
	}
}

// missingRequirement returns the first `requires` entry the call's target
// does not serve, as "apiVersion Kind", or "" when every one is served.
func (c *compiledTool) missingRequirement(ctx context.Context) string {
	cluster := clusterFromContext(ctx)
	if cluster.serves == nil {
		return ""
	}
	for _, req := range c.spec.Requires {
		if !cluster.serves(req.APIVersion, req.Kind) {
			return req.APIVersion + " " + req.Kind
		}
	}
	return ""
}

// typeName names a value's type the way an author thinks of it.
func typeName(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "bool"
	case int64, float64:
		return "number"
	case map[string]any:
		return "map"
	case []any:
		return "list"
	}
	return fmt.Sprintf("%T", value)
}

// actsAsServer reports whether http actions send the server's own
// credentials (ToolSet identity: server).
func (c *compiledTool) actsAsServer() bool {
	return c.identity == IdentityServer
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
	if c.spec.OutputFull != "" {
		if c.outputFullExpr, err = compileExpression(c.spec.OutputFull); err != nil {
			return nil, fmt.Errorf("spec.outputFull: %w", err)
		}
	}
	if c.spec.Variants != nil {
		c.variants = map[string]*compiledTool{}
		for _, name := range c.spec.Variants.optionNames() {
			option := c.spec.Variants.Options[name]
			variant, err := newCompiledTool(Tool{Metadata: Metadata{Name: tool.Metadata.Name}, Spec: option.toolSpec(c.spec)})
			if err != nil {
				return nil, fmt.Errorf("spec.variants.options[%s]: %w", name, err)
			}
			variant.name = c.name
			c.variants[name] = variant
		}
		return c, nil
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
	if http != nil && http.URLExpression != "" {
		if a.urlExpr, err = compileExpression(http.URLExpression); err != nil {
			return nil, fmt.Errorf("urlExpression: %w", err)
		}
		// The base URL decides where the caller's bearer token goes. It may
		// come from what the cluster says (a Route host, a CR's spec), never
		// from what the model sends.
		if a.urlExpr.references("args") {
			return nil, fmt.Errorf("urlExpression %q must not reference args: derive the URL from steps instead", http.URLExpression)
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
			Examples:    c.spec.Examples,
			InputSchema: schema,
			Annotations: api.ToolAnnotations{
				Title:           c.spec.Title,
				ReadOnlyHint:    ptr.To(c.spec.ReadOnly),
				DestructiveHint: ptr.To(c.spec.Destructive),
				IdempotentHint:  ptr.To(c.spec.Idempotent),
				OpenWorldHint:   ptr.To(true),
			},
			Meta: c.spec.Meta,
		},
		RBAC:    c.rbac(ctx, discovery),
		Handler: c.handle,
	}
	if inspector != nil {
		// The closures outlive this apply (they run on every tools/list);
		// strip cancellation from the apply-time context so a future
		// request-scoped reload cannot leave them holding a canceled one.
		filters := context.WithoutCancel(ctx)
		for _, req := range c.spec.Requires {
			gvk := requiredGVK(req)
			tool.TargetCompatibilityFilters = append(tool.TargetCompatibilityFilters, func() bool {
				return api.AnyTargetHasGVK(filters, inspector, gvk)
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
	if len(c.spec.InputSchema) > 0 {
		if err := json.Unmarshal(c.spec.InputSchema, schema); err != nil {
			return nil, fmt.Errorf("spec.inputSchema is not a valid JSON schema: %w", err)
		}
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	c.addStandardArguments(schema)
	return schema, nil
}

// standardArguments are the shared argument descriptions of
// api.StandardArguments: an argument declared with one of these names and no
// description gets the description every tool on the server uses, so the
// model learns it once (docs/specs/tools-for-models.md).
var standardArguments = api.StandardArguments

// addStandardArguments adds the arguments the engine handles (limit,
// continue, detail) and fills in the standard description of a declared
// argument that has none. An author's own definition is never replaced.
func (c *compiledTool) addStandardArguments(schema *jsonschema.Schema) {
	if schema.Properties == nil {
		schema.Properties = map[string]*jsonschema.Schema{}
	}
	for name, property := range schema.Properties {
		if property != nil && property.Description == "" {
			if description, ok := standardArguments[name]; ok {
				property.Description = description
			}
		}
	}
	add := func(name string, property *jsonschema.Schema) {
		if _, declared := schema.Properties[name]; !declared {
			schema.Properties[name] = property
		}
	}
	if c.spec.Limit != nil {
		limit := c.spec.Limit.withDefaults()
		add(limit.Argument, &jsonschema.Schema{
			Type: "integer", Minimum: ptr.To(1.0), Maximum: ptr.To(float64(limit.Max)), Default: json.RawMessage(strconv.Itoa(limit.Default)),
			Description: fmt.Sprintf("Maximum number of items to return (default %d, at most %d). The result says how many there were and how to narrow it.", limit.Default, limit.Max),
		})
		if c.paginates() {
			add(ContinueArgument, &jsonschema.Schema{Type: "string", Description: "Continue token from a previous result, to fetch the next page."})
		}
	}
	if v := c.spec.Variants; v != nil {
		description := strings.TrimSpace(v.Description)
		if description == "" {
			description = "What to do."
		}
		var options []string
		for _, name := range v.optionNames() {
			options = append(options, name+": "+strings.TrimSpace(v.Options[name].Description))
		}
		enum := make([]any, 0, len(v.Options))
		for _, name := range v.optionNames() {
			enum = append(enum, name)
		}
		property := &jsonschema.Schema{Type: "string", Enum: enum, Description: description + " " + strings.Join(options, "; ") + "."}
		if v.Default != "" {
			property.Default, _ = json.Marshal(v.Default)
			property.Description += " Defaults to " + v.Default + "."
		}
		add(v.Argument, property)
		if v.Default == "" && !slices.Contains(schema.Required, v.Argument) {
			schema.Required = append(schema.Required, v.Argument)
		}
	}
	if c.outputFullExpr != nil {
		add(DetailArgument, &jsonschema.Schema{
			Type: "string", Enum: []any{"concise", "full"}, Default: json.RawMessage(`"concise"`),
			Description: "concise (default) returns the essentials; full returns everything.",
		})
	}
}

// paginates reports whether the API server pages the result: a single
// kubernetes `list` action with a limit lets the server apply `limit` and
// `continue` instead of reading everything and cutting.
func (c *compiledTool) paginates() bool {
	return c.spec.Limit != nil && c.action != nil && c.action.kubernetes != nil && c.action.kubernetes.Verb == "list"
}

// effectiveLimit is the limit the model asked for, clamped to the declared
// maximum, or the default.
func (c *compiledTool) effectiveLimit(args map[string]any) int {
	limit := c.spec.Limit.withDefaults()
	n := limit.Default
	if value, ok := args[limit.Argument]; ok && value != nil {
		switch v := value.(type) {
		case int64:
			n = int(v)
		case float64:
			n = int(v)
		}
	}
	if n < 1 {
		n = limit.Default
	}
	if n > limit.Max {
		n = limit.Max
	}
	return n
}

// applyLimit cuts a list-shaped result to the effective limit and describes
// the cut: a trailer for the text result and, for a map result, a
// `truncated` entry in the structured one. continueToken is the API server's
// continue token when the result was paginated there (then nothing is cut
// here and the trailer says how to get the next page).
func (c *compiledTool) applyLimit(result any, args map[string]any, continueToken string) (any, string) {
	if c.spec.Limit == nil {
		return result, ""
	}
	limit := c.spec.Limit.withDefaults()
	n := c.effectiveLimit(args)
	var list []any
	var object map[string]any
	field := limit.Field
	switch v := result.(type) {
	case []any:
		list = v
	case map[string]any:
		if field == "" {
			field = singleListKey(v)
		}
		items, ok := v[field].([]any)
		if !ok {
			return result, ""
		}
		object, list = v, items
	default:
		return result, ""
	}
	total := len(list)
	truncated := map[string]any{"returned": int64(total)}
	var trailer string
	switch {
	case continueToken != "":
		truncated[ContinueArgument] = continueToken
		trailer = fmt.Sprintf("showing %d items; more are available: call again with %s=%q", total, ContinueArgument, continueToken)
	case total > n:
		list = list[:n]
		truncated["returned"], truncated["total"] = int64(n), int64(total)
		trailer = fmt.Sprintf("showing %d of %d items", n, total)
		if narrow := c.narrowWith(); len(narrow) > 0 {
			truncated["narrowWith"] = narrow
			trailer += "; narrow with " + strings.Join(narrow, " or ")
			if n < limit.Max {
				trailer += fmt.Sprintf(", or raise %s (at most %d)", limit.Argument, limit.Max)
			}
		} else if n < limit.Max {
			trailer += fmt.Sprintf("; raise %s (at most %d) for more", limit.Argument, limit.Max)
		}
	default:
		return result, ""
	}
	if object != nil {
		object[field] = list
		object["truncated"] = truncated
		return object, trailer
	}
	return list, trailer
}

// singleListKey returns the only key of the map whose value is a list, or
// "" when there is none or more than one: the list a result without a
// declared limit field is understood to hold.
func singleListKey(object map[string]any) string {
	found := ""
	for key, value := range object {
		if _, ok := value.([]any); ok {
			if found != "" {
				return ""
			}
			found = key
		}
	}
	return found
}

// narrowWith is what the trailer tells the model to narrow with: the
// declared NarrowWith, else the selector arguments the tool declares.
func (c *compiledTool) narrowWith() []string {
	if c.spec.Limit != nil && len(c.spec.Limit.NarrowWith) > 0 {
		return c.spec.Limit.NarrowWith
	}
	var declared map[string]any
	if len(c.spec.InputSchema) > 0 {
		var schema struct {
			Properties map[string]any `json:"properties"`
		}
		if json.Unmarshal(c.spec.InputSchema, &schema) == nil {
			declared = schema.Properties
		}
	}
	var out []string
	for _, name := range []string{"labelSelector", "fieldSelector"} {
		if _, ok := declared[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

// continueOf returns the API server's continue token of a list response.
func continueOf(response any) string {
	object, ok := response.(map[string]any)
	if !ok {
		return ""
	}
	token, _, _ := unstructured.NestedString(object, "metadata", "continue")
	return token
}

// defaultProjection is what a kubernetes `list` returns without an `output`:
// the essentials of every item, never the raw objects.
func defaultProjection(response any) any {
	object, ok := response.(map[string]any)
	if !ok {
		return response
	}
	items, ok := object["items"].([]any)
	if !ok {
		return response
	}
	kind, _ := object["kind"].(string)
	kind = strings.TrimSuffix(kind, "List")
	out := make([]any, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		summary := map[string]any{}
		if kind != "" {
			summary["kind"] = kind
		}
		if name, _, _ := unstructured.NestedString(item, "metadata", "name"); name != "" {
			summary["name"] = name
		}
		if namespace, _, _ := unstructured.NestedString(item, "metadata", "namespace"); namespace != "" {
			summary["namespace"] = namespace
		}
		if phase, _, _ := unstructured.NestedString(item, "status", "phase"); phase != "" {
			summary["phase"] = phase
		}
		if conditions, _, _ := unstructured.NestedSlice(item, "status", "conditions"); len(conditions) > 0 {
			for _, raw := range conditions {
				condition, _ := raw.(map[string]any)
				if condition["type"] == "Ready" {
					summary["ready"] = condition["status"] == "True"
				}
			}
			if _, has := summary["ready"]; !has {
				for _, raw := range conditions {
					condition, _ := raw.(map[string]any)
					if condition["status"] == "True" {
						summary["condition"] = condition["type"]
						break
					}
				}
			}
		}
		out = append(out, summary)
	}
	return out
}

func toolEnv(params api.ToolHandlerParams) runEnv {
	return runEnv{ctx: params, client: params.KubernetesClient, elicitor: params.Elicitor, requireTLS: requireTLS(params.Config)}
}

// serverConfigFor returns the credentials source of a tool that acts as
// the server, for the call's target; nil for a tool that acts as the caller.
func (c *compiledTool) serverConfigFor(ctx context.Context, target string) func() (*rest.Config, error) {
	if !c.actsAsServer() {
		return nil
	}
	return func() (*rest.Config, error) {
		if c.serverClients == nil {
			return nil, errors.New("this server does not offer its own credentials to tools")
		}
		client, err := c.serverClients(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("server credentials: %w", err)
		}
		return client.RESTConfig(), nil
	}
}

func requireTLS(cfg *config.Config) func() bool {
	if cfg == nil {
		return nil
	}
	return func() bool { return cfg.RequireTLS.Get() }
}

func (c *compiledTool) handle(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	if c.variants != nil {
		selected := argString(params.GetArguments(), c.spec.Variants.Argument)
		if selected == "" {
			selected = c.spec.Variants.Default
		}
		variant, ok := c.variants[selected]
		if !ok {
			return api.NewToolCallResult("", fmt.Errorf("%s: %s must be one of %s", c.name, c.spec.Variants.Argument, strings.Join(c.spec.Variants.optionNames(), ", "))), nil
		}
		return variant.handle(params)
	}
	timeout, _ := parseTimeout("spec.timeout", c.spec.Timeout)
	ctx, cancel := context.WithTimeout(params, timeout)
	defer cancel()
	env := toolEnv(params)
	env.ctx = ctx
	env.serverConfig = c.serverConfigFor(ctx, params.Target)
	vars := evalVars{ctx: withServes(ctx, env.client), args: params.GetArguments()}
	if vars.args == nil {
		vars.args = map[string]any{}
	}
	// Whole-number arguments become int64, as JSON responses and CEL
	// results are, so int(args.limit) and args.limit * 2 both work.
	normalizeNumbers(vars.args)
	// `requires` hides the tool while no target serves the kind; the call may
	// still name another target, so check the one it runs against.
	if missing := c.missingRequirement(vars.ctx); missing != "" {
		return api.NewToolCallResult("", fmt.Errorf("%s needs the %s API, which this cluster does not serve", c.name, missing)), nil
	}
	failed := func(err error) (*api.ToolCallResult, error) {
		if ctx.Err() == context.DeadlineExceeded {
			return api.NewToolCallResult("", fmt.Errorf("%s timed out after %s", c.name, timeout)), nil
		}
		return api.NewToolCallResult("", fmt.Errorf("%s failed: %w", c.name, err)), nil
	}
	var response any
	var binary *binaryResult
	var err error
	if c.action != nil {
		if response, err = c.run(c.action, env, vars); err != nil {
			return failed(err)
		}
		if b, ok := response.(*binaryResult); ok {
			binary, response = b, b.summary()
		}
	} else {
		vars.steps = map[string]any{}
		if response, binary, err = c.runSteps(env, &vars); err != nil {
			return failed(err)
		}
	}
	vars.response = response
	result := response
	projection := c.outputExpr
	if c.outputFullExpr != nil && argString(vars.args, DetailArgument) == "full" {
		projection = c.outputFullExpr
	}
	switch {
	case projection != nil:
		if result, err = projection.eval(vars); err != nil {
			return api.NewToolCallResult("", fmt.Errorf("%s failed: %w", c.name, err)), nil
		}
	case c.steps != nil:
		result = vars.steps
	case c.action != nil && c.action.kubernetes != nil && c.action.kubernetes.Verb == "list":
		result = defaultProjection(response)
	}
	result, trailer := c.applyLimit(result, vars.args, continueOf(response))
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
	if trailer != "" {
		text = strings.TrimRight(text, "\n") + "\n# " + trailer + "\n"
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
			return nil, fmt.Errorf("when must evaluate to a boolean, got %s", typeName(value))
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
		return nil, fmt.Errorf("forEach must evaluate to a list, got %s", typeName(value))
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
	// item belongs to this step alone: a later expression referencing it
	// must fail rather than silently see the last element.
	vars.item = nil
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
		// A fail step is a string: a boolean such as `args.n > 3` would
		// otherwise fail every call with the message "false".
		switch message := value.(type) {
		case nil:
			return nil, nil
		case string:
			if message != "" {
				return nil, errors.New(message)
			}
			return nil, nil
		default:
			return nil, fmt.Errorf("fail must evaluate to a string (empty to pass), got %s", typeName(value))
		}
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

// actions returns every action the tool may perform: its own, its steps',
// or those of all its variants.
func (c *compiledTool) actions() []*compiledAction {
	if c.action != nil {
		return []*compiledAction{c.action}
	}
	var actions []*compiledAction
	for _, step := range c.steps {
		if step.action != nil {
			actions = append(actions, step.action)
		}
	}
	if c.spec.Variants != nil {
		for _, name := range c.spec.Variants.optionNames() {
			actions = append(actions, c.variants[name].actions()...)
		}
	}
	return actions
}

// rbac derives the RBAC metadata from the declaration: the union of the
// requirements of every action. Kubernetes actions are bounded by
// construction (one verb, one resource, argument-derived scope) unless the
// subresource comes from the arguments; the resource plural is resolved
// through discovery when only the kind is declared.
func (c *compiledTool) rbac(ctx context.Context, discovery api.AggregateDiscovery) *api.RBACMetadata {
	actions := c.actions()
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
		case action.http != nil && action.http.URLExpression != "":
			urls = append(urls, "the URL computed by "+action.http.URLExpression)
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
		credentials := "the caller's bearer token"
		if c.actsAsServer() {
			credentials = "the server's own credentials (ToolSet identity: server)"
		}
		reason = "performs HTTP requests to " + strings.Join(urls, ", ") + " authenticated with " + credentials
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
		case ns.Expression != "" || ns.All:
			// An omitted argument falls through to every namespace (all: true)
			// or to whatever the expression yields: advertise the superset.
			requirement.Namespace = &api.RBACNamespace{AllNamespaces: true}
		case ns.Argument != "":
			requirement.Namespace = &api.RBACNamespace{Argument: ns.Argument}
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
