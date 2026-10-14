package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
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
	s.Run("fixed values substitute placeholders verbatim, not URL-escaped", func() {
		// client-go validates and escapes path segments itself (and rejects
		// '%'), and rest.Request.Param encodes query values, so Kubernetes
		// references must carry the raw argument.
		value, err := tool.resolveRef(&Ref{Value: "proxy/{path}"}, evalVars{args: map[string]any{"path": "stats/summary"}})
		s.Require().NoError(err)
		s.Equal("proxy/stats/summary", value)
		query, err := tool.resolveQuery(map[string]Ref{"labelSelector": {Value: "app in ({apps})"}}, nil, evalVars{args: map[string]any{"apps": "a, b"}})
		s.Require().NoError(err)
		s.Equal("app in (a, b)", query["labelSelector"])
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
	s.Run("range refuses to allocate unbounded lists", func() {
		expr, err := compileExpression("size(range(args.n))")
		s.Require().NoError(err)
		out, err := expr.eval(evalVars{args: map[string]any{"n": float64(MaxRange)}})
		s.Require().NoError(err)
		s.Equal(float64(MaxRange), out)
		_, err = expr.eval(evalVars{args: map[string]any{"n": float64(10_000_000_000)}})
		s.ErrorContains(err, "range: at most")
		_, err = expr.eval(evalVars{args: map[string]any{"n": int64(MaxRange) + 1}})
		s.ErrorContains(err, "range: at most")
	})
	s.Run("evaluation cost is bounded", func() {
		// Each range stays within MaxRange, but the nested comprehension costs
		// MaxRange² steps: the program must give up instead of running them.
		expr, err := compileExpression("range(args.n).map(i, range(args.n).map(j, i * j)).size()")
		s.Require().NoError(err)
		_, err = expr.eval(evalVars{args: map[string]any{"n": float64(MaxRange)}})
		s.ErrorContains(err, "cost limit")
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
	s.Run("$${ renders a literal ${", func() {
		// Shell-style text in a prompt must not be compiled as CEL.
		t, err := parseTemplate("export HOME=$${HOME} user=${args.user} $$done")
		s.Require().NoError(err)
		out, err := t.render(evalVars{args: map[string]any{"user": "x"}})
		s.Require().NoError(err)
		s.Equal("export HOME=${HOME} user=x $$done", out)
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
	s.Equal(&binaryResult{mimeType: "image/jpeg", data: []byte{1}}, decodeResponseAs([]byte{1}, ResponseBinary, "image/jpeg", "", false), "the Content-Type labels the bytes when the Tool does not")
	s.Equal(&binaryResult{mimeType: "application/octet-stream", data: []byte{1}}, decodeResponseAs([]byte{1}, ResponseBinary, "", "", false), "unlabeled bytes default to application/octet-stream")
	s.Equal("secret token=<REDACTED>", decodeResponseAs([]byte("secret token=abc"), ResponseText, "text/plain", "", true))
	s.Equal(map[string]any{"a": float64(1)}, decodeResponseAs([]byte(`{"a":1}`), "", "application/json", "", false))
}

func (s *DynamicSuite) TestRBACOfMixedSteps() {
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "mixed"}, Spec: ToolSpec{
		Description: "d",
		Steps: []Step{
			{Name: "pods", Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Resource: "pods", Namespace: &NamespaceRef{Value: "istio-system"}}},
			{Name: "mesh", HTTP: &HTTPAction{URL: "https://kiali.example.com", Path: "/api/mesh"}},
		},
	}})
	s.Require().NoError(err)
	meta := tool.rbac(s.T().Context(), nil)
	s.Require().NoError(meta.Validate())
	s.Require().NotNil(meta.Bounded, "the Kubernetes requirements are fully known")
	s.Len(meta.Bounded.Requirements, 1)
	s.Contains(meta.Bounded.Reason, "https://kiali.example.com", "the external call must not be hidden behind the Kubernetes requirements")
	s.Contains(meta.Bounded.Reason, "bearer token")
	raw, _ := json.Marshal(meta)
	s.Contains(string(raw), `"reason":"`)

	s.Run("a purely Kubernetes tool has no reason", func() {
		tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "k8s"}, Spec: ToolSpec{
			Description: "d", Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Resource: "pods"},
		}})
		s.Require().NoError(err)
		raw, _ := json.Marshal(tool.rbac(s.T().Context(), nil))
		s.NotContains(string(raw), `"reason"`)
	})
	s.Run("a service action advertises the proxy verb of its HTTP method", func() {
		// The API server maps the proxied method to the verb it authorizes,
		// so a DELETE advertised as get/create would be granted the wrong one.
		for method, verb := range map[string]string{"": "get", "get": "get", "POST": "create", "put": "update", "PATCH": "patch", "DELETE": "delete"} {
			tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "svc"}, Spec: ToolSpec{
				Description: "d", HTTP: &HTTPAction{Service: &ServiceRef{Namespace: "istio-system", Name: "kiali"}, Method: method},
			}})
			s.Require().NoError(err)
			meta := tool.rbac(s.T().Context(), nil)
			s.Require().NotNil(meta.Bounded)
			s.Equal([]string{verb}, meta.Bounded.Requirements[0].Verbs, "method %q", method)
			s.Equal("proxy", meta.Bounded.Requirements[0].Target.Resource.Subresource)
		}
	})
}

