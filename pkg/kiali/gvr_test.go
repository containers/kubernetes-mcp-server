package kiali

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type fakeFilteringProvider struct{}

func (f *fakeFilteringProvider) AnyTargetHasGVKs(context.Context, []schema.GroupVersionKind) bool {
	return false
}

func (f *fakeFilteringProvider) IsTargetCompatibilityToolFiltersEnabled() bool { return true }

type fakeFilteringProviderWithConfig struct {
	fakeFilteringProvider
	configs map[string]api.ExtendedConfig
}

func (f *fakeFilteringProviderWithConfig) GetProviderConfig(string) (api.ExtendedConfig, bool) {
	return nil, false
}

func (f *fakeFilteringProviderWithConfig) GetToolsetConfig(name string) (api.ExtendedConfig, bool) {
	cfg, ok := f.configs[name]
	return cfg, ok
}

func TestProbeStatusURL(t *testing.T) {
	t.Run("returns true for valid status payload", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/status" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": map[string]any{"Kiali state": "running"},
			})
		}))
		defer srv.Close()

		if !probeStatusURL(context.Background(), srv.URL, &Config{Url: srv.URL}, "") {
			t.Fatal("expected probe to succeed")
		}
	})

	t.Run("returns false for missing status object", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"externalServices":[]}`))
		}))
		defer srv.Close()
		if probeStatusURL(context.Background(), srv.URL, nil, "") {
			t.Fatal("expected probe to fail")
		}
	})

	t.Run("returns false on connection error", func(t *testing.T) {
		if probeStatusURL(context.Background(), "http://127.0.0.1:1", nil, "") {
			t.Fatal("expected probe to fail for unreachable URL")
		}
	})
}

func TestHasKiali_ConfiguredURL(t *testing.T) {
	t.Run("enables when configured URL passes status probe", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": map[string]any{"Kiali state": "running"},
			})
		}))
		defer srv.Close()

		p := &fakeFilteringProviderWithConfig{
			configs: map[string]api.ExtendedConfig{
				"kiali": &Config{Url: srv.URL},
			},
		}
		if !HasKiali(context.Background(), p, nil) {
			t.Fatal("expected HasKiali true for reachable configured URL")
		}
	})

	t.Run("disables when configured URL fails status probe", func(t *testing.T) {
		p := &fakeFilteringProviderWithConfig{
			configs: map[string]api.ExtendedConfig{
				"kiali": &Config{Url: "http://127.0.0.1:1"},
			},
		}
		if HasKiali(context.Background(), p, nil) {
			t.Fatal("expected HasKiali false for unreachable configured URL")
		}
	})

	t.Run("disables when URL is not configured", func(t *testing.T) {
		p := &fakeFilteringProviderWithConfig{
			configs: map[string]api.ExtendedConfig{
				"kiali": &Config{},
			},
		}
		if HasKiali(context.Background(), p, nil) {
			t.Fatal("expected HasKiali false without configured URL")
		}
	})
}
