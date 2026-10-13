package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type DynamicSuite struct {
	suite.Suite
}

func (s *DynamicSuite) TestToolName() {
	s.Run("defaults to metadata.name with dashes and dots replaced", func() {
		tool := Tool{Metadata: Metadata{Name: "kiali-mesh.status"}}
		s.Equal("kiali_mesh_status", tool.ToolName())
	})
	s.Run("spec.name takes precedence", func() {
		tool := Tool{Metadata: Metadata{Name: "ignored"}, Spec: ToolSpec{Name: "explicit_name"}}
		s.Equal("explicit_name", tool.ToolName())
	})
}

func (s *DynamicSuite) TestValidate() {
	valid := func() Tool {
		return Tool{Metadata: Metadata{Name: "pods-get"}, Spec: ToolSpec{
			Description: "Get a pod",
			Kubernetes:  &KubernetesAction{Verb: "get", APIVersion: "v1", Kind: "Pod"},
		}}
	}
	s.Run("accepts a kubernetes action", func() {
		tool := valid()
		s.NoError(tool.Validate())
	})
	s.Run("accepts an http action with a url", func() {
		tool := valid()
		tool.Spec.Kubernetes = nil
		tool.Spec.HTTP = &HTTPAction{URL: "https://kiali.example.com"}
		s.NoError(tool.Validate())
	})
	s.Run("rejects a missing description", func() {
		tool := valid()
		tool.Spec.Description = " "
		s.ErrorContains(tool.Validate(), "spec.description is required")
	})
	s.Run("rejects an invalid tool name", func() {
		tool := valid()
		tool.Spec.Name = "Not Valid"
		s.ErrorContains(tool.Validate(), "tool name")
	})
	s.Run("rejects both actions", func() {
		tool := valid()
		tool.Spec.HTTP = &HTTPAction{URL: "https://example.com"}
		s.ErrorContains(tool.Validate(), "mutually exclusive")
	})
	s.Run("rejects no action", func() {
		tool := valid()
		tool.Spec.Kubernetes = nil
		s.ErrorContains(tool.Validate(), "one of spec.kubernetes, spec.http or spec.steps is required")
	})
	s.Run("accepts an explicit resource instead of a kind", func() {
		tool := valid()
		tool.Spec.Kubernetes = &KubernetesAction{Verb: "update", APIVersion: "subresources.kubevirt.io/v1", Resource: "virtualmachines", Subresource: &Ref{Argument: "action"}}
		s.NoError(tool.Validate())
	})
	s.Run("rejects both kind and resource", func() {
		tool := valid()
		tool.Spec.Kubernetes.Resource = "pods"
		s.ErrorContains(tool.Validate(), "exactly one of spec.kubernetes.kind or spec.kubernetes.resource")
	})
	s.Run("rejects an unknown verb", func() {
		tool := valid()
		tool.Spec.Kubernetes.Verb = "watch"
		s.ErrorContains(tool.Validate(), `verb "watch"`)
	})
	s.Run("rejects http with both url and service", func() {
		tool := valid()
		tool.Spec.Kubernetes = nil
		tool.Spec.HTTP = &HTTPAction{URL: "https://example.com", Service: &ServiceRef{Namespace: "a", Name: "b"}}
		s.ErrorContains(tool.Validate(), "exactly one of spec.http.url or spec.http.service")
	})
	s.Run("rejects a relative http url", func() {
		tool := valid()
		tool.Spec.Kubernetes = nil
		tool.Spec.HTTP = &HTTPAction{URL: "kiali.example.com"}
		s.ErrorContains(tool.Validate(), "absolute http(s) URL")
	})
	s.Run("rejects a body with several sources", func() {
		tool := valid()
		tool.Spec.Kubernetes.Body = &Body{Argument: "manifest", Expression: "args"}
		s.ErrorContains(tool.Validate(), "exactly one of argument, value or expression")
	})
}