func (s *DynamicSuite) TestHTTPTransport() {
	s.Run("a url action builds its transport once, with proxy and idle reaping", func() {
		action, err := compileAction(nil, &HTTPAction{URL: "https://kiali.example.com"})
		s.Require().NoError(err)
		s.Require().NotNil(action.transport, "the transport must be built at compile time and reused by every call")
		s.NotNil(action.transport.Proxy, "HTTPS_PROXY and friends must be honored")
		s.Positive(action.transport.IdleConnTimeout, "idle keep-alive connections must be reaped")
	})
	s.Run("a bad certificate authority is rejected at compile time", func() {
		_, err := compileAction(nil, &HTTPAction{URL: "https://kiali.example.com", CertificateAuthority: "not a pem"})
		s.ErrorContains(err, "certificateAuthority")
	})
	s.Run("a service action uses the caller's REST transport instead", func() {
		action, err := compileAction(nil, &HTTPAction{Service: &ServiceRef{Namespace: "istio-system", Name: "kiali"}})
		s.Require().NoError(err)
		s.Nil(action.transport)
	})
}

func (s *DynamicSuite) TestReadResponseBody() {
	s.Run("reads bodies up to the limit", func() {
		raw, err := readResponseBody(strings.NewReader(strings.Repeat("x", maxResponseBytes)))
		s.Require().NoError(err)
		s.Len(raw, maxResponseBytes)
	})
	s.Run("rejects larger bodies instead of truncating them", func() {
		_, err := readResponseBody(strings.NewReader(strings.Repeat("x", maxResponseBytes+1)))
		s.ErrorContains(err, "exceeds the maximum")
	})
}

func (s *DynamicSuite) TestListFailuresAreLoggedAtDefaultVerbosity() {
	s.Equal("dynamic", (&Toolset{}).GetName())
	s.Empty((&Toolset{}).GetTools(s.T().Context(), api.ToolsetContext{}), "no tools without a cluster inspector")

	// An operator who forgot the ClusterRole for Tool/Prompt resources must
	// see why no dynamic tools appear without raising the log verbosity.
	var lines []string
	logger := funcr.New(func(prefix, args string) { lines = append(lines, args) }, funcr.Options{Verbosity: 0})
	ctx := klog.NewContext(s.T().Context(), logger)
	inspector := &fakeInspector{names: map[string]string{"a": "1"}}
	inspector.setErr(errors.New(`tools.tools.kubernetes-mcp-server.io is forbidden: User "system:serviceaccount:mcp:server" cannot list resource "tools"`))
	toolsetContext := api.ToolsetContext{Inspector: inspector}
	s.Empty((&Toolset{}).GetTools(ctx, toolsetContext))
	s.Empty((&Toolset{}).GetPrompts(ctx, toolsetContext))
	s.Require().Len(lines, 2, "one line per failed list: %v", lines)
	s.Contains(lines[0], "unable to list resources, exposing none")
	s.Contains(lines[0], `"resource"="tools"`)
	s.Contains(lines[0], "is forbidden")
	s.Contains(lines[1], `"resource"="prompts"`)
}

