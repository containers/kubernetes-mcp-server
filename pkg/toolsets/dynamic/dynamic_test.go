package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
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
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"

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
		s.ErrorContains(tool.Validate(), "one of spec.kubernetes, spec.http, spec.steps or spec.variants is required")
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
		s.ErrorContains(tool.Validate(), "exactly one of spec.http.url, spec.http.urlExpression or spec.http.service")
	})
	s.Run("rejects http with both url and urlExpression", func() {
		tool := valid()
		tool.Spec.Kubernetes = nil
		tool.Spec.HTTP = &HTTPAction{URL: "https://example.com", URLExpression: "steps.cm.data.url"}
		s.ErrorContains(tool.Validate(), "exactly one of spec.http.url, spec.http.urlExpression or spec.http.service")
	})
	s.Run("accepts http with a urlExpression", func() {
		tool := valid()
		tool.Spec.Kubernetes = nil
		tool.Spec.HTTP = &HTTPAction{URLExpression: "steps.cm.data.url"}
		s.NoError(tool.Validate())
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

func (s *DynamicSuite) TestResolvePath() {
	tool := &compiledTool{refExprs: map[string]*expression{}}
	s.Run("substitutes and escapes arguments", func() {
		path, err := tool.resolvePath(&Ref{Value: "/namespaces/{namespace}/pods/{name}"}, evalVars{args: map[string]any{"namespace": "a b", "name": "x/y"}})
		s.Require().NoError(err)
		s.Equal("/namespaces/a%20b/pods/x%2Fy", path)
	})
	s.Run("fails on missing arguments", func() {
		_, err := tool.resolvePath(&Ref{Value: "/namespaces/{namespace}/pods/{name}"}, evalVars{args: map[string]any{"namespace": "a"}})
		s.ErrorContains(err, "name parameter required")
	})
	s.Run("leaves literal paths untouched", func() {
		path, err := tool.resolvePath(&Ref{Value: "/api/mcp/get_mesh_status"}, evalVars{})
		s.Require().NoError(err)
		s.Equal("/api/mcp/get_mesh_status", path)
	})
	s.Run("appends an expression verbatim", func() {
		ref := &Ref{Expression: "'/' + steps.tenant + '/loki/api/v1/labels'"}
		s.Require().NoError(tool.compileRefs([]*Ref{ref}))
		path, err := tool.resolvePath(ref, evalVars{steps: map[string]any{"tenant": "a b"}})
		s.Require().NoError(err)
		s.Equal("/a b/loki/api/v1/labels", path)
	})
	s.Run("no path is the empty path", func() {
		path, err := tool.resolvePath(nil, evalVars{})
		s.Require().NoError(err)
		s.Empty(path)
	})
}

func (s *DynamicSuite) TestURLExpression() {
	s.Run("is compiled and must not read the arguments", func() {
		_, err := compileAction(nil, &HTTPAction{URLExpression: "'https://' + args.host"})
		s.ErrorContains(err, "must not reference args")
		_, err = compileAction(nil, &HTTPAction{URLExpression: "steps.route.spec.host.size() > 0 ? 'https://' + steps.route.spec.host : 'http://fallback'"})
		s.NoError(err)
		_, err = compileAction(nil, &HTTPAction{URLExpression: "steps.route.spec."})
		s.ErrorContains(err, "urlExpression")
	})
	s.Run("is reported as the RBAC reason", func() {
		tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{
			Description: "d",
			Steps: []Step{
				{Name: "cm", Kubernetes: &KubernetesAction{Verb: "get", APIVersion: "v1", Resource: "configmaps", Name: &Ref{Value: "loki"}}},
				{Name: "labels", HTTP: &HTTPAction{URLExpression: "steps.cm.data.url"}},
			},
		}})
		s.Require().NoError(err)
		meta := tool.rbac(s.T().Context(), nil)
		s.Require().NotNil(meta.Bounded)
		s.Contains(meta.Bounded.Reason, "the URL computed by steps.cm.data.url")
	})
}

func (s *DynamicSuite) TestIntegersSurviveSteps() {
	// A step result is a plain Go value; integers must stay integers so that
	// a later string(...) renders digits (Loki wants 19-digit nanoseconds,
	// not 1.7e+09) and so that JSON numbers from responses and CEL ints mix.
	expr, err := compileExpression("{'int': 1700000000, 'double': 1.5, 'list': [1, 2.5], 'nested': {'n': 3}, 'when': timestamp(0), 'none': null}")
	s.Require().NoError(err)
	value, err := expr.eval(evalVars{})
	s.Require().NoError(err)
	s.Equal(map[string]any{
		"int": int64(1700000000), "double": 1.5, "list": []any{int64(1), 2.5},
		"nested": map[string]any{"n": int64(3)}, "when": "1970-01-01T00:00:00Z", "none": nil,
	}, value)
	render, err := compileExpression("string(steps.range.start) + ' ' + string(int(steps.range.start)) + ' ' + yaml(steps.range)")
	s.Require().NoError(err)
	_, err = render.eval(evalVars{steps: map[string]any{"range": map[string]any{}}})
	s.Require().Error(err, "a missing key is still an error")
	text, err := render.eval(evalVars{steps: map[string]any{"range": map[string]any{"start": int64(1700000000)}}})
	s.Require().NoError(err)
	s.Equal("1700000000 1700000000 start: 1700000000\n", text)
	s.Equal("1700000000", scalarString(int64(1700000000)))
}

func (s *DynamicSuite) TestServes() {
	expr, err := compileExpression("cluster.serves('route.openshift.io/v1', 'Route') ? 'route' : 'none'")
	s.Require().NoError(err)
	s.Run("asks the target of the current call", func() {
		var asked [][2]string
		ctx := withServesFunc(context.Background(), func(apiVersion, kind string) bool {
			asked = append(asked, [2]string{apiVersion, kind})
			return kind == "Route"
		})
		value, err := expr.eval(evalVars{ctx: ctx})
		s.Require().NoError(err)
		s.Equal("route", value)
		s.Equal([][2]string{{"route.openshift.io/v1", "Route"}}, asked)
	})
	s.Run("is an error outside a call", func() {
		_, err := expr.eval(evalVars{})
		s.ErrorContains(err, "cluster.serves is only available while a tool or prompt runs")
	})
	s.Run("is false without a client", func() {
		value, err := expr.eval(evalVars{ctx: withServes(context.Background(), nil)})
		s.Require().NoError(err)
		s.Equal("none", value)
	})
}

func (s *DynamicSuite) TestRequestPathGuards() {
	for _, path := range []string{"", "/", "/api/v1/labels", "/a/./b", "/x%2Fy", "/trace/abc"} {
		s.NoError(checkRequestPath(path), path)
	}
	for path, message := range map[string]string{
		"@evil.example/steal": "must start with a single /",
		".evil.example/x":     "must start with a single /",
		":8443/x":             "must start with a single /",
		"//evil.example/x":    "must start with a single /",
		"api/x":               "must start with a single /",
		"/x?watch=true":       "query or fragment",
		"/x#frag":             "query or fragment",
		"/a/../../etc":        ".. segment",
		"/a/%2e%2e/b":         ".. segment",
		"/proxy/..":           ".. segment",
	} {
		s.ErrorContains(checkRequestPath(path), message, path)
	}
	s.Run("the parsed request must keep the base origin", func() {
		u, err := url.Parse("https://kiali.example.com" + "/ok")
		s.Require().NoError(err)
		s.NoError(checkSameOrigin("https://kiali.example.com", u))
		u, err = url.Parse("https://kiali.example.com" + "@evil.example/x")
		s.Require().NoError(err)
		s.ErrorContains(checkSameOrigin("https://kiali.example.com", u), "changed the request origin")
		u, err = url.Parse("https://api.example.com/api/v1/namespaces/a/services/b/proxy" + "/%2e%2e/%2e%2e/secrets")
		s.Require().NoError(err)
		s.ErrorContains(checkSameOrigin("https://api.example.com", u), ".. segment")
	})
}