func (s *DynamicSuite) TestCompileExpression() {
	s.Run("rejects invalid syntax at compile time", func() {
		_, err := compileExpression("response.items.map(")
		s.ErrorContains(err, "invalid CEL expression")
	})
	s.Run("rejects unknown variables at compile time", func() {
		_, err := compileExpression("nope.items")
		s.ErrorContains(err, "invalid CEL expression")
	})
	s.Run("projects a list response into plain JSON values", func() {
		expr, err := compileExpression("response.items.map(i, {'name': i.metadata.name, 'replicas': i.spec.replicas})")
		s.Require().NoError(err)
		response := map[string]any{"items": []any{
			map[string]any{"metadata": map[string]any{"name": "a"}, "spec": map[string]any{"replicas": float64(2)}},
		}}
		out, err := expr.eval(evalVars{response: response})
		s.Require().NoError(err)
		raw, _ := json.Marshal(out)
		s.JSONEq(`[{"name":"a","replicas":2}]`, string(raw))
	})
	s.Run("exposes the tool arguments", func() {
		expr, err := compileExpression("{'ns': args.namespace, 'all': args}")
		s.Require().NoError(err)
		out, err := expr.eval(evalVars{args: map[string]any{"namespace": "default"}})
		s.Require().NoError(err)
		raw, _ := json.Marshal(out)
		s.JSONEq(`{"ns":"default","all":{"namespace":"default"}}`, string(raw))
	})
	s.Run("reports runtime errors", func() {
		expr, err := compileExpression("response.missing.field")
		s.Require().NoError(err)
		_, err = expr.eval(evalVars{response: map[string]any{}})
		s.ErrorContains(err, "failed")
	})
}

func (s *DynamicSuite) TestExpandPath() {
	s.Run("substitutes and escapes arguments", func() {
		path, err := expandPath("/namespaces/{namespace}/pods/{name}", map[string]any{"namespace": "a b", "name": "x/y"})
		s.Require().NoError(err)
		s.Equal("/namespaces/a%20b/pods/x%2Fy", path)
	})
	s.Run("fails on missing arguments", func() {
		_, err := expandPath("/namespaces/{namespace}/pods/{name}", map[string]any{"namespace": "a"})
		s.ErrorContains(err, "name parameter required")
	})
	s.Run("leaves literal paths untouched", func() {
		path, err := expandPath("/api/mcp/get_mesh_status", nil)
		s.Require().NoError(err)
		s.Equal("/api/mcp/get_mesh_status", path)
	})
}

func (s *DynamicSuite) TestResolveBody() {
	tool := &compiledTool{}
	s.Run("nil body yields an empty object", func() {
		body, err := tool.resolveBody(nil, nil, evalVars{})
		s.Require().NoError(err)
		s.Equal("{}", string(body))
	})
	s.Run("string arguments are parsed as YAML", func() {
		body, err := tool.resolveBody(&Body{Argument: "manifest"}, nil, evalVars{args: map[string]any{"manifest": "apiVersion: v1\nkind: ConfigMap\n"}})
		s.Require().NoError(err)
		s.JSONEq(`{"apiVersion":"v1","kind":"ConfigMap"}`, string(body))
	})
	s.Run("object arguments are serialized", func() {
		body, err := tool.resolveBody(&Body{Argument: "manifest"}, nil, evalVars{args: map[string]any{"manifest": map[string]any{"kind": "ConfigMap"}}})
		s.Require().NoError(err)
		s.JSONEq(`{"kind":"ConfigMap"}`, string(body))
	})
	s.Run("missing arguments are reported", func() {
		_, err := tool.resolveBody(&Body{Argument: "manifest"}, nil, evalVars{})
		s.ErrorContains(err, "manifest parameter required")
	})
	s.Run("fixed values are passed through", func() {
		body, err := tool.resolveBody(&Body{Value: json.RawMessage(`{"spec":{"replicas":3}}`)}, nil, evalVars{})
		s.Require().NoError(err)
		s.JSONEq(`{"spec":{"replicas":3}}`, string(body))
	})
	s.Run("expressions are evaluated over the arguments", func() {
		expr, err := compileExpression("{'spec': {'replicas': int(args.replicas)}}")
		s.Require().NoError(err)
		tool := &compiledTool{}
		body, err := tool.resolveBody(&Body{Expression: expr.source}, expr, evalVars{args: map[string]any{"replicas": float64(3)}})
		s.Require().NoError(err)
		s.JSONEq(`{"spec":{"replicas":3}}`, string(body))
	})
}

