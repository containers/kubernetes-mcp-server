package korrel8r_test

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	adapter "github.com/containers/kubernetes-mcp-server/pkg/toolsets/korrel8r"
	"github.com/google/jsonschema-go/jsonschema"
	korrel8rapi "github.com/korrel8r/korrel8r/pkg/api"
	upstream "github.com/korrel8r/korrel8r/pkg/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"
)

type Suite struct{ suite.Suite }

func TestKorrel8rIntegration(t *testing.T) { suite.Run(t, new(Suite)) }

type request map[string]any

func (r request) GetArguments() map[string]any { return r }

type kube struct{ api.KubernetesClient }

func (*kube) RESTConfig() *rest.Config { return &rest.Config{} }

func (s *Suite) params(url, token string, args map[string]any) api.ToolHandlerParams {
	cfg, err := config.ReadToml(s.T().Context(), []byte(fmt.Sprintf("[toolset_configs.\"observability/korrel8r\"]\nurl = %q\n", url)), config.WithBaseDefault())
	s.Require().NoError(err)
	ctx := context.WithValue(s.T().Context(), kubernetes.OAuthAuthorizationHeader, "Bearer "+token)
	return api.ToolHandlerParams{Context: ctx, Config: cfg, KubernetesClient: &kube{}, Request: request(args)}
}

func (s *Suite) tool(name string) api.ServerTool {
	for _, t := range (&adapter.Toolset{}).GetTools(s.T().Context(), api.ToolsetContext{}) {
		if t.Tool.Name == "korrel8r_"+name {
			return t
		}
	}
	s.T().Fatalf("tool %s not found", name)
	return api.ServerTool{}
}

func (s *Suite) TestRegistry() {
	downstream := (&adapter.Toolset{}).GetTools(s.T().Context(), api.ToolsetContext{})
	s.Require().Len(downstream, len(upstream.ToolNames))
	for i, name := range upstream.ToolNames {
		s.Run(name, func() {
			binding, ok := upstream.BindTool(nil, name)
			s.Require().True(ok)
			s.Equal("korrel8r_"+name, downstream[i].Tool.Name)
			description := binding.Tool.Description
			for _, toolName := range upstream.ToolNames {
				description = strings.ReplaceAll(description, "'"+toolName+"'", "'korrel8r_"+toolName+"'")
			}
			s.Equal(description, downstream[i].Tool.Description)
			a, err := json.Marshal(binding.Tool.InputSchema)
			s.Require().NoError(err)
			b, err := json.Marshal(downstream[i].Tool.InputSchema)
			s.Require().NoError(err)
			expected := string(a)
			if name == upstream.GetObjects {
				expected = strings.ReplaceAll(expected, "Use 'help'", "Use 'korrel8r_help'")
			}
			s.JSONEq(expected, string(b))
			s.False(downstream[i].IsClusterAware())
			if name == upstream.GetObjects {
				s.Contains(downstream[i].Tool.InputSchema.Properties["query"].Description, "'korrel8r_help'")
				s.Contains(binding.Tool.InputSchema.(*jsonschema.Schema).Properties["query"].Description, "'help'")
			}
		})
	}
	s.Equal(true, *s.tool(upstream.ShowInConsole).Tool.Annotations.DestructiveHint)
}

// Every quoted tool reference must be prefixed, at any schema depth. The
// adapter only rewrites top-level property descriptions, which is all upstream
// needs today; this fails if a tool reference is ever nested deeper.
func (s *Suite) TestNoUnprefixedToolReferencesAtAnyDepth() {
	names := upstream.ToolNames
	for _, tool := range (&adapter.Toolset{}).GetTools(s.T().Context(), api.ToolsetContext{}) {
		s.Run(tool.Tool.Name, func() {
			schema, err := json.Marshal(tool.Tool.InputSchema)
			s.Require().NoError(err)
			document := string(schema) + tool.Tool.Description
			s.NotContains(document, "korrel8r_korrel8r_")
			// Drop the correctly prefixed references; any quoted bare name left
			// behind is a description the rewrite missed.
			remaining := document
			for _, name := range names {
				remaining = strings.ReplaceAll(remaining, "'korrel8r_"+name+"'", "")
			}
			for _, name := range names {
				s.NotContains(remaining, "'"+name+"'", "unprefixed reference to %q", name)
			}
		})
	}
}