func (s *DynamicSuite) TestReservedKubernetesQueryKeys() {
	action := &KubernetesAction{Query: map[string]Ref{"follow": {Value: "false"}}}
	s.NoError(action.checkQueryKeys(map[string][]string{"labelSelector": {"a=b"}, "limit": {"10"}}))
	s.NoError(action.checkQueryKeys(map[string][]string{"follow": {"true"}}), "a declared key is the author's choice")
	for _, key := range []string{"watch", "dryRun", "propagationPolicy", "gracePeriodSeconds", "force", "fieldManager", "timeoutSeconds"} {
		s.ErrorContains(action.checkQueryKeys(map[string][]string{key: {"x"}}), key)
	}
}

func (s *DynamicSuite) TestKubernetesResponseBodyCap() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/small" {
			_, _ = w.Write([]byte(`{"kind":"List","items":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":["`))
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBytes)))
		_, _ = w.Write([]byte(`"]}`))
	}))
	defer server.Close()
	client, err := newRESTClient(&rest.Config{Host: server.URL}, schema.GroupVersion{Version: "v1"})
	s.Require().NoError(err)
	raw, err := client.Get().AbsPath("/api/v1/small").Do(s.T().Context()).Raw()
	s.Require().NoError(err)
	s.Contains(string(raw), `"kind":"List"`)
	_, err = client.Get().AbsPath("/api/v1/huge").Do(s.T().Context()).Raw()
	s.ErrorContains(err, "exceeds the maximum size")
}

func (s *DynamicSuite) TestAnnotationsMatchVerbs() {
	tool := func(readOnly, destructive bool, verb string, steps bool) *Tool {
		t := &Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", ReadOnly: readOnly, Destructive: destructive}}
		action := &KubernetesAction{Verb: verb, APIVersion: "v1", Kind: "Pod", Name: &Ref{Argument: "name"}}
		if steps {
			t.Spec.Steps = []Step{{Name: "a", Kubernetes: action}}
		} else {
			t.Spec.Kubernetes = action
		}
		return t
	}
	s.NoError(tool(true, false, "get", false).Validate())
	s.NoError(tool(true, false, "list", true).Validate())
	s.NoError(tool(false, false, "patch", false).Validate())
	s.NoError(tool(false, true, "delete", true).Validate())
	s.ErrorContains(tool(true, false, "delete", false).Validate(), "spec.kubernetes performs a delete but spec.readOnly is true")
	s.ErrorContains(tool(true, false, "patch", true).Validate(), "spec.steps[0] (a) performs a patch but spec.readOnly is true")
	s.ErrorContains(tool(false, false, "delete", false).Validate(), "performs a delete but spec.destructive is not true")
	s.Run("prompts classify their Kubernetes steps", func() {
		read := Prompt{Spec: PromptSpec{Steps: []Step{{Name: "a", Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Kind: "Pod"}}, {Name: "b", HTTP: &HTTPAction{URL: "https://x", Method: "POST"}}}}}
		s.False(read.Mutates(), "HTTP methods are not classified")
		write := Prompt{Spec: PromptSpec{Steps: []Step{{Name: "a", Kubernetes: &KubernetesAction{Verb: "create", APIVersion: "v1", Kind: "Pod"}}}}}
		s.True(write.Mutates())
	})
}

func (s *DynamicSuite) TestReservedHeaders() {
	for _, header := range []string{"Authorization", "authorization", "Proxy-Authorization", "Host", "Cookie", "Impersonate-User", "impersonate-group"} {
		action := &HTTPAction{URL: "https://example.com", Headers: map[string]Ref{header: {Argument: "x"}}}
		s.ErrorContains(errors.Join(action.validate()...), "may not set "+header, header)
	}
	action := &HTTPAction{URL: "https://example.com", Headers: map[string]Ref{"X-Scope-OrgID": {Argument: "tenant"}, "Accept": {Value: "text/csv"}}}
	s.Empty(action.validate())
}

func (s *DynamicSuite) TestMalformedResourceIsSkipped() {
	// spec.name is a string; a number does not fit the Go definition and the
	// CRD does not catch it. The other resources must still load.
	inspector := &fakeInspector{names: map[string]string{"good": "1", "bad": "1"}, toolSpec: map[string]map[string]any{"bad": {"name": 5}}}
	tools, err := listResources[Tool](context.Background(), inspector, GVR)
	s.Require().NoError(err)
	s.Require().Len(tools, 1)
	s.Equal("good", tools[0].Metadata.Name)
}

func (s *DynamicSuite) TestResponseDecoding() {
	s.Run("JSON is decoded only when the server says so", func() {
		s.Equal(map[string]any{"a": int64(1)}, decodeResponseAs([]byte(`{"a":1}`), "", "application/json; charset=utf-8", "", false))
		s.Equal(map[string]any{"a": int64(1)}, decodeResponseAs([]byte(`{"a":1}`), "", "", "", false), "no Content-Type is JSON (Kubernetes API)")
		s.Equal(map[string]any{"a": int64(1)}, decodeResponseAs([]byte(`{"a":1}`), "", "application/merge-patch+json", "", false))
		s.Equal(`{"a":1}`, decodeResponseAs([]byte(`{"a":1}`), "", "text/plain", "", false), "a log line that is JSON stays text")
		s.Equal("123", decodeResponseAs([]byte(`123`), "", "text/html", "", false))
		s.Equal("{\"a\":1}\n{\"b\":2}\n", decodeResponseAs([]byte("{\"a\":1}\n{\"b\":2}\n"), "", "", "", false), "several JSON documents are not one response")
		s.Equal(map[string]any{"a": int64(1)}, decodeResponseAs([]byte(`{"a":1}`), ResponseJSON, "text/plain", "", false), "response: json forces decoding")
		s.Nil(decodeResponseAs(nil, "", "application/json", "", false))
	})
	s.Run("whole numbers are integers, fractional numbers doubles", func() {
		decoded := decodeResponseAs([]byte(`{"count": 3, "ts": 1700000000, "ratio": 0.5, "big": 1e30, "list": [1, 2.5]}`), "", "application/json", "", false)
		s.Equal(map[string]any{"count": int64(3), "ts": int64(1700000000), "ratio": 0.5, "big": 1e30, "list": []any{int64(1), 2.5}}, decoded)
		expr, err := compileExpression("string(response.ts) + ' ' + string(response.count * 2) + ' ' + string(response.ts - steps.start)")
		s.Require().NoError(err)
		value, err := expr.eval(evalVars{response: decoded, steps: map[string]any{"start": int64(1699999000)}})
		s.Require().NoError(err)
		s.Equal("1700000000 6 1000", value)
	})
	s.Run("arguments render as digits", func() {
		args := map[string]any{"tail": float64(1000000), "ratio": 0.25, "name": "x", "n": nil}
		s.Equal("1000000", argString(args, "tail"))
		s.Equal("0.25", argString(args, "ratio"))
		s.Equal("", argString(args, "n"))
		normalizeNumbers(args)
		s.Equal(int64(1000000), args["tail"])
		s.Equal(0.25, args["ratio"])
	})
}

func (s *DynamicSuite) TestFailStepMustBeAString() {
	compile := func(fail string) *compiledTool {
		tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", Steps: []Step{
			{Name: "check", Fail: fail},
			{Name: "ok", Expression: "'done'"},
		}}})
		s.Require().NoError(err)
		return tool
	}
	run := func(tool *compiledTool, n int64) (any, error) {
		vars := evalVars{args: map[string]any{"n": n}, steps: map[string]any{}}
		last, _, err := tool.runSteps(runEnv{ctx: context.Background()}, &vars)
		return last, err
	}
	_, err := run(compile("args.n > 3"), 1)
	s.ErrorContains(err, "fail must evaluate to a string (empty to pass), got bool")
	tool := compile("args.n > 3 ? 'too many' : ''")
	last, err := run(tool, 1)
	s.Require().NoError(err)
	s.Equal("done", last)
	_, err = run(tool, 5)
	s.ErrorContains(err, "too many")
}