func (s *DynamicSuite) TestResolveNamespace() {
	tool := &compiledTool{refExprs: map[string]*expression{}}
	s.Run("argument wins over value", func() {
		ns, all, err := tool.resolveNamespace(&NamespaceRef{Argument: "namespace", Value: "fixed"}, evalVars{args: map[string]any{"namespace": "arg"}})
		s.Require().NoError(err)
		s.Equal("arg", ns)
		s.False(all)
	})
	s.Run("falls back to the fixed value", func() {
		ns, all, err := tool.resolveNamespace(&NamespaceRef{Argument: "namespace", Value: "fixed"}, evalVars{})
		s.Require().NoError(err)
		s.Equal("fixed", ns)
		s.False(all)
	})
	s.Run("falls back to all namespaces when requested", func() {
		ns, all, err := tool.resolveNamespace(&NamespaceRef{Argument: "namespace", All: true}, evalVars{})
		s.Require().NoError(err)
		s.Equal("", ns)
		s.True(all)
	})
	s.Run("nil means the default namespace", func() {
		ns, all, err := tool.resolveNamespace(nil, evalVars{})
		s.Require().NoError(err)
		s.Equal("", ns)
		s.False(all)
	})
}

func (s *DynamicSuite) TestResolveRef() {
	spec := ToolSpec{Kubernetes: &KubernetesAction{
		Verb: "get", APIVersion: "v1", Kind: "Pod",
		Subresource: &Ref{Expression: "{'os': 'guestosinfo', 'users': 'userlist'}[args.info_type]"},
		Name:        &Ref{Value: "prefix-{name}"},
	}}
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", Kubernetes: spec.Kubernetes}})
	s.Require().NoError(err)
	s.Run("expressions over the arguments", func() {
		value, err := tool.resolveRef(spec.Kubernetes.Subresource, evalVars{args: map[string]any{"info_type": "users"}})
		s.Require().NoError(err)
		s.Equal("userlist", value)
	})
	s.Run("expression errors are reported", func() {
		_, err := tool.resolveRef(spec.Kubernetes.Subresource, evalVars{args: map[string]any{"info_type": "all"}})
		s.ErrorContains(err, "no such key")
	})
	s.Run("fixed values expand placeholders", func() {
		value, err := tool.resolveRef(spec.Kubernetes.Name, evalVars{args: map[string]any{"name": "x"}})
		s.Require().NoError(err)
		s.Equal("prefix-x", value)
	})
	s.Run("a plain YAML string is a fixed value", func() {
		var ref Ref
		s.Require().NoError(json.Unmarshal([]byte(`"proxy/stats/summary"`), &ref))
		s.Equal(Ref{Value: "proxy/stats/summary"}, ref)
		out, err := json.Marshal(ref)
		s.Require().NoError(err)
		s.Equal(`"proxy/stats/summary"`, string(out))
	})
}

