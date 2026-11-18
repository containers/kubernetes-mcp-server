package kiali

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type namespaceFilteringUnstructured struct {
	tp api.TargetProvider
}

func (m *namespaceFilteringUnstructured) Resource(gvr schema.GroupVersionResource) api.AggregateNamespaceableResourceInterface {
	if gvr != KialiGVR {
		return &mockKialiEmptyResource{tp: m.tp}
	}
	return &namespaceFilteringNamespacedResource{tp: m.tp}
}

type namespaceFilteringNamespacedResource struct {
	namespace string
	tp        api.TargetProvider
}

func (m *namespaceFilteringNamespacedResource) Namespace(ns string) api.AggregateResourceInterface {
	return &namespaceFilteringNamespacedResource{namespace: ns, tp: m.tp}
}

func (m *namespaceFilteringNamespacedResource) List(ctx context.Context, _ metav1.ListOptions) api.Results[*unstructured.UnstructuredList] {
	return api.NewResults(ctx, m.tp, func(context.Context, string) (*unstructured.UnstructuredList, error) {
		if m.namespace != "istio-system" && m.namespace != metav1.NamespaceAll {
			return &unstructured.UnstructuredList{}, nil
		}
		cr := unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "kiali.io/v1alpha1",
			"kind":       "Kiali",
			"metadata":   map[string]any{"name": "kiali", "namespace": "istio-system"},
		}}
		return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{cr}}, nil
	})
}

func (m *namespaceFilteringNamespacedResource) Get(ctx context.Context, _ string, _ metav1.GetOptions, _ ...string) api.Results[*unstructured.Unstructured] {
	return api.NewResults(ctx, m.tp, func(context.Context, string) (*unstructured.Unstructured, error) {
		return nil, nil
	})
}

type namespaceFilteringInspector struct {
	mockKialiInspector
}

func (m *namespaceFilteringInspector) Unstructured() api.AggregateUnstructured {
	return &namespaceFilteringUnstructured{tp: m}
}

func TestListKialiCRs_RespectsAllowedNamespaces(t *testing.T) {
	inspector := &namespaceFilteringInspector{mockKialiInspector: mockKialiInspector{available: true}}

	t.Run("finds CR in default istio-system", func(t *testing.T) {
		crs := listKialiCRs(context.Background(), inspector, []string{"istio-system"})
		if len(crs) != 1 {
			t.Fatalf("expected 1 CR in istio-system, got %d", len(crs))
		}
	})

	t.Run("skips namespaces outside allowed list", func(t *testing.T) {
		crs := listKialiCRs(context.Background(), inspector, []string{"other-ns"})
		if len(crs) != 0 {
			t.Fatalf("expected no CRs outside allowed namespaces, got %d", len(crs))
		}
	})

	t.Run("wildcard lists CRs cluster-wide", func(t *testing.T) {
		crs := listKialiCRs(context.Background(), inspector, []string{DiscoveryAllNamespacesWildcard})
		if len(crs) != 1 {
			t.Fatalf("expected 1 CR with cluster-wide list, got %d", len(crs))
		}
	})
}

func TestNamespacesForServiceDNSProbe(t *testing.T) {
	t.Run("uses configured namespaces", func(t *testing.T) {
		got := namespacesForServiceDNSProbe([]string{"mesh", "istio-system"}, nil)
		if len(got) != 2 || got[0] != "mesh" {
			t.Fatalf("expected configured list, got %v", got)
		}
	})

	t.Run("wildcard derives probe namespaces from cluster-wide Kiali CRs", func(t *testing.T) {
		inspector := &namespaceFilteringInspector{mockKialiInspector: mockKialiInspector{available: true}}
		crs := listKialiCRs(context.Background(), inspector, []string{DiscoveryAllNamespacesWildcard})
		got := namespacesForServiceDNSProbe([]string{DiscoveryAllNamespacesWildcard}, crs)
		if len(got) != 1 || got[0] != "istio-system" {
			t.Fatalf("expected [istio-system] from CR, got %v", got)
		}
	})
}

func TestWellKnownInternalURLs(t *testing.T) {
	urls := wellKnownInternalURLs([]string{"istio-system", "mesh"}, false)
	if len(urls) != 8 {
		t.Fatalf("expected 8 URLs, got %v", urls)
	}
	if urls[0] != "https://kiali.istio-system.svc:20001" {
		t.Errorf("unexpected first URL: %s", urls[0])
	}
	if urls[4] != "https://kiali.mesh.svc:20001" {
		t.Errorf("unexpected fifth URL: %s", urls[4])
	}
}
