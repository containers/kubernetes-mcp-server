package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicclient "k8s.io/client-go/dynamic"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

// helm returns the helm executable, or skips the test when none is available.
func (s *DynamicToolsSuite) helm() string {
	if helm, err := exec.LookPath("helm"); err == nil {
		return helm
	}
	helm := filepath.Join("..", "..", "_output", "tools", "bin", "helm")
	if _, err := os.Stat(helm); err != nil {
		s.T().Skip("helm not available (make helm)")
	}
	return helm
}

// loadChart renders a chart under charts/dynamic-tools with the
// given --set values and creates every Tool it produces.
func (s *DynamicToolsSuite) loadChart(chart string, values ...string) {
	args := []string{"template", "x", filepath.Join("..", "..", "charts", "dynamic-tools", chart)}
	for _, value := range values {
		args = append(args, "--set", value)
	}
	out, err := exec.Command(s.helm(), args...).CombinedOutput()
	s.Require().NoError(err, string(out))
	for _, doc := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n---") {
		switch {
		case strings.Contains(doc, "kind: ToolSet"):
			s.createToolSet(doc)
		case strings.Contains(doc, "kind: Tool"):
			s.createTool(doc)
		}
	}
}

type recordedRequest struct {
	Method, URI, Tenant, Accept string
}

func (r recordedRequest) query() url.Values {
	u, err := url.ParseRequestURI(r.URI)
	if err != nil {
		return nil
	}
	return u.Query()
}

// observabilityBackend fakes Loki, Tempo, Prometheus and Alertmanager behind
// one TLS server, keyed by the request path each client uses.
func (s *DynamicToolsSuite) observabilityBackend() (*httptest.Server, func() []recordedRequest) {
	var mu sync.Mutex
	var requests []recordedRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, recordedRequest{r.Method, r.URL.RequestURI(), r.Header.Get("X-Scope-OrgID"), r.Header.Get("Accept")})
		mu.Unlock()
		var body any
		switch {
		case strings.HasSuffix(r.URL.Path, "/loki/api/v1/labels"):
			body = map[string]any{"status": "success", "data": []string{"namespace", "pod"}}
		case strings.HasSuffix(r.URL.Path, "/loki/api/v1/label/pod/values"):
			body = map[string]any{"status": "success", "data": []string{"a", "b"}}
		case strings.HasSuffix(r.URL.Path, "/api/v2/search/tags"):
			body = map[string]any{"scopes": []any{map[string]any{"name": "resource", "tags": []string{"service.name"}}}}
		case strings.HasSuffix(r.URL.Path, "/api/v2/search/tag/resource.service.name/values"):
			body = map[string]any{"tagValues": []any{map[string]any{"type": "string", "value": "payments"}}}
		case r.URL.Path == "/api/v1/query":
			body = map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": []any{
				map[string]any{"metric": map[string]string{"pod": "a"}, "value": []any{1700000000.0, "1"}},
			}}}
		case r.URL.Path == "/api/v1/labels":
			body = map[string]any{"status": "success", "data": []string{"pod", "namespace"}}
		case r.URL.Path == "/api/v1/label/pod/values":
			body = map[string]any{"status": "success", "data": []string{"a", "b", "c"}}
		case strings.HasSuffix(r.URL.Path, "/loki/api/v1/query_range"):
			body = map[string]any{"status": "success", "data": map[string]any{"resultType": "streams", "result": []any{
				map[string]any{"stream": map[string]string{"pod": "a"}, "values": [][]string{{"1700000000000000000", "hello"}, {"truncated"}}},
			}}}
		case strings.HasSuffix(r.URL.Path, "/api/search"):
			body = map[string]any{"traces": []any{map[string]any{"traceID": "abc"}}, "metrics": map[string]any{"inspectedBytes": "1"}}
		case strings.HasSuffix(r.URL.Path, "/api/v2/traces/missing"):
			body = map[string]any{"trace": nil}
		case strings.Contains(r.URL.Path, "/api/v2/traces/"):
			body = map[string]any{"trace": map[string]any{"resourceSpans": []any{}}}
		case r.URL.Path == "/api/v1/query_range":
			body = map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{
				map[string]any{"metric": map[string]string{"pod": "a"}, "values": []any{[]any{1700000000.0, "1"}, []any{1700000060.0, "NaN"}, []any{1700000120.0, "4"}}},
			}}}
		case r.URL.Path == "/api/v1/label/__name__/values":
			body = map[string]any{"status": "success", "data": []string{"up"}}
		case r.URL.Path == "/api/v2/alerts":
			body = []any{map[string]any{
				"labels": map[string]string{"alertname": "Down"}, "annotations": map[string]string{"summary": "down"},
				"startsAt": "2025-01-01T00:00:00Z", "endsAt": "0001-01-01T00:00:00Z", "fingerprint": "f",
				"status": map[string]any{"state": "active", "silencedBy": []string{}, "inhibitedBy": []string{}},
			}}
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	return server, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), requests...)
	}
}

