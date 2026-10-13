package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

// compiledTool is a validated Tool definition with its CEL expressions
// compiled once at registration time, so a broken definition is rejected
// (and logged) when the toolset is applied rather than on first call.
type compiledTool struct {
	name       string
	spec       ToolSpec
	outputExpr *expression
	bodyExpr   *expression
	queryExpr  *expression
	// refExprs holds the compiled Ref expressions keyed by their source.
	refExprs map[string]*expression
}

func newCompiledTool(tool Tool) (*compiledTool, error) {
	if err := tool.Validate(); err != nil {
		return nil, err
	}
	c := &compiledTool{name: tool.ToolName(), spec: tool.Spec, refExprs: map[string]*expression{}}
	for _, ref := range c.spec.refs() {
		if ref == nil || ref.Expression == "" {
			continue
		}
		if _, done := c.refExprs[ref.Expression]; done {
			continue
		}
		expr, err := compileExpression(ref.Expression)
		if err != nil {
			return nil, err
		}
		c.refExprs[ref.Expression] = expr
	}
	if c.spec.Output != "" {
		expr, err := compileExpression(c.spec.Output)
		if err != nil {
			return nil, fmt.Errorf("spec.output: %w", err)
		}
		c.outputExpr = expr
	}
	if source := c.queryExpression(); source != "" {
		expr, err := compileExpression(source)
		if err != nil {
			return nil, fmt.Errorf("queryExpression: %w", err)
		}
		c.queryExpr = expr
	}
	if body := c.body(); body != nil && body.Expression != "" {
		expr, err := compileExpression(body.Expression)
		if err != nil {
			return nil, fmt.Errorf("body.expression: %w", err)
		}
		c.bodyExpr = expr
	}
	return c, nil
}

func (c *compiledTool) queryExpression() string {
	switch {
	case c.spec.Kubernetes != nil:
		return c.spec.Kubernetes.QueryExpression
	case c.spec.HTTP != nil:
		return c.spec.HTTP.QueryExpression
	}
	return ""
}

func (c *compiledTool) body() *Body {
	switch {
	case c.spec.Kubernetes != nil:
		return c.spec.Kubernetes.Body
	case c.spec.HTTP != nil:
		return c.spec.HTTP.Body
	}
	return nil
}

// serverTool converts the definition into the SDK-neutral ServerTool the MCP
// server registers. discovery is used to derive the RBAC metadata and may be
// nil.
func (c *compiledTool) serverTool(ctx context.Context, discovery api.AggregateDiscovery) (api.ServerTool, error) {
	schema, err := c.inputSchema()
	if err != nil {
		return api.ServerTool{}, err
	}
	return api.ServerTool{
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
	}, nil
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

func (c *compiledTool) handle(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	args := params.GetArguments()
	if args == nil {
		args = map[string]any{}
	}
	var response any
	var err error
	if c.spec.Kubernetes != nil {
		response, err = c.runKubernetes(params, args)
	} else {
		response, err = c.runHTTP(params, args)
	}
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("%s failed: %w", c.name, err)), nil
	}
	result := response
	if c.outputExpr != nil {
		if result, err = c.outputExpr.eval(args, response); err != nil {
			return api.NewToolCallResult("", fmt.Errorf("%s failed: %w", c.name, err)), nil
		}
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

// rbac derives the RBAC metadata from the declaration. Kubernetes actions are
// bounded by construction (one verb, one resource, argument-derived scope);
// the resource plural is resolved through discovery and the declaration falls
// back to unbounded when it cannot be.
func (c *compiledTool) rbac(ctx context.Context, discovery api.AggregateDiscovery) *api.RBACMetadata {
	if a := c.spec.HTTP; a != nil {
		if a.Service != nil {
			return api.RBACBounded(api.RBACRequirement{
				Verbs:        []string{"get", "create"},
				Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "services", Subresource: "proxy"}},
				Namespace:    &api.RBACNamespace{Name: a.Service.Namespace},
				ResourceName: &api.RBACResourceName{Name: a.Service.Name},
			})
		}
		return &api.RBACMetadata{Version: api.RBACVersionV1Alpha1, None: &api.NoRBAC{
			Reason: "performs an HTTP request to " + a.URL + " authenticated with the caller's bearer token",
		}}
	}
	a := c.spec.Kubernetes
	plural := a.Resource
	if plural == "" {
		var err error
		if plural, err = resolvePlural(ctx, discovery, a.APIVersion, a.Kind); err != nil {
			return api.RBACUnbounded(fmt.Sprintf("unable to resolve the resource for %s/%s: %v", a.APIVersion, a.Kind, err))
		}
	}
	group := ""
	if i := strings.Index(a.APIVersion, "/"); i >= 0 {
		group = a.APIVersion[:i]
	}
	requirement := api.RBACRequirement{
		Verbs:  []string{a.Verb},
		Target: api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: group, Resource: plural, Subresource: rbacSubresource(a.Subresource)}},
	}
	if sub := a.Subresource; sub != nil && (sub.Argument != "" || sub.Expression != "" || strings.Contains(sub.Value, "{")) {
		return api.RBACUnbounded("the subresource is derived from the tool arguments")
	}
	if ns := a.Namespace; ns != nil {
		switch {
		case ns.Argument != "":
			requirement.Namespace = &api.RBACNamespace{Argument: ns.Argument}
		case ns.Expression != "":
			return api.RBACUnbounded("the namespace is derived from a CEL expression")
		case ns.Value != "":
			requirement.Namespace = &api.RBACNamespace{Name: ns.Value}
		case ns.All:
			requirement.Namespace = &api.RBACNamespace{AllNamespaces: true}
		}
	}
	if name := a.Name; name != nil {
		switch {
		case name.Argument != "":
			requirement.ResourceName = &api.RBACResourceName{Argument: name.Argument}
		case name.Expression != "":
			return api.RBACUnbounded("the resource name is derived from a CEL expression")
		case name.Value != "":
			requirement.ResourceName = &api.RBACResourceName{Name: name.Value}
		}
	}
	return api.RBACBounded(requirement)
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