// Each binding owns its own schema: building tools must not affect later bindings.
func (s *Suite) TestUpstreamSchemaNotMutated() {
	(&adapter.Toolset{}).GetTools(s.T().Context(), api.ToolsetContext{})
	binding, ok := upstream.BindTool(nil, upstream.GetObjects)
	s.Require().True(ok)
	s.Contains(binding.Tool.InputSchema.(*jsonschema.Schema).Properties["query"].Description, "'help'")
	s.NotContains(binding.Tool.InputSchema.(*jsonschema.Schema).Properties["query"].Description, "korrel8r_")
}

func (s *Suite) TestConsoleSessionsAndErrors() {
	views := map[string]string{"alice": "a", "bob": "b"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "bad path", 404)
			return
		}
		if user == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if user == "missing" {
			http.Error(w, "no console connected", 404)
			return
		}
		if r.Method == http.MethodPut {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			views[user] = fmt.Sprint(body["view"])
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"view":%q}`, views[user])
	}))
	defer server.Close()
	get := s.tool(upstream.GetConsole)
	show := s.tool(upstream.ShowInConsole)
	noIdentity := s.params(server.URL, "alice", nil)
	noIdentity.Context = s.T().Context()
	// Leave authentication decisions to the backend, including for console tools.
	rejected, err := get.Handler(noIdentity)
	s.Require().NoError(err)
	s.ErrorContains(rejected.Error, "401")
	result, err := get.Handler(s.params(server.URL, "alice", nil))
	s.Require().NoError(err)
	s.Contains(result.ContentBlocks[0].Text(), `"a"`)
	result, err = show.Handler(s.params(server.URL, "alice", map[string]any{"view": "updated"}))
	s.Require().NoError(err)
	s.NoError(result.Error)
	result, err = get.Handler(s.params(server.URL, "bob", nil))
	s.Require().NoError(err)
	s.Contains(result.ContentBlocks[0].Text(), `"b"`)
	result, err = get.Handler(s.params(server.URL, "missing", nil))
	s.Require().NoError(err)
	s.ErrorContains(result.Error, "no console connected")
}

// An enabled but unconfigured toolset reports a recoverable tool error that
// names the option to set, except for the static instructions tool.
func (s *Suite) TestUnconfiguredToolset() {
	cfg := config.BaseDefault()
	s.NotContains(cfg.Toolsets.Get(), adapter.ToolsetName, "Korrel8r must be opt-in")
	params := api.ToolHandlerParams{Context: s.T().Context(), Config: cfg, KubernetesClient: &kube{}, Request: request(nil)}
	result, err := s.tool(upstream.ListDomains).Handler(params)
	s.Require().NoError(err)
	s.ErrorContains(result.Error, `set [toolset_configs."observability/korrel8r"].url`)
}

func (s *Suite) TestKubeconfigAuthMode() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer cluster-token" {
			http.Error(w, "wrong token", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"view":"cluster"}`))
	}))
	defer server.Close()
	params := s.params(server.URL, "caller-token", nil)
	cfg, err := config.ReadToml(s.T().Context(), []byte(fmt.Sprintf("[toolset_configs.\"observability/korrel8r\"]\nurl = %q\nauth_mode = \"kubeconfig\"\n", server.URL)), config.WithBaseDefault())
	s.Require().NoError(err)
	params.Config = cfg
	params.KubernetesClient = &tokenKube{token: "cluster-token"}
	result, err := s.tool(upstream.GetConsole).Handler(params)
	s.Require().NoError(err)
	s.NoError(result.Error)
	s.Contains(result.ContentBlocks[0].Text(), "cluster")
}

type tokenKube struct {
	api.KubernetesClient
	token string
}

func (k *tokenKube) RESTConfig() *rest.Config { return &rest.Config{BearerToken: k.token} }