func (s *DynamicSuite) TestListFailuresKeepTheLastKnownResources() {
	// A re-apply triggered by a kubeconfig change or a configuration reload
	// calls GetTools directly: a transient list failure there must not
	// deregister every dynamic tool, since the poller would not notice the
	// loss (its fingerprint is unchanged) and nothing would bring them back.
	var lines []string
	logger := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{Verbosity: 0})
	ctx := klog.NewContext(s.T().Context(), logger)
	inspector := &fakeInspector{names: map[string]string{"a": "1"}, prompts: map[string]string{"p": "1"}}
	toolsetContext := api.ToolsetContext{Inspector: inspector}
	toolset := &Toolset{}
	s.Len(toolset.GetTools(ctx, toolsetContext), 1)
	s.Len(toolset.GetPrompts(ctx, toolsetContext), 1)

	inspector.setErr(errors.New("the server is currently unable to handle the request"))
	s.Run("a list failure serves the last known tools and prompts", func() {
		s.Len(toolset.GetTools(ctx, toolsetContext), 1)
		s.Len(toolset.GetPrompts(ctx, toolsetContext), 1)
		s.Require().Len(lines, 2, "%v", lines)
		s.Contains(lines[0], "keeping the last known ones")
		s.Contains(lines[0], `"count"=1`)
	})
	s.Run("the next successful list replaces them", func() {
		inspector.setErr(nil)
		inspector.set(map[string]string{"a": "1", "b": "1"})
		s.Len(toolset.GetTools(ctx, toolsetContext), 2)
	})
}

func (s *DynamicSuite) TestWatchToolset() {
	previous := time.Duration(pollInterval.Load())
	SetPollInterval(10 * time.Millisecond)
	defer SetPollInterval(previous)

	var mu sync.Mutex
	var lines []string
	logger := funcr.New(func(_, args string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, args)
	}, funcr.Options{Verbosity: 0})
	logged := func(substr string) int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, line := range lines {
			if strings.Contains(line, substr) {
				n++
			}
		}
		return n
	}
	ctx := klog.NewContext(s.T().Context(), logger)
	inspector := &fakeInspector{names: map[string]string{"a": "1"}}
	listErr := errors.New(`tools.tools.kubernetes-mcp-server.io is forbidden`)

	s.Run("fingerprint reports a failed list instead of an empty set", func() {
		fp, err := fingerprint(ctx, inspector)
		s.Require().NoError(err)
		s.Equal("tool/a=1", fp)
		inspector.setErr(listErr)
		_, err = fingerprint(ctx, inspector)
		s.ErrorContains(err, "is forbidden")
		inspector.setErr(nil)
	})

	var changes atomic.Int32
	var applyFails atomic.Bool
	stop := (&Toolset{}).WatchToolset(ctx, api.ToolsetContext{Inspector: inspector}, func() error {
		changes.Add(1)
		if applyFails.Load() {
			return errors.New("convert failed")
		}
		return nil
	})
	defer stop()
	// polls waits for n more list calls from the watcher.
	polls := func(n int32) {
		before := inspector.lists.Load()
		s.Eventually(func() bool { return inspector.lists.Load() >= before+n }, 5*time.Second, 10*time.Millisecond)
	}

	s.Run("a list failure neither re-applies the toolsets nor logs every poll", func() {
		// A failed poll says nothing about the resources: treating it as
		// "every Tool was deleted" would drop all dynamic tools and flap them
		// back on the next success. And a missing ClusterRole must not
		// produce one error line per poll (four a minute, forever).
		inspector.setErr(listErr)
		polls(10)
		s.Zero(changes.Load())
		s.Equal(1, logged("unable to list"), "%v", lines)
		s.Equal(1, logged("is forbidden"), "%v", lines)
	})
	s.Run("recovering with the same resources does not re-apply either, and is logged once", func() {
		inspector.setErr(nil)
		polls(10)
		s.Zero(changes.Load())
		s.Equal(1, logged("can be listed again"), "%v", lines)
		s.Equal(1, logged("unable to list"), "%v", lines)
	})
	s.Run("a new failure is logged again", func() {
		inspector.setErr(listErr)
		polls(2)
		s.Equal(2, logged("unable to list"), "%v", lines)
		inspector.setErr(nil)
	})
	s.Run("a real change re-applies once", func() {
		inspector.set(map[string]string{"a": "1", "b": "1"})
		s.Eventually(func() bool { return changes.Load() == 1 }, 2*time.Second, 10*time.Millisecond)
		polls(5)
		s.Equal(int32(1), changes.Load())
	})
	s.Run("a failed re-apply is retried on the next poll, not forgotten", func() {
		applyFails.Store(true)
		inspector.set(map[string]string{"a": "2", "b": "1"})
		s.Eventually(func() bool { return changes.Load() >= 3 }, 2*time.Second, 10*time.Millisecond, "the change must be retried while the re-apply fails")
		applyFails.Store(false)
		s.Eventually(func() bool {
			before := changes.Load()
			time.Sleep(50 * time.Millisecond)
			return changes.Load() == before
		}, 2*time.Second, 10*time.Millisecond, "once the re-apply succeeds the change is consumed")
	})
}

