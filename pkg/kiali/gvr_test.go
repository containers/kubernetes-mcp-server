package kiali

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type fakeFilteringProvider struct {
	hasGVKs     bool
	queriedGVKs []schema.GroupVersionKind
}

func (f *fakeFilteringProvider) AnyTargetHasGVKs(_ context.Context, gvks []schema.GroupVersionKind) bool {
	f.queriedGVKs = gvks
	return f.hasGVKs
}

func (f *fakeFilteringProvider) IsTargetCompatibilityToolFiltersEnabled() bool { return true }

// fakeFilteringProviderWithConfig implements both FilteringProvider and ExtendedConfigProvider.
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

func TestInternalServiceURLs(t *testing.T) {
	t.Run("uses status deployment fields and tries web_root defaults", func(t *testing.T) {
		cr := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": "kiali", "namespace": "istio-system"},
			"status": map[string]any{
				"deployment": map[string]any{
					"instanceName": "kiali",
					"namespace":    "istio-system",
				},
			},
		}}
		urls := internalServiceURLs(cr)
		if len(urls) != 2 {
			t.Fatalf("expected 2 candidates, got %v", urls)
		}
		if urls[0] != "http://kiali.istio-system.svc:20001" {
			t.Errorf("unexpected first URL: %s", urls[0])
		}
		if urls[1] != "http://kiali.istio-system.svc:20001/kiali" {
			t.Errorf("unexpected second URL: %s", urls[1])
		}
	})

	t.Run("honors explicit web_root and port", func(t *testing.T) {
		cr := &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": "my-kiali", "namespace": "mesh"},
			"spec": map[string]any{
				"deployment": map[string]any{"instance_name": "my-kiali", "namespace": "mesh"},
				"server":     map[string]any{"port": int64(20001), "web_root": "/kiali"},
			},
		}}
		urls := internalServiceURLs(cr)
		if len(urls) != 1 || urls[0] != "http://my-kiali.mesh.svc:20001/kiali" {
			t.Fatalf("unexpected URLs: %v", urls)
		}
	})
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
			fakeFilteringProvider: fakeFilteringProvider{hasGVKs: false},
			configs: map[string]api.ExtendedConfig{
				"kiali": &Config{Url: srv.URL},
			},
		}
		if !HasKiali(p)() {
			t.Fatal("expected HasKiali true for reachable configured URL")
		}
	})

	t.Run("disables when configured URL fails status probe", func(t *testing.T) {
		p := &fakeFilteringProviderWithConfig{
			fakeFilteringProvider: fakeFilteringProvider{hasGVKs: true},
			configs: map[string]api.ExtendedConfig{
				"kiali": &Config{Url: "http://127.0.0.1:1"},
			},
		}
		if HasKiali(p)() {
			t.Fatal("expected HasKiali false for unreachable configured URL")
		}
	})
}

func TestHasKiali_DiscoverFromCR(t *testing.T) {
	t.Run("returns false when no URL and provider cannot access cluster", func(t *testing.T) {
		p := &fakeFilteringProviderWithConfig{
			fakeFilteringProvider: fakeFilteringProvider{hasGVKs: true},
			configs: map[string]api.ExtendedConfig{
				"kiali": &Config{},
			},
		}
		if HasKiali(p)() {
			t.Fatal("expected false without cluster access for discovery")
		}
	})

	t.Run("discoverAndValidate finds CR but fails when service unreachable", func(t *testing.T) {
		cr := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "kiali.io/v1alpha1",
			"kind":       "Kiali",
			"metadata":   map[string]any{"name": "kiali", "namespace": "istio-system"},
		}}
		scheme := runtime.NewScheme()
		dc := fake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{KialiGVR: "KialiList"},
			cr,
		)
		dc.PrependReactor("list", "kialis", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*cr}}, nil
		})
		url, ok := discoverAndValidateInternalURL(context.Background(), dc, &Config{}, "")
		if ok || url != "" {
			t.Fatalf("expected discovery to fail for unreachable in-cluster URL, got %q", url)
		}
	})
}

func TestHasKiali_NoURLNoCR(t *testing.T) {
	filter := HasKiali(&fakeFilteringProvider{hasGVKs: false})
	if filter() {
		t.Fatal("expected false when no URL and no cluster access")
	}
}