func (s *Suite) TestInvalidAuthMode() {
	_, err := config.ReadToml(s.T().Context(), []byte("[toolset_configs.\"observability/korrel8r\"]\nurl = \"https://example.com\"\nauth_mode = \"other\"\n"), config.WithBaseDefault())
	s.ErrorContains(err, "invalid auth mode")
}

func (s *Suite) TestAllHandlersAgainstREST() {
	responses := map[string]string{
		"/domains":            `[{"name":"k8s"}]`,
		"/domain/k8s/classes": `["Pod"]`,
		"/help/k8s":           `{"documentation":"query syntax"}`,
		"/graphs/neighbors":   `{"nodes":[]}`,
		"/graphs/goals":       `{"nodes":[]}`,
		"/objects":            `[{"name":"pod"}]`,
		"/console":            `{"view":"k8s:Pod:pod"}`,
		"/console/events":     ``,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, korrel8rapi.BasePath)
		body, ok := responses[path]
		if !ok {
			http.Error(w, "unexpected path", 404)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	cases := []struct {
		name       string
		args       map[string]any
		structured bool
	}{
		{upstream.ListDomains, nil, true},
		{upstream.ListDomainClasses, map[string]any{"domain": "k8s"}, true},
		{upstream.Help, map[string]any{"domain": "k8s"}, true},
		{upstream.CreateNeighborsGraph, map[string]any{"start": map[string]any{"queries": []string{"k8s:Pod:pod"}}, "depth": 1}, true},
		{upstream.CreateGoalsGraph, map[string]any{"start": map[string]any{"queries": []string{"k8s:Pod:pod"}}, "goals": []string{"log:application"}}, true},
		{upstream.GetObjects, map[string]any{"query": "k8s:Pod:pod"}, true},
		{upstream.GetConsole, nil, true},
		{upstream.ShowInConsole, map[string]any{"view": "k8s:Pod:pod"}, false},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			result, err := s.tool(tc.name).Handler(s.params(server.URL, "alice", tc.args))
			s.Require().NoError(err)
			s.Require().NoError(result.Error)
			if tc.structured {
				s.NotNil(result.StructuredContent)
				s.NotEmpty(result.ContentBlocks)
			} else {
				s.Nil(result.StructuredContent)
			}
		})
	}
}

// Help documentation can name other tools and must be prefixed.
// Object data from other tools is never rewritten.
func (s *Suite) TestHelpToolReferencesArePrefixed() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, korrel8rapi.BasePath) {
		case "/help/k8s":
			_, _ = w.Write([]byte(`{"documentation":"Use 'get_objects' after 'help'."}`))
		default:
			_, _ = w.Write([]byte(`[{"name":"use 'get_objects' literally"}]`))
		}
	}))
	defer server.Close()
	result, err := s.tool(upstream.Help).Handler(s.params(server.URL, "alice", map[string]any{"domain": "k8s"}))
	s.Require().NoError(err)
	s.Require().NoError(result.Error)
	documentation := result.StructuredContent.(*upstream.HelpResult).Documentation
	s.Equal("Use 'korrel8r_get_objects' after 'korrel8r_help'.", documentation)
	var text upstream.HelpResult
	s.Require().NoError(json.Unmarshal([]byte(result.ContentBlocks[0].Text()), &text))
	s.Equal(documentation, text.Documentation)
	// Data from other tools is passed through verbatim.
	result, err = s.tool(upstream.GetObjects).Handler(s.params(server.URL, "alice", map[string]any{"query": "k8s:Pod:pod"}))
	s.Require().NoError(err)
	s.Require().NoError(result.Error)
	s.Contains(result.ContentBlocks[0].Text(), "use 'get_objects' literally")
}

func (s *Suite) TestInvalidArguments() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.T().Error("invalid arguments must not reach the backend")
	}))
	defer server.Close()
	for _, args := range []map[string]any{{}, {"query": 42}} {
		result, err := s.tool(upstream.GetObjects).Handler(s.params(server.URL, "alice", args))
		s.Require().NoError(err)
		s.Error(result.Error)
	}
}