func (s *DynamicSuite) TestToolSetFamilies() {
	inspector := &fakeInspector{
		names:    map[string]string{"loki-labels": "1", "mislabeled": "1", "orphan": "1", "plain": "1"},
		prompts:  map[string]string{"loki-triage": "1", "plain-prompt": "1"},
		toolsets: map[string]map[string]any{"loki": {"prefix": "loki", "instructions": "Discover labels first.", "requires": []any{map[string]any{"apiVersion": "loki.grafana.com/v1", "kind": "LokiStack"}}}},
		toolSpec: map[string]map[string]any{
			"loki-labels": {"toolset": "loki", "name": "loki_labels"},
			"mislabeled":  {"toolset": "loki", "name": "labels"},
			"orphan":      {"toolset": "nope"},
		},
		promptSpec: map[string]map[string]any{"loki-triage": {"toolset": "loki"}, "plain-prompt": {"toolset": "loki", "name": "triage"}},
	}
	toolsetContext := api.ToolsetContext{Inspector: inspector}
	base := &Toolset{}
	names := func(tools []api.ServerTool) []string {
		var out []string
		for _, tool := range tools {
			out = append(out, tool.Tool.Name)
		}
		sort.Strings(out)
		return out
	}
	s.Run("the whole toolset exposes every family plus the tools without one", func() {
		s.Equal([]string{"loki_labels", "plain"}, names(base.GetTools(s.T().Context(), toolsetContext)), "mislabeled lacks the prefix, orphan names a missing ToolSet")
		prompts := base.GetPrompts(s.T().Context(), toolsetContext)
		s.Require().Len(prompts, 1, "a prompt of the loki family must be named loki-...")
		s.Equal("loki-triage", prompts[0].Prompt.Name)
	})
	s.Run("a family exposes its own tools and prompts only", func() {
		family := base.Family("dynamic/loki")
		s.Require().NotNil(family)
		s.Equal("dynamic/loki", family.GetName())
		s.Equal([]string{"loki_labels"}, names(family.GetTools(s.T().Context(), toolsetContext)))
		prompts := family.GetPrompts(s.T().Context(), toolsetContext)
		s.Require().Len(prompts, 1)
		s.Equal("loki-triage", prompts[0].Prompt.Name)
		s.Nil(base.Family("dynamic/"), "an empty family name is not a family")
		s.Nil(base.Family("other/loki"))
	})
	s.Run("the family's requires are inherited", func() {
		family := base.Family("dynamic/loki")
		tools := family.GetTools(s.T().Context(), api.ToolsetContext{Inspector: inspector, TargetCompatibilityFiltersEnabled: true})
		s.Require().Len(tools, 1)
		s.NotEmpty(tools[0].TargetCompatibilityFilters, "the LokiStack requirement of the ToolSet applies to the tool")
	})
	s.Run("instructions are contributed under a heading", func() {
		s.Equal("## loki\n\nDiscover labels first.", base.GetInstructions(s.T().Context(), toolsetContext))
		s.Equal("## loki\n\nDiscover labels first.", base.Family("dynamic/loki").(api.ToolsetInstructions).GetInstructions(s.T().Context(), toolsetContext))
		s.Empty(base.Family("dynamic/tempo").(api.ToolsetInstructions).GetInstructions(s.T().Context(), toolsetContext))
	})
	s.Run("ToolSet validation", func() {
		s.NoError((&ToolSet{Metadata: Metadata{Name: "x"}, Spec: ToolSetSpec{Description: "d", Prefix: "x", Identity: "server"}}).Validate())
		s.ErrorContains((&ToolSet{Metadata: Metadata{Name: "x"}, Spec: ToolSetSpec{Description: "d", Prefix: "Bad-Prefix"}}).Validate(), "spec.prefix")
		s.ErrorContains((&ToolSet{Metadata: Metadata{Name: "x"}, Spec: ToolSetSpec{Description: "d", Identity: "nobody"}}).Validate(), "spec.identity")
		s.ErrorContains((&ToolSet{Metadata: Metadata{Name: "x"}, Spec: ToolSetSpec{Description: " "}}).Validate(), "spec.description")
	})
}

// restConfigClient is an api.KubernetesClient that only answers RESTConfig,
// which is all a url-mode http action needs.
type restConfigClient struct {
	api.KubernetesClient
	config *rest.Config
}

func (c restConfigClient) RESTConfig() *rest.Config { return c.config }

func (s *DynamicSuite) TestToolSetIdentity() {
	var authorizations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", HTTP: &HTTPAction{URL: server.URL}}})
	s.Require().NoError(err)
	caller := restConfigClient{config: &rest.Config{BearerToken: "caller-token"}}
	env := runEnv{ctx: context.Background(), client: caller}
	s.Run("a tool acts as the caller by default", func() {
		_, err := tool.run(tool.action, env, evalVars{})
		s.Require().NoError(err)
		s.Equal([]string{"Bearer caller-token"}, authorizations)
		s.Contains(tool.rbac(context.Background(), nil).None.Reason, "the caller's bearer token")
	})
	s.Run("a tool of a server-identity family sends the server's credentials", func() {
		authorizations = nil
		tool.identity = IdentityServer
		tool.serverClients = func(context.Context, string) (api.KubernetesClient, error) {
			return restConfigClient{config: &rest.Config{BearerToken: "server-token"}}, nil
		}
		env.serverConfig = tool.serverConfigFor(context.Background(), "east")
		_, err := tool.run(tool.action, env, evalVars{})
		s.Require().NoError(err)
		s.Equal([]string{"Bearer server-token"}, authorizations)
		s.Contains(tool.rbac(context.Background(), nil).None.Reason, "the server's own credentials (ToolSet identity: server)")
	})
	s.Run("a server that does not offer its credentials fails the call", func() {
		tool.serverClients = nil
		env.serverConfig = tool.serverConfigFor(context.Background(), "")
		_, err := tool.run(tool.action, env, evalVars{})
		s.ErrorContains(err, "does not offer its own credentials")
	})
}

// dynamicOnlyClient is an api.KubernetesClient that only answers
// DynamicClient, which is all status reporting needs.
type dynamicOnlyClient struct {
	api.KubernetesClient
	dynamic dynamic.Interface
}

func (c dynamicOnlyClient) DynamicClient() dynamic.Interface { return c.dynamic }

func (s *DynamicSuite) TestStatusReporting() {
	scheme := runtime.NewScheme()
	tool := func(name, version string, generation int64) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": Group + "/" + Version, "kind": Kind,
			"metadata": map[string]any{"name": name, "resourceVersion": version, "generation": generation},
			"spec":     map[string]any{"description": "d"},
		}}
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{GVR: "ToolList"}, tool("good", "1", 2), tool("bad", "1", 1))
	toolsetContext := api.ToolsetContext{ServerClients: func(context.Context, string) (api.KubernetesClient, error) {
		return dynamicOnlyClient{dynamic: client}, nil
	}}
	listed := []listedStatus{{name: "good", resourceVersion: "1", generation: 2}, {name: "bad", resourceVersion: "1", generation: 1}, {name: "undecided", resourceVersion: "1"}}
	outcomes := map[string]outcome{
		"good": {ready: true, reason: ReasonRegistered, message: "exposed as tool good"},
		"bad":  {reason: ReasonInvalid, message: "spec.description is required"},
	}
	base := &Toolset{}
	base.reportStatus(context.Background(), toolsetContext, GVR, Kind, listed, outcomes)
	condition := func(name string) map[string]any {
		object, err := client.Resource(GVR).Get(context.Background(), name, metav1.GetOptions{})
		s.Require().NoError(err)
		conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
		s.Require().Len(conditions, 1, name)
		return conditions[0].(map[string]any)
	}
	s.Run("a Ready condition says what the server did", func() {
		good := condition("good")
		s.Equal("True", good["status"])
		s.Equal(ReasonRegistered, good["reason"])
		s.Equal("exposed as tool good", good["message"])
		s.EqualValues(2, good["observedGeneration"])
		bad := condition("bad")
		s.Equal("False", bad["status"])
		s.Equal(ReasonInvalid, bad["reason"])
		s.Equal("spec.description is required", bad["message"])
	})
	s.Run("an undecided resource is left alone", func() {
		_, err := client.Resource(GVR).Get(context.Background(), "undecided", metav1.GetOptions{})
		s.True(apierrors.IsNotFound(err), "never existed and must not be created by a status write")
	})
	s.Run("an unchanged condition is not written again", func() {
		updates := func() int {
			n := 0
			for _, action := range client.Actions() {
				if action.GetVerb() == "update" {
					n++
				}
			}
			return n
		}
		for i := range listed {
			object, err := client.Resource(GVR).Get(context.Background(), listed[i].name, metav1.GetOptions{})
			if err != nil {
				continue
			}
			raw, _ := json.Marshal(object.Object["status"])
			var status ResourceStatus
			_ = json.Unmarshal(raw, &status)
			listed[i].conditions = status.Conditions
		}
		writes := updates()
		s.Equal(2, writes, "one write per decided resource so far")
		base.reportStatus(context.Background(), toolsetContext, GVR, Kind, listed, outcomes)
		s.Equal(writes, updates(), "the same outcome at the same generation is a no-op")
	})
	s.Run("without a server client nothing is written and nothing fails", func() {
		base.reportStatus(context.Background(), api.ToolsetContext{}, GVR, Kind, listed, outcomes)
	})
}

