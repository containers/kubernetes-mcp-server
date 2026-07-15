package kubevirt

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

func TestHasVirtualMachine(t *testing.T) {
	t.Run("queries for VirtualMachine GVK", func(t *testing.T) {
		p := &fakeFilteringProvider{hasGVKs: true}
		filter := HasVirtualMachine(p)
		filter()

		if len(p.queriedGVKs) != 1 {
			t.Fatalf("expected 1 GVK query, got %d", len(p.queriedGVKs))
		}
		if p.queriedGVKs[0] != VirtualMachineGVK {
			t.Errorf("expected query for %v, got %v", VirtualMachineGVK, p.queriedGVKs[0])
		}
	})

	t.Run("returns true when provider has VirtualMachine GVK", func(t *testing.T) {
		filter := HasVirtualMachine(&fakeFilteringProvider{hasGVKs: true})
		if !filter() {
			t.Error("expected HasVirtualMachine to return true")
		}
	})

	t.Run("returns false when provider does not have VirtualMachine GVK", func(t *testing.T) {
		filter := HasVirtualMachine(&fakeFilteringProvider{hasGVKs: false})
		if filter() {
			t.Error("expected HasVirtualMachine to return false")
		}
	})
}