func (s *DynamicSuite) TestCELExtensions() {
	s.Run("merge overrides left with right", func() {
		expr, err := compileExpression("merge(args, {'mcp_mode': 'true', 'b': 2})")
		s.Require().NoError(err)
		out, err := expr.eval(evalVars{args: map[string]any{"a": "1", "b": float64(1)}})
		s.Require().NoError(err)
		raw, _ := json.Marshal(out)
		s.JSONEq(`{"a":"1","b":2,"mcp_mode":"true"}`, string(raw))
	})
	s.Run("transformMapEntry renames and drops keys", func() {
		expr, err := compileExpression("args.transformMapEntry(k, v, k == 'meshCluster' ? {'clusterName': v} : (k == 'drop' ? {} : {k: v}))")
		s.Require().NoError(err)
		out, err := expr.eval(evalVars{args: map[string]any{"meshCluster": "east", "drop": "x", "keep": "y"}})
		s.Require().NoError(err)
		raw, _ := json.Marshal(out)
		s.JSONEq(`{"clusterName":"east","keep":"y"}`, string(raw))
	})
	s.Run("now is the current time", func() {
		expr, err := compileExpression("{'start': int(now) - 300, 'end': int(now)}")
		s.Require().NoError(err)
		out, err := expr.evalStringMap(evalVars{})
		s.Require().NoError(err)
		s.Regexp(`^\d{10}$`, out["end"])
		s.Regexp(`^\d{10}$`, out["start"])
	})
	s.Run("evalStringMap drops empty values and formats whole numbers", func() {
		expr, err := compileExpression("{'a': 1.0, 'b': 'x', 'c': '', 'd': null, 'e': true, 'f': 1.5}")
		s.Require().NoError(err)
		out, err := expr.evalStringMap(evalVars{})
		s.Require().NoError(err)
		s.Equal(map[string]string{"a": "1", "b": "x", "e": "true", "f": "1.5"}, out)
	})
}

func (s *DynamicSuite) TestStepsValidation() {
	base := func(steps ...Step) *Tool {
		return &Tool{Metadata: Metadata{Name: "composed"}, Spec: ToolSpec{Description: "d", Steps: steps}}
	}
	get := &KubernetesAction{Verb: "get", APIVersion: "v1", Kind: "Pod", Name: &Ref{Argument: "name"}}
	s.Run("accepts steps", func() {
		s.NoError(base(Step{Name: "pod", Kubernetes: get}, Step{Name: "names", Expression: "[steps.pod.metadata.name]"}).Validate())
	})
	s.Run("rejects steps next to a single action", func() {
		tool := base(Step{Name: "pod", Kubernetes: get})
		tool.Spec.Kubernetes = get
		s.ErrorContains(tool.Validate(), "mutually exclusive")
	})
	s.Run("rejects duplicate and invalid names", func() {
		s.ErrorContains(base(Step{Name: "a", Kubernetes: get}, Step{Name: "a", Kubernetes: get}).Validate(), "duplicated")
		s.ErrorContains(base(Step{Name: "not valid", Kubernetes: get}).Validate(), "must match")
	})
	s.Run("rejects a step without exactly one of kubernetes, http or expression", func() {
		s.ErrorContains(base(Step{Name: "a"}).Validate(), "exactly one of kubernetes, http, expression, fail or elicit")
		s.ErrorContains(base(Step{Name: "a", Kubernetes: get, Expression: "1"}).Validate(), "exactly one of kubernetes, http, expression, fail or elicit")
	})
	s.Run("rejects too many steps", func() {
		steps := make([]Step, MaxSteps+1)
		for i := range steps {
			steps[i] = Step{Name: fmt.Sprintf("s%d", i), Expression: "1"}
		}
		s.ErrorContains(base(steps...).Validate(), "at most")
	})
	s.Run("compiles every step expression", func() {
		_, err := newCompiledTool(*base(Step{Name: "a", Expression: "1", When: "args.x ==", ForEach: "[1]"}))
		s.ErrorContains(err, "spec.steps[0] (a): when")
	})
}