// argsRequest is an api.ToolCallRequest carrying fixed arguments.
type argsRequest map[string]any

func (r argsRequest) GetArguments() map[string]any { return r }

func (s *DynamicSuite) TestTimeout() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "slow"}, Spec: ToolSpec{Description: "d", Timeout: "100ms", HTTP: &HTTPAction{URL: server.URL}}})
	s.Require().NoError(err)
	params := api.ToolHandlerParams{Context: context.Background(), KubernetesClient: restConfigClient{config: &rest.Config{}}, Request: argsRequest{}}
	started := time.Now()
	result, err := tool.handle(params)
	s.Require().NoError(err)
	s.Require().Error(result.Error)
	s.ErrorContains(result.Error, "slow timed out after 100ms")
	s.Less(time.Since(started), 2*time.Second, "the call ends at the timeout, not when the backend answers")
	s.Run("the timeout is validated", func() {
		invalid := &Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", Timeout: "soon", Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Kind: "Pod"}}}
		s.ErrorContains(invalid.Validate(), `spec.timeout "soon" is not a duration`)
		invalid.Spec.Timeout = "0s"
		s.ErrorContains(invalid.Validate(), "must be positive")
		valid, err := parseTimeout("spec.timeout", "")
		s.Require().NoError(err)
		s.Equal(DefaultTimeout, valid)
	})
}

func (s *DynamicSuite) TestErrorsAreWrittenForTheModel() {
	s.Run("a runtime error carries the CEL message, not the definition", func() {
		expr, err := compileExpression("steps.pods.items.map(p, p.metadata.name)")
		s.Require().NoError(err)
		_, err = expr.eval(evalVars{steps: map[string]any{"pods": map[string]any{}}})
		s.Require().Error(err)
		s.Contains(err.Error(), "no such key: items")
		s.NotContains(err.Error(), "steps.pods.items.map", "the expression source is for the author's log, not the model")
	})
	s.Run("type mismatches are named in the author's terms", func() {
		s.Equal("map", typeName(map[string]any{}))
		s.Equal("list", typeName([]any{}))
		s.Equal("null", typeName(nil))
		s.Equal("number", typeName(int64(1)))
		s.Equal("number", typeName(1.5))
		tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", Steps: []Step{{Name: "a", Expression: "{'x': 1}", ForEach: "{'not': 'a list'}"}}}})
		s.Require().NoError(err)
		vars := evalVars{steps: map[string]any{}}
		_, _, err = tool.runSteps(runEnv{ctx: context.Background()}, &vars)
		s.ErrorContains(err, "forEach must evaluate to a list, got map")
	})
}

func (s *DynamicSuite) TestRBACNamespaceFallback() {
	// namespace: {argument: ns, all: true} lists every namespace when the
	// argument is omitted, so the metadata must say all namespaces, not the
	// argument (the shipped deployments-summary example has this shape).
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d",
		Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "apps/v1", Resource: "deployments", Namespace: &NamespaceRef{Argument: "namespace", All: true}}}})
	s.Require().NoError(err)
	meta := tool.rbac(s.T().Context(), nil)
	s.Require().NotNil(meta.Bounded)
	s.Require().Len(meta.Bounded.Requirements, 1)
	s.True(meta.Bounded.Requirements[0].Namespace.AllNamespaces)
	s.Empty(meta.Bounded.Requirements[0].Namespace.Argument)
	scoped, err := newCompiledTool(Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d",
		Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "apps/v1", Resource: "deployments", Namespace: &NamespaceRef{Argument: "namespace"}}}})
	s.Require().NoError(err)
	s.Equal("namespace", scoped.rbac(s.T().Context(), nil).Bounded.Requirements[0].Namespace.Argument, "without all: true the argument scopes the request")
}

func (s *DynamicSuite) TestOutputBudget() {
	items := func(n int) []any {
		out := make([]any, 0, n)
		for i := range n {
			out = append(out, map[string]any{"name": fmt.Sprintf("pod-%d", i)})
		}
		return out
	}
	schema := json.RawMessage(`{"type": "object", "properties": {"labelSelector": {"type": "string"}, "namespace": {"type": "string"}}}`)
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "pods"}, Spec: ToolSpec{Description: "d", InputSchema: schema,
		Limit:      &Limit{Default: 2, Max: 5},
		Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Resource: "pods"}}})
	s.Require().NoError(err)
	s.Run("a list beyond the limit is cut and the trailer says how to narrow it", func() {
		result, trailer := tool.applyLimit(items(7), map[string]any{}, "")
		s.Len(result, 2)
		s.Equal("showing 2 of 7 items; narrow with labelSelector, or raise limit (at most 5)", trailer)
	})
	s.Run("the model's limit applies, clamped to the maximum", func() {
		result, trailer := tool.applyLimit(items(7), map[string]any{"limit": int64(4)}, "")
		s.Len(result, 4)
		s.Equal("showing 4 of 7 items; narrow with labelSelector, or raise limit (at most 5)", trailer)
		result, trailer = tool.applyLimit(items(7), map[string]any{"limit": float64(50)}, "")
		s.Len(result, 5)
		s.Equal("showing 5 of 7 items; narrow with labelSelector", trailer, "at the maximum there is nothing to raise")
		s.Equal(5, tool.effectiveLimit(map[string]any{"limit": int64(9)}))
		s.Equal(2, tool.effectiveLimit(map[string]any{"limit": int64(0)}), "a non-positive limit is the default")
	})
	s.Run("a list within the limit is untouched", func() {
		result, trailer := tool.applyLimit(items(2), map[string]any{}, "")
		s.Len(result, 2)
		s.Empty(trailer)
	})
	s.Run("a paginated result keeps its page and hands over the continue token", func() {
		result, trailer := tool.applyLimit(items(2), map[string]any{}, "tok123")
		s.Len(result, 2)
		s.Equal(`showing 2 items; more are available: call again with continue="tok123"`, trailer)
	})
	s.Run("a map result is cut at its declared field and says so in structured form", func() {
		metrics, err := newCompiledTool(Tool{Metadata: Metadata{Name: "metrics"}, Spec: ToolSpec{Description: "d",
			Limit: &Limit{Default: 1, Max: 10, Field: "metrics", NarrowWith: []string{"name_regex"}},
			HTTP:  &HTTPAction{URL: "https://prom.example"}, Output: "{'metrics': response.data}"}})
		s.Require().NoError(err)
		result, trailer := metrics.applyLimit(map[string]any{"metrics": []any{"a", "b", "c"}}, map[string]any{}, "")
		object := result.(map[string]any)
		s.Equal([]any{"a"}, object["metrics"])
		s.Equal(map[string]any{"returned": int64(1), "total": int64(3), "narrowWith": []string{"name_regex"}}, object["truncated"])
		s.Equal("showing 1 of 3 items; narrow with name_regex, or raise limit (at most 10)", trailer)
		untouched, trailer := metrics.applyLimit(map[string]any{"other": 1}, map[string]any{}, "")
		s.Equal(map[string]any{"other": 1}, untouched)
		s.Empty(trailer)
	})
	s.Run("a map result with one list needs no declared field", func() {
		labels, err := newCompiledTool(Tool{Metadata: Metadata{Name: "labels"}, Spec: ToolSpec{Description: "d",
			Limit: &Limit{Default: 1, Max: 10},
			HTTP:  &HTTPAction{URL: "https://prom.example"}, Output: "{'label': args.label, 'values': response.data}"}})
		s.Require().NoError(err)
		result, trailer := labels.applyLimit(map[string]any{"label": "pod", "values": []any{"a", "b"}}, map[string]any{}, "")
		s.Equal([]any{"a"}, result.(map[string]any)["values"])
		s.Equal("pod", result.(map[string]any)["label"])
		s.Equal("showing 1 of 2 items; raise limit (at most 10) for more", trailer)
		ambiguous, trailer := labels.applyLimit(map[string]any{"a": []any{1, 2}, "b": []any{3, 4}}, map[string]any{}, "")
		s.Equal(map[string]any{"a": []any{1, 2}, "b": []any{3, 4}}, ambiguous, "two lists: nothing is cut")
		s.Empty(trailer)
	})
	s.Run("the engine adds limit and continue to the schema, and fills standard descriptions", func() {
		schema, err := tool.inputSchema()
		s.Require().NoError(err)
		s.Contains(schema.Properties["limit"].Description, "default 2, at most 5")
		s.Equal(json.RawMessage("2"), schema.Properties["limit"].Default)
		s.Contains(schema.Properties["continue"].Description, "next page")
		s.Contains(schema.Properties["labelSelector"].Description, "label selector", "an undocumented standard argument gets the shared description")
		s.Contains(schema.Properties["namespace"].Description, "default namespace")
		s.Nil(schema.Properties["detail"], "no outputFull, no detail argument")
	})
	s.Run("a declared argument keeps its own definition", func() {
		own, err := newCompiledTool(Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d",
			InputSchema: json.RawMessage(`{"type": "object", "properties": {"limit": {"type": "integer", "description": "mine"}}}`),
			Limit:       &Limit{}, HTTP: &HTTPAction{URL: "https://x"}}})
		s.Require().NoError(err)
		schema, err := own.inputSchema()
		s.Require().NoError(err)
		s.Equal("mine", schema.Properties["limit"].Description)
	})
	s.Run("outputFull adds the detail argument", func() {
		detailed, err := newCompiledTool(Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d",
			HTTP: &HTTPAction{URL: "https://x"}, Output: "response.a", OutputFull: "response"}})
		s.Require().NoError(err)
		schema, err := detailed.inputSchema()
		s.Require().NoError(err)
		s.Equal([]any{"concise", "full"}, schema.Properties["detail"].Enum)
		bad := &Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", HTTP: &HTTPAction{URL: "https://x"}, OutputFull: "response"}}
		s.ErrorContains(bad.Validate(), "spec.outputFull needs spec.output")
	})
	s.Run("limit validation", func() {
		bad := &Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", HTTP: &HTTPAction{URL: "https://x"}, Limit: &Limit{Default: 10, Max: 5}}}
		s.ErrorContains(bad.Validate(), "spec.limit.default 10 exceeds spec.limit.max 5")
		filled := (&Limit{}).withDefaults()
		s.Equal(Limit{Default: DefaultLimit, Max: DefaultLimitMax, Argument: LimitArgument}, filled)
	})
}