// builtInToolset stands in for a compiled-in toolset (core, helm, ...) whose
// tool and prompt names a Tool or Prompt resource must not take over.
type builtInToolset struct{}

func (b *builtInToolset) GetName() string        { return "built-in-test" }
func (b *builtInToolset) GetDescription() string { return "built-in toolset of the dynamic tests" }
func (b *builtInToolset) GetTools(_ context.Context, _ api.ToolsetContext) []api.ServerTool {
	return []api.ServerTool{{Tool: api.Tool{Name: "builtin_tool", Description: "the real one"}}}
}
func (b *builtInToolset) GetPrompts(_ context.Context, _ api.ToolsetContext) []api.ServerPrompt {
	return []api.ServerPrompt{{Prompt: config.Prompt{Name: "builtin-prompt", Description: "the real one"}}}
}
func (b *builtInToolset) GetResources(_ context.Context, _ api.ToolsetContext) []api.ServerResource {
	return nil
}
func (b *builtInToolset) GetResourceTemplates(_ context.Context, _ api.ToolsetContext) []api.ServerResourceTemplate {
	return nil
}

func (s *DynamicSuite) TestResourcesShadowingBuiltInsAreSkipped() {
	// A cluster-level actor must not be able to replace a compiled-in tool
	// (say, make pods_delete look read-only) by creating a Tool of that name.
	var lines []string
	logger := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{Verbosity: 0})
	ctx := klog.NewContext(s.T().Context(), logger)
	inspector := &fakeInspector{
		names:   map[string]string{"builtin-tool": "1", "other-tool": "1"},
		prompts: map[string]string{"builtin-prompt": "1", "other-prompt": "1"},
	}
	toolsetContext := api.ToolsetContext{Inspector: inspector, Toolsets: []api.Toolset{&builtInToolset{}, &Toolset{}}}

	var toolNames []string
	for _, tool := range (&Toolset{}).GetTools(ctx, toolsetContext) {
		toolNames = append(toolNames, tool.Tool.Name)
	}
	s.Equal([]string{"other_tool"}, toolNames)
	var promptNames []string
	for _, prompt := range (&Toolset{}).GetPrompts(ctx, toolsetContext) {
		promptNames = append(promptNames, prompt.Prompt.Name)
	}
	s.Equal([]string{"other-prompt"}, promptNames)

	s.Require().Len(lines, 2, "one line per skipped resource: %v", lines)
	s.Contains(lines[0], "shadows")
	s.Contains(lines[0], "builtin_tool")
	s.Contains(lines[0], "built-in-test")
	s.Contains(lines[1], "shadows")
	s.Contains(lines[1], "builtin-prompt")
	s.Contains(lines[1], "built-in-test")
}

