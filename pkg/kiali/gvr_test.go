package kiali

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

func TestHasKiali(t *testing.T) {
	t.Run("queries for Kiali GVK", func(t *testing.T) {
		p := &fakeFilteringProvider{hasGVKs: true}
		filter := HasKiali(p)
		filter()

		if len(p.queriedGVKs) != 1 {
			t.Fatalf("expected 1 GVK query, got %d", len(p.queriedGVKs))
		}
		if p.queriedGVKs[0] != KialiGVK {
			t.Errorf("expected query for %v, got %v", KialiGVK, p.queriedGVKs[0])
		}
	})

	t.Run("returns true when provider has Kiali GVK", func(t *testing.T) {
		filter := HasKiali(&fakeFilteringProvider{hasGVKs: true})
		if !filter() {
			t.Error("expected HasKiali to return true")
		}
	})

	t.Run("returns false when provider does not have Kiali GVK", func(t *testing.T) {
		filter := HasKiali(&fakeFilteringProvider{hasGVKs: false})
		if filter() {
			t.Error("expected HasKiali to return false")
		}
	})
}