func (s *DynamicSuite) TestDefaultProjectionAndSanitizing() {
	response := map[string]any{"kind": "PodList", "metadata": map[string]any{"continue": "next"}, "items": []any{
		map[string]any{"metadata": map[string]any{"name": "a", "namespace": "ns", "managedFields": []any{1}, "annotations": map[string]any{"kubectl.kubernetes.io/last-applied-configuration": "{}", "keep": "me"}},
			"status": map[string]any{"phase": "Running", "conditions": []any{map[string]any{"type": "Ready", "status": "True"}}}},
		map[string]any{"metadata": map[string]any{"name": "b", "annotations": map[string]any{"kubectl.kubernetes.io/last-applied-configuration": "{}"}},
			"status": map[string]any{"conditions": []any{map[string]any{"type": "Available", "status": "True"}}}},
	}}
	sanitized := sanitizeObjects(response).(map[string]any)
	first := sanitized["items"].([]any)[0].(map[string]any)["metadata"].(map[string]any)
	s.Nil(first["managedFields"])
	s.Equal(map[string]any{"keep": "me"}, first["annotations"])
	second := sanitized["items"].([]any)[1].(map[string]any)["metadata"].(map[string]any)
	s.Nil(second["annotations"], "an annotations map left empty is dropped")
	s.Equal("next", continueOf(sanitized))
	s.Equal([]any{
		map[string]any{"kind": "Pod", "name": "a", "namespace": "ns", "phase": "Running", "ready": true},
		map[string]any{"kind": "Pod", "name": "b", "condition": "Available"},
	}, defaultProjection(sanitized))
	s.Equal("not a list", defaultProjection("not a list"))
}

func (s *DynamicSuite) TestTimeVocabulary() {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for value, expected := range map[string]int64{
		"NOW": now.Unix(), "now": now.Unix(), "NOW-1h": now.Unix() - 3600, "NOW+30s": now.Unix() + 30, "NOW-1h30m": now.Unix() - 5400,
		"1700000000": 1700000000, "2026-10-14T11:00:00Z": now.Unix() - 3600, "2026-10-14T13:00:00+01:00": now.Unix(), "2026-10-14T11:00:00.5Z": now.Unix() - 3600,
	} {
		got, err := parseTime(value, now)
		s.Require().NoError(err, value)
		s.Equal(expected, got, value)
	}
	for _, value := range []string{"", "yesterday", "NOW-", "NOW-1x", "2026-10-14"} {
		_, err := parseTime(value, now)
		s.Error(err, value)
	}
	_, err := parseTime("tomorrow", now)
	s.ErrorContains(err, "must be RFC 3339 (2025-01-01T00:00:00Z), Unix seconds, NOW, or NOW minus a duration (NOW-1h)")
	for value, expected := range map[string]int64{"30s": 30, "15m": 900, "2h": 7200, "1d": 86400, "1w": 604800, "1y": 31536000, "1h30m": 5400, "1500ms": 1} {
		got, err := parseSeconds(value)
		s.Require().NoError(err, value)
		s.Equal(expected, got, value)
	}
	for _, value := range []string{"", "m", "5", "1h x", "-1h"} {
		_, err := parseSeconds(value)
		s.Error(err, value)
	}
	s.Run("as CEL functions", func() {
		expr, err := compileExpression("time('1700000000') + seconds('1h')")
		s.Require().NoError(err)
		value, err := expr.eval(evalVars{})
		s.Require().NoError(err)
		s.Equal(int64(1700003600), value)
		expr, err = compileExpression("time(args.start)")
		s.Require().NoError(err)
		_, err = expr.eval(evalVars{args: map[string]any{"start": "yesterday"}})
		s.ErrorContains(err, `time "yesterday" must be RFC 3339`)
		s.NotContains(err.Error(), "time(args.start)", "the model gets the message, not the expression")
	})
}

