package kiali

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func kialiTestConfig(url string) *config.Config {
	cfg, err := config.ReadToml(context.Background(), []byte(fmt.Sprintf(`
		[toolset_configs.kiali]
		url = "%s"
	`, url)))
	if err != nil {
		panic(err)
	}
	return cfg
}

type mockKialiInspector struct {
	available bool
}

func (m *mockKialiInspector) Discovery() api.AggregateDiscovery       { return m }
func (m *mockKialiInspector) Unstructured() api.AggregateUnstructured { return nil }
func (m *mockKialiInspector) GetTargets(context.Context) ([]string, error) {
	return []string{""}, nil
}
func (m *mockKialiInspector) GetDefaultTarget() string       { return "" }
func (m *mockKialiInspector) GetTargetParameterName() string { return "" }
func (m *mockKialiInspector) IsMultiTarget() bool            { return false }
func (m *mockKialiInspector) ServerResourcesForGroupVersion(ctx context.Context, groupVersion string) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, m, func(context.Context, string) (*metav1.APIResourceList, error) {
		list := &metav1.APIResourceList{}
		if m.available {
			list.APIResources = []metav1.APIResource{{Kind: KialiGVK.Kind}}
		}
		return list, nil
	})
}

func TestInternalServiceURLs(t *testing.T) {
	t.Run("uses status deployment fields and tries web_root defaults", func(t *testing.T) {
		cr := &KialiCR{
			ObjectMeta: metav1.ObjectMeta{Name: "kiali", Namespace: "istio-system"},
			Status: KialiStatus{
				Deployment: KialiDeploymentStatus{
					InstanceName: "kiali",
					Namespace:    "istio-system",
				},
			},
		}
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
		cr := &KialiCR{
			ObjectMeta: metav1.ObjectMeta{Name: "my-kiali", Namespace: "mesh"},
			Spec: KialiSpec{
				Deployment: KialiDeploymentSpec{
					InstanceName: "my-kiali",
					Namespace:    "mesh",
				},
				Server: KialiServerSpec{
					Port:    20001,
					WebRoot: "/kiali",
				},
			},
		}
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

		if !HasKiali(context.Background(), kialiTestConfig(srv.URL), nil, nil) {
			t.Fatal("expected HasKiali true for reachable configured URL")
		}
	})

	t.Run("disables when configured URL fails status probe", func(t *testing.T) {
		if HasKiali(context.Background(), kialiTestConfig("http://127.0.0.1:1"), nil, nil) {
			t.Fatal("expected HasKiali false for unreachable configured URL")
		}
	})
}

func TestHasKiali_DiscoverFromCR(t *testing.T) {
	t.Run("returns false when no URL and provider cannot access cluster", func(t *testing.T) {
		if HasKiali(context.Background(), config.New(), nil, &mockKialiInspector{available: true}) {
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
	if HasKiali(context.Background(), config.New(), nil, nil) {
		t.Fatal("expected false when no URL and no cluster access")
	}
}

func TestProbeCandidateURLs(t *testing.T) {
	t.Run("returns first reachable URL", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": map[string]any{"Kiali state": "running"},
			})
		}))
		defer srv.Close()

		url, ok := probeCandidateURLs(context.Background(), []string{
			"http://127.0.0.1:1",
			srv.URL,
			"http://127.0.0.1:2",
		}, nil, "")
		if !ok || url != srv.URL {
			t.Fatalf("expected reachable URL %q, got %q ok=%v", srv.URL, url, ok)
		}
	})
}