func (s *DynamicSuite) TestResourcesWithDuplicateNamesAreSkipped() {
	// foo-bar and foo.bar both become foo_bar; the server refuses a duplicate
	// name, which would take every toolset down, so only the first is kept.
	var lines []string
	logger := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{Verbosity: 0})
	ctx := klog.NewContext(s.T().Context(), logger)
	inspector := &fakeInspector{
		names:   map[string]string{"foo-bar": "1", "foo.bar": "1", "other": "1"},
		prompts: map[string]string{"dup": "1", "dup-2": "1"},
		// Prompt names are not normalized, so the duplicate is an explicit spec.name.
		promptSpec: map[string]map[string]any{"dup-2": {"name": "dup"}},
	}
	toolsetContext := api.ToolsetContext{Inspector: inspector}

	var toolNames []string
	for _, tool := range (&Toolset{}).GetTools(ctx, toolsetContext) {
		toolNames = append(toolNames, tool.Tool.Name)
	}
	s.Equal([]string{"foo_bar", "other"}, toolNames)
	var promptNames []string
	for _, prompt := range (&Toolset{}).GetPrompts(ctx, toolsetContext) {
		promptNames = append(promptNames, prompt.Prompt.Name)
	}
	s.Equal([]string{"dup"}, promptNames)

	s.Require().Len(lines, 2, "one line per skipped resource: %v", lines)
	s.Contains(lines[0], "already taken")
	s.Contains(lines[0], `"tool"="foo.bar"`)
	s.Contains(lines[0], `"first"="foo-bar"`)
	s.Contains(lines[1], "already taken")
	s.Contains(lines[1], `"prompt"="dup-2"`)
	s.Contains(lines[1], `"first"="dup"`)
}

// fakeInspector serves Tool and Prompt resources (name -> resourceVersion)
// from memory and can be switched to fail every list.
type fakeInspector struct {
	mu      sync.Mutex
	names   map[string]string
	prompts map[string]string
	// promptSpec overrides spec fields of the named prompts.
	promptSpec map[string]map[string]any
	err        error
	// lists counts the list calls, so tests can wait for a number of polls.
	lists atomic.Int32
}

func (f *fakeInspector) set(names map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.names = names
}

func (f *fakeInspector) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeInspector) Discovery() api.AggregateDiscovery { return nil }

func (f *fakeInspector) Unstructured() api.AggregateUnstructured { return f }

func (f *fakeInspector) Resource(gvr schema.GroupVersionResource) api.AggregateNamespaceableResourceInterface {
	return &fakeResource{inspector: f, gvr: gvr}
}

type fakeResource struct {
	inspector *fakeInspector
	gvr       schema.GroupVersionResource
}

func (r *fakeResource) Namespace(string) api.AggregateResourceInterface { return r }

func (r *fakeResource) Get(ctx context.Context, _ string, _ metav1.GetOptions, _ ...string) api.Results[*unstructured.Unstructured] {
	return api.NewResults(ctx, singleTarget{}, func(context.Context, string) (*unstructured.Unstructured, error) {
		return nil, errors.New("not implemented")
	})
}

func (r *fakeResource) List(ctx context.Context, _ metav1.ListOptions) api.Results[*unstructured.UnstructuredList] {
	return api.NewResults(ctx, singleTarget{}, func(context.Context, string) (*unstructured.UnstructuredList, error) {
		r.inspector.lists.Add(1)
		r.inspector.mu.Lock()
		defer r.inspector.mu.Unlock()
		if r.inspector.err != nil {
			return nil, r.inspector.err
		}
		list := &unstructured.UnstructuredList{}
		if r.gvr == PromptGVR {
			for name, version := range r.inspector.prompts {
				spec := map[string]any{"description": "d", "messages": []any{map[string]any{"role": "user", "text": "hi"}}}
				for key, value := range r.inspector.promptSpec[name] {
					spec[key] = value
				}
				list.Items = append(list.Items, unstructured.Unstructured{Object: map[string]any{
					"apiVersion": Group + "/" + Version, "kind": "Prompt",
					"metadata": map[string]any{"name": name, "resourceVersion": version},
					"spec":     spec,
				}})
			}
			return list, nil
		}
		if r.gvr != GVR {
			return list, nil
		}
		for name, version := range r.inspector.names {
			list.Items = append(list.Items, unstructured.Unstructured{Object: map[string]any{
				"apiVersion": Group + "/" + Version, "kind": "Tool",
				"metadata": map[string]any{"name": name, "resourceVersion": version},
				"spec":     map[string]any{"description": "d", "kubernetes": map[string]any{"verb": "get", "apiVersion": "v1", "kind": "Pod"}},
			}})
		}
		return list, nil
	})
}

type singleTarget struct{}

func (singleTarget) IsMultiTarget() bool                          { return false }
func (singleTarget) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }
func (singleTarget) GetDefaultTarget() string                     { return "" }
func (singleTarget) GetTargetParameterName() string               { return "" }

func TestDynamic(t *testing.T) {
	suite.Run(t, new(DynamicSuite))
}