func (s *DynamicSuite) TestVariants() {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "svc"}, Spec: ToolSpec{
		Description: "Manage the service", Destructive: true,
		InputSchema: json.RawMessage(`{"type": "object", "properties": {"name": {"type": "string"}}, "required": ["name"]}`),
		Output:      "{'mode': args.action}",
		Variants: &Variants{Argument: "action", Description: "What to do with the service.", Options: map[string]Variant{
			"restart": {Description: "restart it in place", HTTP: &HTTPAction{URL: server.URL, Method: "POST", Path: &Ref{Value: "/restart/{name}"}}},
			"status":  {Description: "report its health", HTTP: &HTTPAction{URL: server.URL, Path: &Ref{Value: "/status/{name}"}}, Output: "{'mode': 'status', 'ok': response.ok}"},
			"remove":  {Description: "delete it", Kubernetes: &KubernetesAction{Verb: "delete", APIVersion: "v1", Resource: "services", Name: &Ref{Argument: "name"}}},
		}},
	}})
	s.Require().NoError(err)
	s.Run("the selecting argument is a required enum listing the options", func() {
		schema, err := tool.inputSchema()
		s.Require().NoError(err)
		action := schema.Properties["action"]
		s.Require().NotNil(action)
		s.Equal([]any{"remove", "restart", "status"}, action.Enum)
		s.Equal("What to do with the service. remove: delete it; restart: restart it in place; status: report its health.", action.Description)
		s.Contains(schema.Required, "action")
		s.Contains(schema.Required, "name")
	})
	params := func(args map[string]any) api.ToolHandlerParams {
		return api.ToolHandlerParams{Context: context.Background(), KubernetesClient: restConfigClient{config: &rest.Config{}}, Request: argsRequest(args)}
	}
	s.Run("the call runs the selected option with the tool's or the option's output", func() {
		result, err := tool.handle(params(map[string]any{"action": "restart", "name": "web"}))
		s.Require().NoError(err)
		s.Require().NoError(result.Error, result.Content)
		s.Equal("mode: restart\n", result.Content)
		result, err = tool.handle(params(map[string]any{"action": "status", "name": "web"}))
		s.Require().NoError(err)
		s.Require().NoError(result.Error, result.Content)
		s.Equal("mode: status\nok: true\n", result.Content)
		mu.Lock()
		defer mu.Unlock()
		s.Equal([]string{"POST /restart/web", "GET /status/web"}, paths)
	})
	s.Run("an unknown or missing option is refused by name", func() {
		result, err := tool.handle(params(map[string]any{"action": "explode", "name": "web"}))
		s.Require().NoError(err)
		s.ErrorContains(result.Error, "svc: action must be one of remove, restart, status")
		result, err = tool.handle(params(map[string]any{"name": "web"}))
		s.Require().NoError(err)
		s.ErrorContains(result.Error, "action must be one of")
	})
	s.Run("the RBAC metadata is the union of the options", func() {
		meta := tool.rbac(context.Background(), nil)
		s.Require().NotNil(meta.Bounded)
		s.Require().Len(meta.Bounded.Requirements, 1, "the kubernetes delete")
		s.Equal([]string{"delete"}, meta.Bounded.Requirements[0].Verbs)
		s.Contains(meta.Bounded.Reason, server.URL)
	})
	s.Run("identity reaches every option", func() {
		tool.setIdentity(IdentityServer, nil)
		for _, variant := range tool.variants {
			s.True(variant.actsAsServer())
		}
	})
	s.Run("validation", func() {
		base := func() *Tool {
			return &Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", Variants: &Variants{Argument: "action", Options: map[string]Variant{
				"a": {Description: "a", Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Kind: "Pod"}},
			}}}}
		}
		s.NoError(base().Validate())
		bad := base()
		bad.Spec.Kubernetes = &KubernetesAction{Verb: "list", APIVersion: "v1", Kind: "Pod"}
		s.ErrorContains(bad.Validate(), "mutually exclusive")
		bad = base()
		bad.Spec.Variants.Options = map[string]Variant{}
		s.ErrorContains(bad.Validate(), "at least one option")
		bad = base()
		bad.Spec.Variants.Options["b"] = Variant{Description: "b"}
		s.ErrorContains(bad.Validate(), "spec.variants.options[b] needs one of kubernetes, http or steps")
		bad = base()
		bad.Spec.Variants.Options["b"] = Variant{Kubernetes: &KubernetesAction{Verb: "get", APIVersion: "v1", Kind: "Pod", Name: &Ref{Argument: "name"}}}
		s.ErrorContains(bad.Validate(), "spec.variants.options[b].description is required")
		bad = base()
		bad.Spec.ReadOnly = true
		bad.Spec.Variants.Options["b"] = Variant{Description: "b", Kubernetes: &KubernetesAction{Verb: "delete", APIVersion: "v1", Kind: "Pod", Name: &Ref{Argument: "name"}}}
		s.ErrorContains(bad.Validate(), "spec.variants.options[b] performs a delete but spec.readOnly is true")
		bad = base()
		bad.Spec.Variants.Argument = "not valid"
		s.ErrorContains(bad.Validate(), "spec.variants.argument")
	})
}

func (s *DynamicSuite) TestBindings() {
	expr, err := compileExpression("cel.bind(doubled, [1, 2, 3].map(i, i * 2), sum(doubled) / double(size(doubled)))")
	s.Require().NoError(err)
	value, err := expr.eval(evalVars{})
	s.Require().NoError(err)
	s.Equal(4.0, value)
	// A binding does not hide an argument reference from the urlExpression check.
	bound, err := compileExpression("cel.bind(host, args.host, 'https://' + host)")
	s.Require().NoError(err)
	s.True(bound.references("args"))
}

func (s *DynamicSuite) TestQueryListsAndSum() {
	s.Run("list values become repeated parameters", func() {
		expr, err := compileExpression("{'match[]': ['up', 'node_load1'], 'limit': 1, 'skip': null, 'empty': []}")
		s.Require().NoError(err)
		values, err := expr.evalStringMap(evalVars{})
		s.Require().NoError(err)
		s.Equal(map[string][]string{"match[]": {"up", "node_load1"}, "limit": {"1"}}, values)
	})
	s.Run("sum folds a list of numbers", func() {
		expr, err := compileExpression("sum([1, 2.5, 3])")
		s.Require().NoError(err)
		value, err := expr.eval(evalVars{})
		s.Require().NoError(err)
		s.Equal(6.5, value)
		expr, err = compileExpression("sum([])")
		s.Require().NoError(err)
		value, err = expr.eval(evalVars{})
		s.Require().NoError(err)
		s.Equal(0.0, value)
		expr, err = compileExpression("sum(['a'])")
		s.Require().NoError(err)
		_, err = expr.eval(evalVars{})
		s.ErrorContains(err, "must be numbers")
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
		s.Equal([]string{"app in (a, b)"}, query["labelSelector"])
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
		s.Require().Len(out["end"], 1)
		s.Require().Len(out["start"], 1)
		s.Regexp(`^\d{10}$`, out["end"][0])
		s.Regexp(`^\d{10}$`, out["start"][0])
	})
	s.Run("range refuses to allocate unbounded lists", func() {
		expr, err := compileExpression("size(range(args.n))")
		s.Require().NoError(err)
		out, err := expr.eval(evalVars{args: map[string]any{"n": float64(MaxRange)}})
		s.Require().NoError(err)
		s.Equal(int64(MaxRange), out)
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
		s.Equal(map[string][]string{"a": {"1"}, "b": {"x"}, "e": {"true"}, "f": {"1.5"}}, out)
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

func (s *DynamicSuite) TestItemBelongsToItsForEachStep() {
	// item binds only inside the forEach step: a later expression that
	// references it must fail rather than silently see the last element.
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "scoped"}, Spec: ToolSpec{
		Description: "d",
		Steps: []Step{
			{Name: "each", Expression: "item", ForEach: "[1, 2, 3]"},
			{Name: "after", Expression: "item"},
		},
	}})
	s.Require().NoError(err)
	_, _, err = tool.runSteps(runEnv{ctx: context.Background()}, &evalVars{args: map[string]any{}, steps: map[string]any{}})
	s.Require().Error(err)
	s.Contains(err.Error(), "after")
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
	s.Equal(map[string]any{"a": int64(1)}, decodeResponseAs([]byte(`{"a":1}`), "", "application/json", "", false))
}