func (s *DynamicSuite) TestStepsExecution() {
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "composed"}, Spec: ToolSpec{
		Description: "d",
		Steps: []Step{
			{Name: "doubled", Expression: "args.n == 'explode' ? 0 : int(args.n) * 2"},
			{Name: "skipped", Expression: "'never'", When: "steps.doubled > 100"},
			{Name: "squares", Expression: "item * item", ForEach: "range(steps.doubled)"},
			{Name: "tooMany", Expression: "item", ForEach: "range(1000)", When: "args.n == 'explode'"},
		},
		Output: "{'doubled': steps.doubled, 'skipped': steps.skipped, 'squares': steps.squares, 'last': response}",
	}})
	s.Require().NoError(err)
	run := func(args map[string]any) (map[string]any, error) {
		vars := evalVars{args: args, steps: map[string]any{}}
		var last any
		for _, step := range tool.steps {
			result, err := tool.runStep(step, runEnv{}, vars)
			if err != nil {
				return nil, err
			}
			vars.steps[step.spec.Name] = result
			last = result
		}
		vars.response = last
		out, err := tool.outputExpr.eval(vars)
		if err != nil {
			return nil, err
		}
		return out.(map[string]any), nil
	}
	s.Run("steps see previous results, when skips, forEach fans out", func() {
		out, err := run(map[string]any{"n": float64(3)})
		s.Require().NoError(err)
		raw, _ := json.Marshal(out)
		s.JSONEq(`{"doubled":6,"skipped":null,"squares":[0,1,4,9,16,25],"last":null}`, string(raw))
	})
	s.Run("forEach fan-out is bounded", func() {
		_, err := run(map[string]any{"n": "explode"})
		s.ErrorContains(err, "at most 64")
	})
}

type fakeElicitor struct {
	action   string
	messages []string
	err      error
}

func (f *fakeElicitor) Elicit(_ context.Context, params *api.ElicitParams) (*api.ElicitResult, error) {
	f.messages = append(f.messages, params.Message)
	if f.err != nil {
		return nil, f.err
	}
	return &api.ElicitResult{Action: f.action, Content: map[string]any{"ok": true}}, nil
}

func (s *DynamicSuite) TestFailAndElicitSteps() {
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "guarded"}, Spec: ToolSpec{
		Description: "d",
		Steps: []Step{
			{Name: "check", Fail: "!has(args.url) || !args.url.startsWith('https://') ? 'url must be https' : ''"},
			{Name: "consent", Elicit: &ElicitStep{Message: "Proceed with ${args.url}?"}},
		},
	}})
	s.Require().NoError(err)
	run := func(elicitor api.Elicitor, args map[string]any) (map[string]any, error) {
		vars := evalVars{args: args, steps: map[string]any{}}
		_, _, err := tool.runSteps(runEnv{ctx: context.Background(), elicitor: elicitor}, &vars)
		return vars.steps, err
	}
	s.Run("fail stops the tool with the message", func() {
		_, err := run(nil, map[string]any{"url": "http://x"})
		s.EqualError(err, "step check: url must be https")
	})
	s.Run("elicit renders the message and records the answer", func() {
		elicitor := &fakeElicitor{action: "accept"}
		steps, err := run(elicitor, map[string]any{"url": "https://x"})
		s.Require().NoError(err)
		s.Equal([]string{"Proceed with https://x?"}, elicitor.messages)
		s.Equal(map[string]any{"action": "accept", "content": map[string]any{"ok": true}}, steps["consent"])
	})
	s.Run("elicit reports unsupported clients instead of failing", func() {
		steps, err := run(nil, map[string]any{"url": "https://x"})
		s.Require().NoError(err)
		s.Equal(map[string]any{"action": "unsupported"}, steps["consent"])
		steps, err = run(&fakeElicitor{err: api.ErrElicitationNotSupported}, map[string]any{"url": "https://x"})
		s.Require().NoError(err)
		s.Equal(map[string]any{"action": "unsupported"}, steps["consent"])
	})
}