func (s *Suite) TestToolsDoNotProbeDomains() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != korrel8rapi.BasePath+"/help/k8s" {
			s.T().Errorf("unexpected request to %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"documentation":"query syntax"}`))
	}))
	defer server.Close()
	result, err := s.tool(upstream.Help).Handler(s.params(server.URL, "alice", map[string]any{"domain": "k8s"}))
	s.Require().NoError(err)
	s.Require().NoError(result.Error)
	s.Contains(result.ContentBlocks[0].Text(), "query syntax")
}

func (s *Suite) TestTrailingSlashEndpoint() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != korrel8rapi.BasePath+"/domains" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`[{"name":"k8s"}]`))
	}))
	defer server.Close()

	for _, suffix := range []string{"/", "///"} {
		s.Run(suffix, func() {
			result, err := s.tool(upstream.ListDomains).Handler(s.params(server.URL+suffix, "alice", nil))
			s.Require().NoError(err)
			s.Require().NoError(result.Error)
			s.Contains(result.ContentBlocks[0].Text(), `"k8s"`)
		})
	}
}

// The upstream client has its own 8 MiB default; the configured server limit
// must take precedence even for responses larger than that default.
func (s *Suite) TestHigherBackendResponseLimit() {
	body := `{"documentation":"` + strings.Repeat("x", 9<<20) + `"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix"+korrel8rapi.BasePath+"/help/k8s" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	for _, tc := range []struct {
		name, url string
		limit     int64
		tooLarge  bool
	}{
		{"higher cap", server.URL + "/prefix", 12 << 20, false},
		{"trailing slash", server.URL + "/prefix/", 12 << 20, false},
		{"disabled cap", server.URL + "/prefix", 0, false},
		{"lower cap", server.URL + "/prefix", 4 << 20, true},
	} {
		s.Run(tc.name, func() {
			params := s.params(tc.url, "alice", map[string]any{"domain": "k8s"})
			params.Config.MaxBackendResponseBytes.SetForTest(tc.limit)
			result, err := s.tool(upstream.Help).Handler(params)
			s.Require().NoError(err)
			if tc.tooLarge {
				s.Error(result.Error)
				return
			}
			s.Require().NoError(result.Error)
			s.Contains(result.ContentBlocks[0].Text(), strings.Repeat("x", 9<<20))
		})
	}
}

func (s *Suite) TestBackendLimitAndTLS() {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"` + strings.Repeat("x", 200) + `"}]`))
	}))
	defer server.Close()
	params := s.params(server.URL, "alice", nil)
	params.Config.MaxBackendResponseBytes.SetForTest(32)
	params.KubernetesClient = &trustedKube{ca: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})}
	result, err := s.tool(upstream.ListDomains).Handler(params)
	s.Require().NoError(err)
	s.ErrorContains(result.Error, "32")
	params.Config.MaxBackendResponseBytes.SetForTest(1024)
	result, err = s.tool(upstream.ListDomains).Handler(params)
	s.Require().NoError(err)
	s.NoError(result.Error)
}

type trustedKube struct {
	api.KubernetesClient
	ca []byte
}

func (k *trustedKube) RESTConfig() *rest.Config {
	return &rest.Config{TLSClientConfig: rest.TLSClientConfig{CAData: k.ca}}
}

func (s *Suite) TestRequireTLSBlocksHTTP() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.T().Error("HTTP backend should not be contacted when require_tls is enabled")
	}))
	defer server.Close()
	params := s.params(server.URL, "alice", nil)
	params.Config.RequireTLS.SetForTest(true)
	result, err := s.tool(upstream.ListDomains).Handler(params)
	s.Require().NoError(err)
	s.ErrorContains(result.Error, "require_tls")
}

