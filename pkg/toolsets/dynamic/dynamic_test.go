package dynamic

import (
	"encoding/json"
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
		s.ErrorContains(tool.Validate(), "one of spec.kubernetes or spec.http is required")
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
		out, err := expr.eval(nil, response)
		s.Require().NoError(err)
		raw, _ := json.Marshal(out)
		s.JSONEq(`[{"name":"a","replicas":2}]`, string(raw))
	})
	s.Run("exposes the tool arguments", func() {
		expr, err := compileExpression("{'ns': args.namespace, 'all': args}")
		s.Require().NoError(err)
		out, err := expr.eval(map[string]any{"namespace": "default"}, nil)
		s.Require().NoError(err)
		raw, _ := json.Marshal(out)
		s.JSONEq(`{"ns":"default","all":{"namespace":"default"}}`, string(raw))
	})
	s.Run("reports runtime errors", func() {
		expr, err := compileExpression("response.missing.field")
		s.Require().NoError(err)
		_, err = expr.eval(nil, map[string]any{})
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
		body, err := tool.resolveBody(nil, nil)
		s.Require().NoError(err)
		s.Equal("{}", string(body))
	})
	s.Run("string arguments are parsed as YAML", func() {
		body, err := tool.resolveBody(&Body{Argument: "manifest"}, map[string]any{"manifest": "apiVersion: v1\nkind: ConfigMap\n"})
		s.Require().NoError(err)
		s.JSONEq(`{"apiVersion":"v1","kind":"ConfigMap"}`, string(body))
	})
	s.Run("object arguments are serialized", func() {
		body, err := tool.resolveBody(&Body{Argument: "manifest"}, map[string]any{"manifest": map[string]any{"kind": "ConfigMap"}})
		s.Require().NoError(err)
		s.JSONEq(`{"kind":"ConfigMap"}`, string(body))
	})
	s.Run("missing arguments are reported", func() {
		_, err := tool.resolveBody(&Body{Argument: "manifest"}, nil)
		s.ErrorContains(err, "manifest parameter required")
	})
	s.Run("fixed values are passed through", func() {
		body, err := tool.resolveBody(&Body{Value: json.RawMessage(`{"spec":{"replicas":3}}`)}, nil)
		s.Require().NoError(err)
		s.JSONEq(`{"spec":{"replicas":3}}`, string(body))
	})
	s.Run("expressions are evaluated over the arguments", func() {
		expr, err := compileExpression("{'spec': {'replicas': int(args.replicas)}}")
		s.Require().NoError(err)
		tool := &compiledTool{bodyExpr: expr}
		body, err := tool.resolveBody(&Body{Expression: expr.source}, map[string]any{"replicas": float64(3)})
		s.Require().NoError(err)
		s.JSONEq(`{"spec":{"replicas":3}}`, string(body))
	})
}

func (s *DynamicSuite) TestResolveNamespace() {
	tool := &compiledTool{refExprs: map[string]*expression{}}
	s.Run("argument wins over value", func() {
		ns, all, err := tool.resolveNamespace(&NamespaceRef{Argument: "namespace", Value: "fixed"}, map[string]any{"namespace": "arg"})
		s.Require().NoError(err)
		s.Equal("arg", ns)
		s.False(all)
	})
	s.Run("falls back to the fixed value", func() {
		ns, all, err := tool.resolveNamespace(&NamespaceRef{Argument: "namespace", Value: "fixed"}, nil)
		s.Require().NoError(err)
		s.Equal("fixed", ns)
		s.False(all)
	})
	s.Run("falls back to all namespaces when requested", func() {
		ns, all, err := tool.resolveNamespace(&NamespaceRef{Argument: "namespace", All: true}, nil)
		s.Require().NoError(err)
		s.Equal("", ns)
		s.True(all)
	})
	s.Run("nil means the default namespace", func() {
		ns, all, err := tool.resolveNamespace(nil, nil)
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
		value, err := tool.resolveRef(spec.Kubernetes.Subresource, map[string]any{"info_type": "users"})
		s.Require().NoError(err)
		s.Equal("userlist", value)
	})
	s.Run("expression errors are reported", func() {
		_, err := tool.resolveRef(spec.Kubernetes.Subresource, map[string]any{"info_type": "all"})
		s.ErrorContains(err, "no such key")
	})
	s.Run("fixed values expand placeholders", func() {
		value, err := tool.resolveRef(spec.Kubernetes.Name, map[string]any{"name": "x"})
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
		out, err := expr.eval(map[string]any{"a": "1", "b": float64(1)}, nil)
		s.Require().NoError(err)
		raw, _ := json.Marshal(out)
		s.JSONEq(`{"a":"1","b":2,"mcp_mode":"true"}`, string(raw))
	})
	s.Run("transformMapEntry renames and drops keys", func() {
		expr, err := compileExpression("args.transformMapEntry(k, v, k == 'meshCluster' ? {'clusterName': v} : (k == 'drop' ? {} : {k: v}))")
		s.Require().NoError(err)
		out, err := expr.eval(map[string]any{"meshCluster": "east", "drop": "x", "keep": "y"}, nil)
		s.Require().NoError(err)
		raw, _ := json.Marshal(out)
		s.JSONEq(`{"clusterName":"east","keep":"y"}`, string(raw))
	})
	s.Run("now is the current time", func() {
		expr, err := compileExpression("{'start': int(now) - 300, 'end': int(now)}")
		s.Require().NoError(err)
		out, err := expr.evalStringMap(nil)
		s.Require().NoError(err)
		s.Regexp(`^\d{10}$`, out["end"])
		s.Regexp(`^\d{10}$`, out["start"])
	})
	s.Run("evalStringMap drops empty values and formats whole numbers", func() {
		expr, err := compileExpression("{'a': 1.0, 'b': 'x', 'c': '', 'd': null, 'e': true, 'f': 1.5}")
		s.Require().NoError(err)
		out, err := expr.evalStringMap(nil)
		s.Require().NoError(err)
		s.Equal(map[string]string{"a": "1", "b": "x", "e": "true", "f": "1.5"}, out)
	})
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