func (s *DynamicSuite) TestTemplates() {
	s.Run("renders strings, scalars, null and YAML for structures", func() {
		t, err := parseTemplate("name=${args.name} n=${args.n} none=${args.missing}\n${yaml({'a': [1, 2]})}end")
		s.Require().NoError(err)
		out, err := t.render(evalVars{args: map[string]any{"name": "x", "n": float64(2), "missing": nil}})
		s.Require().NoError(err)
		s.Equal("name=x n=2 none=\na:\n- 1\n- 2\nend", out)
	})
	s.Run("placeholders may contain braces", func() {
		t, err := parseTemplate("${{'k': args.v}.k}!")
		s.Require().NoError(err)
		out, err := t.render(evalVars{args: map[string]any{"v": "ok"}})
		s.Require().NoError(err)
		s.Equal("ok!", out)
	})
	s.Run("rejects unterminated placeholders and bad expressions", func() {
		_, err := parseTemplate("${args.x")
		s.ErrorContains(err, "unterminated")
		_, err = parseTemplate("${args.x ==}")
		s.ErrorContains(err, "invalid CEL expression")
	})
	s.Run("json and redact functions", func() {
		expr, err := compileExpression("json({'a': 1}) + ' ' + redact('token=abc')")
		s.Require().NoError(err)
		out, err := expr.eval(evalVars{})
		s.Require().NoError(err)
		s.Equal(`{"a":1} token=<REDACTED>`, out)
	})
}

func (s *DynamicSuite) TestPromptValidation() {
	valid := func() *Prompt {
		return &Prompt{Metadata: Metadata{Name: "vm-troubleshoot"}, Spec: PromptSpec{
			Description: "d",
			Arguments:   []PromptArgument{{Name: "name", Required: true}},
			Steps:       []Step{{Name: "vm", Kubernetes: &KubernetesAction{Verb: "get", APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine", Name: &Ref{Argument: "name"}}}},
			Messages:    []PromptMessage{{Role: "user", Text: "VM: ${yaml(steps.vm)}"}, {Role: "assistant", Text: "ok", When: "has(steps.vm.status)"}},
		}}
	}
	s.Run("accepts a prompt with steps and templates", func() {
		s.NoError(CheckPrompt(*valid()))
		s.Equal("vm-troubleshoot", valid().PromptName())
	})
	s.Run("rejects missing messages, bad roles and duplicate arguments", func() {
		p := valid()
		p.Spec.Messages = nil
		s.ErrorContains(p.Validate(), "at least one message")
		p = valid()
		p.Spec.Messages[0].Role = "system"
		s.ErrorContains(p.Validate(), "must be user or assistant")
		p = valid()
		p.Spec.Arguments = append(p.Spec.Arguments, PromptArgument{Name: "name"})
		s.ErrorContains(p.Validate(), "duplicated")
	})
	s.Run("rejects bad templates at compile time", func() {
		p := valid()
		p.Spec.Messages[0].Text = "${steps.vm"
		s.ErrorContains(CheckPrompt(*p), "spec.messages[0].text")
	})
}

func (s *DynamicSuite) TestResponseAndRequiresValidation() {
	tool := Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d",
		Kubernetes: &KubernetesAction{Verb: "get", APIVersion: "v1", Kind: "Pod", Name: &Ref{Argument: "n"}, Response: "bytes"}}}
	s.ErrorContains(tool.Validate(), "response")
	tool.Spec.Kubernetes.Response = ResponseBinary
	tool.Spec.Requires = []GVKRef{{APIVersion: "v1"}}
	s.ErrorContains(tool.Validate(), "spec.requires[0]")
	tool.Spec.Requires = []GVKRef{{APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine"}}
	s.NoError(tool.Validate())
	s.Equal(&binaryResult{mimeType: "image/png", data: []byte{1}}, decodeResponseAs([]byte{1}, ResponseBinary, "", "image/png", false))
	s.Equal("secret token=<REDACTED>", decodeResponseAs([]byte("secret token=abc"), ResponseText, "text/plain", "", true))
	s.Equal(map[string]any{"a": float64(1)}, decodeResponseAs([]byte(`{"a":1}`), "", "application/json", "", false))
}

func (s *DynamicSuite) TestToolset() {
	toolset := &Toolset{}
	s.Equal("dynamic", toolset.GetName())
	s.Run("exposes no tools without a cluster inspector", func() {
		s.Empty(toolset.GetTools(s.T().Context(), api.ToolsetContext{}))
	})
}

func TestDynamic(t *testing.T) {
	suite.Run(t, new(DynamicSuite))
}
