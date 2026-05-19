package netobserv

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func TestExecuteGetAlertRules_pluginFallbackToPrometheus(t *testing.T) {
	t.Parallel()

	var pluginCalled, promCalled bool
	plugin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pluginCalled = true
		if r.URL.Path != pluginPrometheusRulesPath {
			t.Fatalf("unexpected plugin path: %s", r.URL.Path)
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(plugin.Close)

	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promCalled = true
		if r.URL.Path != prometheusRulesPath {
			t.Fatalf("unexpected prometheus path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("type"); got != "alert" {
			t.Fatalf("expected type=alert, got %q", got)
		}
		if got := r.URL.Query().Get("match[]"); got != "{alertname=NetObserv_*}" {
			t.Fatalf("unexpected match[]: %q", got)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"groups":[]}}`))
	}))
	t.Cleanup(prom.Close)

	tomlCfg := fmt.Sprintf("[toolset_configs.netobserv]\nurl = %q\nprometheus_url = %q\ninsecure = true\n", plugin.URL, prom.URL)
	cfg := test.Must(config.ReadToml([]byte(tomlCfg)))
	client := NewNetObserv(cfg, nil)
	client.bearerToken = "test-token"

	content, err := client.ExecuteGetAlertRules(t.Context(), map[string]any{
		"type":  "alert",
		"match": "alertname=NetObserv_*",
	})
	if err != nil {
		t.Fatalf("ExecuteGetAlertRules: %v", err)
	}
	if !pluginCalled {
		t.Fatal("expected plugin to be called first")
	}
	if !promCalled {
		t.Fatal("expected prometheus fallback to be called")
	}
	if content != `{"status":"success","data":{"groups":[]}}` {
		t.Fatalf("unexpected content: %s", content)
	}
}

func TestExecuteGetAlertRules_pluginSuccessSkipsFallback(t *testing.T) {
	t.Parallel()

	var promCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pluginPrometheusRulesPath {
			_, _ = w.Write([]byte(`{"status":"success","data":{"groups":[{"name":"g1"}]}}`))
			return
		}
		if r.URL.Path == prometheusRulesPath {
			promCalled = true
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	tomlCfg := fmt.Sprintf("[toolset_configs.netobserv]\nurl = %q\nprometheus_url = %q\ninsecure = true\n", server.URL, server.URL)
	cfg := test.Must(config.ReadToml([]byte(tomlCfg)))
	client := NewNetObserv(cfg, nil)

	content, err := client.ExecuteGetAlertRules(t.Context(), map[string]any{"type": "alert"})
	if err != nil {
		t.Fatalf("ExecuteGetAlertRules: %v", err)
	}
	if promCalled {
		t.Fatal("expected no prometheus fallback when plugin succeeds")
	}
	if content == "" {
		t.Fatal("expected content from plugin")
	}
}

func TestExecuteGetAlertSilences_pluginFallback(t *testing.T) {
	t.Parallel()

	var pluginCalled, amCalled bool
	plugin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pluginCalled = true
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(plugin.Close)

	am := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		amCalled = true
		if r.URL.Path != alertmanagerSilencesPath {
			t.Fatalf("unexpected alertmanager path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("filter"); got != "alertname=Foo" {
			t.Fatalf("unexpected filter: %q", got)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(am.Close)

	tomlCfg := fmt.Sprintf("[toolset_configs.netobserv]\nurl = %q\nalertmanager_url = %q\ninsecure = true\n", plugin.URL, am.URL)
	cfg := test.Must(config.ReadToml([]byte(tomlCfg)))
	client := NewNetObserv(cfg, nil)

	_, err := client.ExecuteGetAlertSilences(t.Context(), map[string]any{"filter": "alertname=Foo"})
	if err != nil {
		t.Fatalf("ExecuteGetAlertSilences: %v", err)
	}
	if !pluginCalled || !amCalled {
		t.Fatalf("pluginCalled=%v amCalled=%v", pluginCalled, amCalled)
	}
}
