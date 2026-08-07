package kubernetes

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
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

func TestHasNodeMetrics(t *testing.T) {
	t.Run("queries for NodeMetrics GVK", func(t *testing.T) {
		p := &fakeFilteringProvider{hasGVKs: true}
		filter := HasNodeMetrics(p)
		filter()

		if len(p.queriedGVKs) != 1 {
			t.Fatalf("expected 1 GVK query, got %d", len(p.queriedGVKs))
		}
		if p.queriedGVKs[0] != NodeMetricsGVK {
			t.Errorf("expected query for %v, got %v", NodeMetricsGVK, p.queriedGVKs[0])
		}
	})

	t.Run("returns true when provider has NodeMetrics GVK", func(t *testing.T) {
		filter := HasNodeMetrics(&fakeFilteringProvider{hasGVKs: true})
		if !filter() {
			t.Error("expected HasNodeMetrics to return true")
		}
	})

	t.Run("returns false when provider does not have NodeMetrics GVK", func(t *testing.T) {
		filter := HasNodeMetrics(&fakeFilteringProvider{hasGVKs: false})
		if filter() {
			t.Error("expected HasNodeMetrics to return false")
		}
	})
}

func TestHasPodMetrics(t *testing.T) {
	t.Run("queries for PodMetrics GVK", func(t *testing.T) {
		p := &fakeFilteringProvider{hasGVKs: true}
		filter := HasPodMetrics(p)
		filter()

		if len(p.queriedGVKs) != 1 {
			t.Fatalf("expected 1 GVK query, got %d", len(p.queriedGVKs))
		}
		if p.queriedGVKs[0] != PodMetricsGVK {
			t.Errorf("expected query for %v, got %v", PodMetricsGVK, p.queriedGVKs[0])
		}
	})

	t.Run("returns true when provider has PodMetrics GVK", func(t *testing.T) {
		filter := HasPodMetrics(&fakeFilteringProvider{hasGVKs: true})
		if !filter() {
			t.Error("expected HasPodMetrics to return true")
		}
	})

	t.Run("returns false when provider does not have PodMetrics GVK", func(t *testing.T) {
		filter := HasPodMetrics(&fakeFilteringProvider{hasGVKs: false})
		if filter() {
			t.Error("expected HasPodMetrics to return false")
		}
	})
}