// TestConvertedObservabilityTools renders the Loki, Tempo and Prometheus
// charts and runs them against envtest (LokiStack, TempoStack and Route
// resources) and a fake backend: instance discovery through Routes, tenant
// handling, time windows, repeated query parameters and series statistics.
func (s *DynamicToolsSuite) TestConvertedObservabilityTools() {
	ctx := s.T().Context()
	server, requests := s.observabilityBackend()
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "https://")
	lokiStacks := schema.GroupVersionResource{Group: "loki.grafana.com", Version: "v1", Resource: "lokistacks"}
	tempoStacks := schema.GroupVersionResource{Group: "tempo.grafana.com", Version: "v1alpha1", Resource: "tempostacks"}
	routes := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
	client := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig())
	defer func() {
		for _, gvr := range []schema.GroupVersionResource{lokiStacks, tempoStacks, routes} {
			_ = client.Resource(gvr).Namespace("default").DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
		}
	}()
	s.createUnstructured(lokiStacks, `
apiVersion: loki.grafana.com/v1
kind: LokiStack
metadata: {name: logging}
spec: {tenants: {mode: openshift-logging}}
status: {conditions: [{type: Pending, status: "False"}, {type: Ready, status: "True"}]}
`)
	s.createUnstructured(routes, `
apiVersion: route.openshift.io/v1
kind: Route
metadata: {name: logging-gateway-http}
spec: {host: "`+host+`", to: {name: logging-gateway-http}}
`)
	s.createUnstructured(tempoStacks, `
apiVersion: tempo.grafana.com/v1alpha1
kind: TempoStack
metadata: {name: simplest}
spec: {tenants: {mode: openshift, authentication: [{tenantName: dev}]}}
status: {conditions: [{type: Ready, status: "True"}]}
`)
	s.createUnstructured(routes, `
apiVersion: route.openshift.io/v1
kind: Route
metadata: {name: tempo-simplest-gateway}
spec: {host: "`+host+`", to: {name: tempo-simplest-gateway}}
`)
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	s.loadChart("loki-mcp-tools", "useRoute=true", "insecureSkipTLSVerify=true")
	s.loadChart("tempo-mcp-tools", "useRoute=true", "insecureSkipTLSVerify=true")
	s.loadChart("prometheus-mcp-tools", "url="+server.URL, "alertmanagerUrl="+server.URL, "insecureSkipTLSVerify=true")
	s.InitMcpClient()
	last := func() recordedRequest {
		all := requests()
		s.Require().NotEmpty(all)
		return all[len(all)-1]
	}

	s.Run("loki_instances resolves the gateway Route and the stack status", func() {
		result, err := s.CallTool("loki_instances", nil)
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "lokiName: logging")
		s.Contains(s.text(result), "status: Ready")
		s.Contains(s.text(result), "url: https://"+host+"/api/logs/v1")
	})
	s.Run("loki_labels queries the discovered gateway as the tenant", func() {
		result, err := s.CallTool("loki_labels", map[string]any{"lokiNamespace": "default", "lokiName": "logging", "tenant": "application", "start": "NOW-1h", "end": "now"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "- namespace\n")
		req := last()
		s.True(strings.HasPrefix(req.URI, "/api/logs/v1/application/loki/api/v1/labels?"), req.URI)
		s.Equal("application", req.Tenant)
		s.Regexp(`^\d{19}$`, req.query().Get("start"), "Loki takes nanosecond timestamps")
		s.Regexp(`^\d{19}$`, req.query().Get("end"))
	})
	s.Run("loki_labels with a label lists its values", func() {
		result, err := s.CallTool("loki_labels", map[string]any{"lokiNamespace": "default", "lokiName": "logging", "tenant": "application", "label": "pod"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "label: pod\n")
		s.Contains(s.text(result), "- a\n")
		s.True(strings.HasPrefix(last().URI, "/api/logs/v1/application/loki/api/v1/label/pod/values?"), last().URI)
	})
	s.Run("loki_labels reports argument errors like the in-tree tool", func() {
		result, err := s.CallTool("loki_labels", map[string]any{"lokiNamespace": "default", "lokiName": "logging", "start": "NOW"})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), "both start and end must be provided together")
		result, err = s.CallTool("loki_labels", map[string]any{"lokiNamespace": "default"})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), "both lokiNamespace and lokiName must be provided together")
		result, err = s.CallTool("loki_labels", map[string]any{"lokiNamespace": "default", "lokiName": "nope"})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), "not found")
	})
	s.Run("loki_query caps the limit and keeps the stream shape", func() {
		result, err := s.CallTool("loki_query", map[string]any{"lokiNamespace": "default", "lokiName": "logging", "query": `{pod="a"}`, "limit": 5000, "duration": "5m"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		req := last()
		s.True(strings.HasPrefix(req.URI, "/api/logs/v1/loki/api/v1/query_range?"), "no tenant, no tenant segment: %s", req.URI)
		s.Empty(req.Tenant)
		s.Equal("1000", req.query().Get("limit"))
		s.Equal("backward", req.query().Get("direction"))
		s.Equal(`{pod="a"}`, req.query().Get("query"))
		s.Contains(s.text(result), "resultType: streams")
		s.Contains(s.text(result), "line: hello")
		s.Equal(1, strings.Count(s.text(result), "timestamp:"), "a malformed value row is dropped")
	})
	s.Run("tempo_instances reports tenants", func() {
		result, err := s.CallTool("tempo_instances", nil)
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "kind: TempoStack")
		s.Contains(s.text(result), "multitenancy: true")
		s.Contains(s.text(result), "- dev")
		s.NotContains(s.text(result), "service:")
	})
	s.Run("tempo_search goes through the tenant gateway", func() {
		result, err := s.CallTool("tempo_search", map[string]any{"tempoNamespace": "default", "tempoName": "simplest", "tenant": "dev", "query": "{ status=error }", "start": "NOW-1h", "end": "NOW", "limit": 20})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "traceID: abc")
		req := last()
		s.True(strings.HasPrefix(req.URI, "/api/traces/v1/dev/tempo/api/search?"), req.URI)
		s.Equal("application/vnd.grafana.llm", req.Accept)
		s.Equal("{ status=error }", req.query().Get("q"))
		s.Equal("20", req.query().Get("limit"))
		s.Empty(req.query().Get("spss"))
		s.Regexp(`^\d{10}$`, req.query().Get("start"), "Tempo takes second timestamps")
	})
	s.Run("tempo tools reject tenant and limit errors like the in-tree tools", func() {
		result, err := s.CallTool("tempo_search", map[string]any{"tempoNamespace": "default", "tempoName": "simplest", "tenant": "prod", "query": "{}"})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), "tenant 'prod' does not exist for instance 'simplest' in namespace 'default'")
		result, err = s.CallTool("tempo_search", map[string]any{"tempoNamespace": "default", "tempoName": "simplest", "tenant": "dev", "query": "{}", "limit": 5000})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), "requested search results limit 5000 is greater than max limit 1000")
		result, err = s.CallTool("tempo_trace", map[string]any{"tempoNamespace": "default", "tempoName": "simplest", "tenant": "dev", "traceId": "missing"})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), "trace missing not found")
	})
	s.Run("tempo_tags lists names, and the values of one tag", func() {
		result, err := s.CallTool("tempo_tags", map[string]any{"tempoNamespace": "default", "tempoName": "simplest", "tenant": "dev", "scope": "resource"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "- service.name\n")
		s.True(strings.HasPrefix(last().URI, "/api/traces/v1/dev/tempo/api/v2/search/tags?"), last().URI)
		s.Equal("resource", last().query().Get("scope"))
		result, err = s.CallTool("tempo_tags", map[string]any{"tempoNamespace": "default", "tempoName": "simplest", "tenant": "dev", "tag": "resource.service.name", "scope": "resource"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "value: payments")
		s.Equal("/api/traces/v1/dev/tempo/api/v2/search/tag/resource.service.name/values", strings.SplitN(last().URI, "?", 2)[0])
		s.Empty(last().query().Get("scope"), "scope only applies to names")
	})
	s.Run("prom_query without a range is an instant query", func() {
		result, err := s.CallTool("prom_query", map[string]any{"query": "up"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		req := last()
		s.Equal("/api/v1/query", strings.SplitN(req.URI, "?", 2)[0])
		s.Regexp(`^\d{10}$`, req.query().Get("time"))
		s.Contains(s.text(result), "resultType: vector")
		s.Contains(s.text(result), "pod: a")
	})
	s.Run("prom_query with a range derives the step", func() {
		result, err := s.CallTool("prom_query", map[string]any{"query": "up", "duration": "1h"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Equal("60s", last().query().Get("step"), "a sixtieth of the hour")
		result, err = s.CallTool("prom_query", map[string]any{"query": "up", "duration": "5m"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Equal("15s", last().query().Get("step"), "never below 15s")
	})
	s.Run("prom_labels lists names, and the values of one label, within the budget", func() {
		result, err := s.CallTool("prom_labels", map[string]any{"metric": "up"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Equal("/api/v1/labels", strings.SplitN(last().URI, "?", 2)[0])
		s.Equal("up", last().query().Get("match[]"))
		s.Contains(s.text(result), "- pod\n")
		result, err = s.CallTool("prom_labels", map[string]any{"label": "pod", "limit": 2})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Equal("/api/v1/label/pod/values", strings.SplitN(last().URI, "?", 2)[0])
		s.Contains(s.text(result), "label: pod\n")
		s.Contains(s.text(result), "# showing 2 of 3 items; narrow with metric", "the budget finds the one list in the result")
	})
	s.Run("prom_query with a range summarizes each series", func() {
		result, err := s.CallTool("prom_query", map[string]any{"query": "up", "step": "1m", "duration": "5m"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		req := last()
		s.Equal("/api/v1/query_range", strings.SplitN(req.URI, "?", 2)[0])
		s.Equal("1m", req.query().Get("step"))
		s.Equal("up", req.query().Get("query"))
		s.Regexp(`^\d{10}$`, req.query().Get("start"))
		text := s.text(result)
		for _, expected := range []string{"resultType: matrix", "count: 3", "min: 1", "max: 4", "avg: 2.5", "delta: 3", "hasNaN: true", "hasInf: false", "nonFiniteCount: 1", "pod: a"} {
			s.Contains(text, expected)
		}
		s.NotContains(text, "values:", "samples are summarized unless rangeQueryFullResponse is set")
	})
	s.Run("prom_metrics builds the name matcher", func() {
		result, err := s.CallTool("prom_metrics", map[string]any{"match": "up.*"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "- up\n")
		req := last()
		s.Equal(`{__name__=~"up.*"}`, req.query().Get("match[]"))
		result, err = s.CallTool("prom_metrics", map[string]any{"match": `a"}`})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), "contains disallowed characters")
	})
	s.Run("prom_alerts repeats the filter parameter and projects the alerts", func() {
		result, err := s.CallTool("prom_alerts", map[string]any{"filter": "alertname=Down, severity=critical", "active": true})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		req := last()
		s.Equal([]string{"alertname=Down", "severity=critical"}, req.query()["filter"])
		s.Equal("true", req.query().Get("active"))
		s.Empty(req.query().Get("silenced"))
		s.Contains(s.text(result), "state: active")
		s.Contains(s.text(result), "alertname: Down")
		s.NotContains(s.text(result), "fingerprint")
	})
	s.Run("prom_chart carries its UI meta", func() {
		tool := s.findTool("prom_chart")
		s.Require().NotNil(tool)
		s.Equal(map[string]any{"id": "mcp-obs/show-timeseries"}, tool.Meta["olsUi"])
	})
}