func (s *DynamicSuite) TestRBACOfMixedSteps() {
	tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "mixed"}, Spec: ToolSpec{
		Description: "d",
		Steps: []Step{
			{Name: "pods", Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Resource: "pods", Namespace: &NamespaceRef{Value: "istio-system"}}},
			{Name: "mesh", HTTP: &HTTPAction{URL: "https://kiali.example.com", Path: &Ref{Value: "/api/mesh"}}},
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

func (s *DynamicSuite) TestFamiliesShareOnePollLoop() {
	// Every enabled family of a server asks for the same watch; the loop
	// is shared and refcounted, so one family's stop leaves it running for
	// the others and the last stop ends it.
	SetPollInterval(time.Millisecond)
	defer SetPollInterval(DefaultPollInterval)
	inspector := &fakeInspector{names: map[string]string{"a-tool": "1"}}
	onChange := func() error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	toolset := &Toolset{}
	stopOne := toolset.WatchToolset(ctx, api.ToolsetContext{Inspector: inspector}, onChange)
	stopTwo := (&familyToolset{Toolset: toolset, family: "a"}).WatchToolset(ctx, api.ToolsetContext{Inspector: inspector}, onChange)

	stopOne()
	counts := make(chan int32, 1)
	go func() { counts <- inspector.lists.Load() }()
	time.Sleep(50 * time.Millisecond)
	s.Greater(inspector.lists.Load(), <-counts, "one stop leaves the shared loop running for the other family")

	stopTwo()
	go func() { counts <- inspector.lists.Load() }()
	time.Sleep(50 * time.Millisecond)
	s.Equal(<-counts, inspector.lists.Load(), "the last stop ends the loop")
}

func (s *DynamicSuite) TestHTTPHostAllowlist() {
	s.Run("the matcher", func() {
		allowed := []string{"kiali.svc", "*.apps.example.com", "*.wild-port.example:8443", "with-port.svc:20001", "  spaced.svc  "}
		s.True(httpHostAllowed(nil, "anything.example"), "an empty list allows every host")
		s.True(httpHostAllowed(allowed, "kiali.svc"))
		s.True(httpHostAllowed(allowed, "KIALI.SVC:20001"), "the host matches case-insensitively, the port is ignored")
		s.True(httpHostAllowed(allowed, "team.apps.example.com"), "a wildcard matches a subdomain")
		s.True(httpHostAllowed(allowed, "team.wild-port.example"), "a wildcard's port is ignored too")
		s.False(httpHostAllowed(allowed, "apps.example.com"), "the bare domain must be listed separately")
		s.False(httpHostAllowed(allowed, "evil-apps.example.com"), "a look-alike suffix is not a subdomain")
		s.True(httpHostAllowed(allowed, "with-port.svc:9999"), "the entry's port is not a security boundary")
		s.True(httpHostAllowed(allowed, "spaced.svc"), "entries are trimmed")
		s.False(httpHostAllowed(allowed, "other.svc"))
	})
	s.Run("a fixed url outside the list is withheld at registration", func() {
		var lines []string
		logger := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{Verbosity: 0})
		ctx := klog.NewContext(s.T().Context(), logger)
		inspector := &fakeInspector{
			names: map[string]string{"denied-url": "1", "allowed-url": "1"},
			toolSpec: map[string]map[string]any{
				"denied-url":  {"kubernetes": nil, "http": map[string]any{"url": "https://denied.example.com"}},
				"allowed-url": {"kubernetes": nil, "http": map[string]any{"url": "https://allowed.svc"}},
			},
		}
		toolsetContext := api.ToolsetContext{Inspector: inspector, AllowedHTTPHosts: []string{"allowed.svc"}}
		var names []string
		for _, tool := range (&Toolset{}).GetTools(ctx, toolsetContext) {
			names = append(names, tool.Tool.Name)
		}
		s.Equal([]string{"allowed_url"}, names)
		s.Require().Len(lines, 1, "one line for the withheld resource: %v", lines)
		s.Contains(lines[0], "denied-url")
		s.Contains(lines[0], "denied.example.com")
	})
	s.Run("fixed urls are collected from the action, the steps and the variants", func() {
		spec := ToolSpec{
			Description: "d",
			Steps: []Step{
				{Name: "call", HTTP: &HTTPAction{URL: "https://steps.example.com"}},
				{Name: "skip", HTTP: &HTTPAction{URLExpression: "'https://computed.example.com'"}},
			},
			Variants: &Variants{
				Argument: "action",
				Options: map[string]Variant{
					"one": {HTTP: &HTTPAction{URL: "https://variant.example.com"}},
					"two": {Steps: []Step{{Name: "inner", HTTP: &HTTPAction{URL: "https://nested.example.com"}}}},
				},
			},
		}
		s.ElementsMatch([]string{
			"https://steps.example.com", "https://variant.example.com", "https://nested.example.com",
		}, fixedHTTPURLs(spec))
		s.Empty(fixedHTTPURLs(ToolSpec{Description: "d", Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Kind: "Pod"}}))
	})
	s.Run("a fixed url in a step is withheld at registration too", func() {
		var lines []string
		logger := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{Verbosity: 0})
		ctx := klog.NewContext(s.T().Context(), logger)
		inspector := &fakeInspector{
			names: map[string]string{"step-url": "1"},
			toolSpec: map[string]map[string]any{
				"step-url": {"kubernetes": nil, "steps": []any{map[string]any{
					"name": "call",
					"http": map[string]any{"url": "https://denied.example.com"},
				}}},
			},
		}
		toolsetContext := api.ToolsetContext{Inspector: inspector, AllowedHTTPHosts: []string{"allowed.svc"}}
		s.Empty((&Toolset{}).GetTools(ctx, toolsetContext))
		s.Require().Len(lines, 1, "one line for the withheld resource: %v", lines)
		s.Contains(lines[0], "step-url")
		s.Contains(lines[0], "denied.example.com")
	})
	s.Run("a prompt with a fixed url in a step is withheld at registration too", func() {
		var lines []string
		logger := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{Verbosity: 0})
		ctx := klog.NewContext(s.T().Context(), logger)
		inspector := &fakeInspector{
			prompts: map[string]string{"denied-prompt": "1"},
			promptSpec: map[string]map[string]any{"denied-prompt": {"steps": []any{map[string]any{
				"name": "call",
				"http": map[string]any{"url": "https://denied.example.com"},
			}}}},
		}
		toolsetContext := api.ToolsetContext{Inspector: inspector, AllowedHTTPHosts: []string{"allowed.svc"}}
		s.Empty((&Toolset{}).GetPrompts(ctx, toolsetContext))
		s.Require().Len(lines, 1, "one line for the withheld prompt: %v", lines)
		s.Contains(lines[0], "denied-prompt")
		s.Contains(lines[0], "denied.example.com")
	})
	s.Run("every request is checked at call time", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()
		tool, err := newCompiledTool(Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", HTTP: &HTTPAction{URL: server.URL}}})
		s.Require().NoError(err)
		env := runEnv{ctx: context.Background(), client: restConfigClient{config: &rest.Config{}}}
		env.allowedHosts = func() []string { return []string{"allowed.svc"} }
		_, err = tool.run(tool.action, env, evalVars{})
		s.ErrorContains(err, "not in the server's allowed_http_hosts")
		env.allowedHosts = func() []string { return []string{hostOf(server.URL)} }
		_, err = tool.run(tool.action, env, evalVars{})
		s.Require().NoError(err)
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
	s.Require().Len(lines, 3, "one line per failed list (tools, toolsets, prompts): %v", lines)
	s.Contains(lines[0], "unable to list resources, exposing none")
	s.Contains(lines[0], `"resource"="tools"`)
	s.Contains(lines[0], "is forbidden")
	s.Contains(lines[1], `"resource"="toolsets"`)
	s.Contains(lines[2], `"resource"="prompts"`)
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
		s.Require().Len(lines, 3, "one per kind: tools, toolsets, prompts: %v", lines)
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
	// (say, make pods_delete look read-only) by creating a Tool of that name —
	// not even when the toolset that owns the name is disabled.
	toolsets.Register(&builtInToolset{})
	defer toolsets.Clear()
	var lines []string
	logger := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{Verbosity: 0})
	ctx := klog.NewContext(s.T().Context(), logger)
	inspector := &fakeInspector{
		names:   map[string]string{"builtin-tool": "1", "other-tool": "1"},
		prompts: map[string]string{"builtin-prompt": "1", "other-prompt": "1"},
	}
	toolsetContext := api.ToolsetContext{Inspector: inspector}

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
	// toolSpec overrides spec fields of the named tools.
	toolSpec map[string]map[string]any
	// toolsets are ToolSet resources by name with their spec fields.
	toolsets map[string]map[string]any
	err      error
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
		if r.gvr == ToolSetGVR {
			for name, fields := range r.inspector.toolsets {
				spec := map[string]any{"description": "d"}
				for key, value := range fields {
					spec[key] = value
				}
				list.Items = append(list.Items, unstructured.Unstructured{Object: map[string]any{
					"apiVersion": Group + "/" + Version, "kind": ToolSetKind,
					"metadata": map[string]any{"name": name, "resourceVersion": "1"},
					"spec":     spec,
				}})
			}
			return list, nil
		}
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
			spec := map[string]any{"description": "d", "kubernetes": map[string]any{"verb": "get", "apiVersion": "v1", "kind": "Pod"}}
			for key, value := range r.inspector.toolSpec[name] {
				spec[key] = value
			}
			list.Items = append(list.Items, unstructured.Unstructured{Object: map[string]any{
				"apiVersion": Group + "/" + Version, "kind": "Tool",
				"metadata": map[string]any{"name": name, "resourceVersion": version},
				"spec":     spec,
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

func (s *DynamicSuite) TestVariantsDefault() {
	tool := Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d",
		Variants: &Variants{Argument: "info", Default: "all", Options: map[string]Variant{
			"all": {Description: "everything", HTTP: &HTTPAction{URL: "https://x", Path: &Ref{Value: "/all"}}},
			"os":  {Description: "the os", HTTP: &HTTPAction{URL: "https://x", Path: &Ref{Value: "/os"}}},
		}}}}
	s.Run("the argument is optional and documented with its default", func() {
		compiled, err := newCompiledTool(tool)
		s.Require().NoError(err)
		schema, err := compiled.inputSchema()
		s.Require().NoError(err)
		s.NotContains(schema.Required, "info")
		s.Equal(json.RawMessage(`"all"`), schema.Properties["info"].Default)
		s.Contains(schema.Properties["info"].Description, "Defaults to all.")
	})
	s.Run("an example may omit the argument", func() {
		tool.Spec.Examples = []Example{{Summary: "everything"}}
		s.NoError(tool.Validate())
	})
	s.Run("the default must be an option", func() {
		bad := tool
		bad.Spec.Variants = &Variants{Argument: "info", Default: "nope", Options: tool.Spec.Variants.Options}
		s.ErrorContains(bad.Validate(), `spec.variants.default "nope" is not one of the options`)
	})
}

func (s *DynamicSuite) TestLintsAndExamples() {
	schema := json.RawMessage(`{"type": "object", "required": ["query"], "properties": {
		"query": {"type": "string", "description": "A PromQL query."},
		"mode": {"type": "string", "description": "Either 'fast' or 'slow'."},
		"namespace": {"type": "string"},
		"raw": {"type": "string"},
		"raw_value": {"type": "string", "description": "Spelled wrong."}}}`)
	s.Run("a tool that follows the guidance has no lint", func() {
		tool := Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "Answers one question about the cluster.",
			InputSchema: json.RawMessage(`{"type": "object", "properties": {"query": {"type": "string", "description": "A query."}}}`),
			HTTP:        &HTTPAction{URL: "https://x"}, Output: "response",
			Examples: []Example{{Summary: "the question", Arguments: map[string]any{"query": "up"}}}}}
		s.NoError(tool.Validate())
		s.Empty(tool.Lint())
	})
	s.Run("the lints name what the guidance asks for", func() {
		tool := Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "Lists.", InputSchema: schema,
			Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Resource: "pods"}}}
		s.NoError(tool.Validate(), "lints are advice, not validation")
		s.Equal([]string{
			"description is shorter than a sentence: say what question the tool answers and when to prefer it",
			"argument mode spells out its values in prose; declare them as enum",
			"argument raw has no description",
			"argument raw_value is not camelCase",
			"no examples: one example call shows the model how to fill the arguments",
			"spec.kubernetes lists without output or limit: the result is the default projection of every item",
		}, tool.Lint(), "namespace is a standard argument and needs no description")
	})
	s.Run("examples are checked against the schema", func() {
		tool := Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d", InputSchema: schema, Limit: &Limit{},
			HTTP: &HTTPAction{URL: "https://x"},
			Examples: []Example{
				{Summary: "ok", Arguments: map[string]any{"query": "up", "limit": 3}},
				{Arguments: map[string]any{"typo": 1}},
			}}}
		err := tool.Validate()
		s.ErrorContains(err, "spec.examples[1].summary is required")
		s.ErrorContains(err, `spec.examples[1] lacks the required argument "query"`)
		s.ErrorContains(err, "spec.examples[1].arguments.typo is not a declared argument")
		s.NotContains(err.Error(), "spec.examples[0]", "limit is an argument the engine adds")
	})
	s.Run("an example of a variants tool gives the variants argument, with one of its options", func() {
		tool := Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d",
			InputSchema: json.RawMessage(`{"type": "object", "properties": {"name": {"type": "string"}}}`),
			Variants: &Variants{Argument: "action", Options: map[string]Variant{
				"start": {Description: "start it", Kubernetes: &KubernetesAction{Verb: "get", APIVersion: "v1", Resource: "pods", Name: &Ref{Argument: "name"}}},
			}},
			Examples: []Example{{Summary: "start", Arguments: map[string]any{"name": "a", "action": "stop"}}, {Summary: "no action", Arguments: map[string]any{"name": "a"}}}}}
		err := tool.Validate()
		s.ErrorContains(err, "spec.examples[0].arguments.action is stop, not one of [start]")
		s.ErrorContains(err, `spec.examples[1] lacks the required argument "action"`)
		tool.Spec.Examples = []Example{{Summary: "start", Arguments: map[string]any{"name": "a", "action": "start"}}}
		s.NoError(tool.Validate())
		compiled, err := newCompiledTool(tool)
		s.Require().NoError(err)
		s.Empty(compiled.variants["start"].spec.Examples, "the option does not carry the parent's examples")
	})
	s.Run("examples are rendered into the description in one form", func() {
		tool := api.Tool{Description: "Does a thing.  ", Examples: []Example{
			{Summary: "a", Arguments: map[string]any{"query": "up", "limit": 3}},
			{Summary: "b"},
		}}
		s.Equal("Does a thing.\n\nExample: a: {\"limit\":3,\"query\":\"up\"}\n\nExample: b: {}", tool.Describe())
	})
	s.Run("forwarding the model's arguments wholesale is refused", func() {
		tool := Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d",
			Steps: []Step{{Name: "q", Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Resource: "pods", QueryExpression: " args "}},
				{Name: "b", Kubernetes: &KubernetesAction{Verb: "create", APIVersion: "v1", Resource: "pods", Body: &Body{Expression: "args"}}}}}}
		err := tool.Validate()
		s.ErrorContains(err, "spec.steps[0] (q).queryExpression forwards the model's arguments wholesale (args); declare the parameters the tool means")
		s.ErrorContains(err, "spec.steps[1] (b).body.expression forwards the model's arguments wholesale (args); declare the fields the tool means")
		fine := Tool{Metadata: Metadata{Name: "t"}, Spec: ToolSpec{Description: "d",
			Kubernetes: &KubernetesAction{Verb: "list", APIVersion: "v1", Resource: "pods", QueryExpression: "{'labelSelector': args.selector}"}}}
		s.NoError(fine.Validate())
	})
}

func (s *DynamicSuite) TestFamiliesUnavailable() {
	// A tool naming a family while ToolSet resources cannot be listed is told
	// that, with the RBAC the server's identity needs, rather than that its
	// family does not exist.
	inspector := &fakeInspector{names: map[string]string{}}
	inspector.setErr(apierrors.NewForbidden(schema.GroupResource{Group: Group, Resource: "toolsets"}, "", errors.New("forbidden")))
	index := (&Toolset{}).listFamilies(s.T().Context(), api.ToolsetContext{Inspector: inspector}, "", false)
	s.True(index.unavailable)
	_, mine, reason := index.familyOf("loki", "loki_labels", "_", "")
	s.False(mine)
	s.Contains(reason, "names a ToolSet resource, but ToolSet resources could not be listed: loki")
	s.Contains(reason, "rbac.dynamicTools: true")
	s.Equal(ReasonUnknownToolSet, familyReason(reason))
	inspector.setErr(nil)
	index = (&Toolset{}).listFamilies(s.T().Context(), api.ToolsetContext{Inspector: inspector}, "", false)
	s.False(index.unavailable)
	_, _, reason = index.familyOf("loki", "loki_labels", "_", "")
	s.Equal("names a ToolSet resource that does not exist: loki", reason)
}