// The scheme decides whether the transport is TLS, so it must be read from the
// parsed URL rather than sniffed from the raw string.
func (s *Suite) TestRequireTLSAcceptsUppercaseScheme() {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"k8s"}]`))
	}))
	defer server.Close()
	params := s.params(strings.Replace(server.URL, "https://", "HTTPS://", 1), "alice", nil)
	params.Config.RequireTLS.SetForTest(true)
	params.KubernetesClient = &trustedKube{ca: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})}
	result, err := s.tool(upstream.ListDomains).Handler(params)
	s.Require().NoError(err)
	s.Require().NoError(result.Error)
	s.Contains(result.ContentBlocks[0].Text(), `"k8s"`)
}

// A slow backend must be bounded by the configured timeout, not the 30s default.
func (s *Suite) TestConfiguredTimeout() {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer func() { close(release); server.Close() }()
	cfg, err := config.ReadToml(s.T().Context(), fmt.Appendf(nil,
		"[toolset_configs.%q]\nurl = %q\ntimeout = \"50ms\"\n", "observability/korrel8r", server.URL), config.WithBaseDefault())
	s.Require().NoError(err)
	params := s.params(server.URL, "alice", nil)
	params.Config = cfg
	result, err := s.tool(upstream.ListDomains).Handler(params)
	s.Require().NoError(err)
	s.Require().Error(result.Error)
	s.True(strings.Contains(result.Error.Error(), "deadline exceeded") || strings.Contains(result.Error.Error(), "Client.Timeout exceeded"), "unexpected timeout error: %v", result.Error)
}

func (s *Suite) TestValidationAndCancellation() {
	for _, url := range []string{
		"", "https://", "file:///tmp/x", "ftp://example.com",
		"https://user@example.com", "https://example.com/path?query=1",
		"https://example.com/path?", "https://example.com/path#fragment", "https://example.com/path#",
	} {
		s.Run(url, func() { s.Error((&adapter.Config{URL: url}).Validate()) })
	}
	s.Error((&adapter.Config{URL: "https://example.com", Timeout: -time.Second}).Validate())
	s.NoError((&adapter.Config{URL: "HTTPS://example.com", Timeout: time.Minute}).Validate())
	// Both HTTP and HTTPS endpoints are allowed unless require_tls is enabled.
	for _, url := range []string{
		"https://example.com",
		"https://korrel8r.korrel8r.svc:8443",
		"http://example.com",
		"http://svc.example.com",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://korrel8r.korrel8r.svc:8080",
		"http://korrel8r.korrel8r.svc.cluster.local:8080",
	} {
		s.Run(url, func() {
			cfg := &adapter.Config{URL: url}
			s.NoError(cfg.Validate())
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[]`)) }))
	defer server.Close()
	params := s.params(server.URL, "alice", nil)
	ctx, cancel := context.WithCancel(params.Context)
	cancel()
	params.Context = ctx
	result, err := s.tool(upstream.ListDomains).Handler(params)
	s.Require().NoError(err)
	s.ErrorContains(result.Error, "context canceled")
}

// Callers with the same configuration share one transport, so connections are
// reused, while each request still carries its own caller's token.
func (s *Suite) TestSharedTransportKeepsCallerIdentity() {
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		_, _ = fmt.Fprintf(w, `{"view":%q}`, user)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	tool := s.tool(upstream.GetConsole)
	shared := s.params(server.URL, "", nil)
	for _, user := range []string{"alice", "bob", "alice", "bob"} {
		params := shared
		params.Context = context.WithValue(s.T().Context(), kubernetes.OAuthAuthorizationHeader, "Bearer "+user)
		result, err := tool.Handler(params)
		s.Require().NoError(err)
		s.Require().NoError(result.Error)
		s.Contains(result.ContentBlocks[0].Text(), `"`+user+`"`)
	}
	s.Equal(int32(1), connections.Load())
}

// Upstream REST errors already name korrel8r; the adapter must not repeat it.
func (s *Suite) TestErrorPrefixNotRepeated() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no console connected", http.StatusNotFound)
	}))
	defer server.Close()
	result, err := s.tool(upstream.GetConsole).Handler(s.params(server.URL, "alice", nil))
	s.Require().NoError(err)
	s.Require().Error(result.Error)
	s.True(strings.HasPrefix(result.Error.Error(), "korrel8r: GET "), result.Error.Error())
	s.NotContains(result.Error.Error(), "korrel8r: korrel8r:")
	// Errors that do not come from the REST client get the prefix.
	result, err = s.tool(upstream.GetObjects).Handler(s.params(server.URL, "alice", map[string]any{}))
	s.Require().NoError(err)
	s.Require().Error(result.Error)
	s.True(strings.HasPrefix(result.Error.Error(), "korrel8r: "), result.Error.Error())
}
