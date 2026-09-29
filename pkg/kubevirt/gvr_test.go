package kubevirt

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeInspector struct {
	hasGVKs     bool
	queriedGVKs []schema.GroupVersionKind
}

func (f *fakeInspector) Discovery() api.AggregateDiscovery       { return f }
func (f *fakeInspector) Unstructured() api.AggregateUnstructured { return nil }
func (f *fakeInspector) HasGVKs(ctx context.Context, gvks []schema.GroupVersionKind) api.BoolResults {
	f.queriedGVKs = gvks
	return api.BoolResults(api.NewResults(ctx, f, func(context.Context, string) (bool, error) {
		return f.hasGVKs, nil
	}))
}
func (f *fakeInspector) ServerResourcesForGroupVersion(ctx context.Context, _ string) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, f, func(context.Context, string) (*metav1.APIResourceList, error) {
		return &metav1.APIResourceList{}, nil
	})
}
func (f *fakeInspector) IsMultiTarget() bool                          { return false }
func (f *fakeInspector) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }
func (f *fakeInspector) GetDefaultTarget() string                     { return "" }
func (f *fakeInspector) GetTargetParameterName() string               { return "" }

func TestHasVirtualMachineTemplate(t *testing.T) {
	t.Run("queries for VirtualMachineTemplate GVK", func(t *testing.T) {
		p := &fakeInspector{hasGVKs: true}
		filter := HasVirtualMachineTemplate(t.Context(), p)
		filter()

		if len(p.queriedGVKs) != 1 {
			t.Fatalf("expected 1 GVK query, got %d", len(p.queriedGVKs))
		}
		if p.queriedGVKs[0] != VirtualMachineTemplateGVK {
			t.Errorf("expected query for %v, got %v", VirtualMachineTemplateGVK, p.queriedGVKs[0])
		}
	})

	t.Run("returns true when provider has VirtualMachineTemplate GVK", func(t *testing.T) {
		filter := HasVirtualMachineTemplate(t.Context(), &fakeInspector{hasGVKs: true})
		if !filter() {
			t.Error("expected HasVirtualMachineTemplate to return true")
		}
	})

	t.Run("returns false when provider does not have VirtualMachineTemplate GVK", func(t *testing.T) {
		filter := HasVirtualMachineTemplate(t.Context(), &fakeInspector{hasGVKs: false})
		if filter() {
			t.Error("expected HasVirtualMachineTemplate to return false")
		}
	})
}

func TestHasVirtualMachine(t *testing.T) {
	t.Run("queries for VirtualMachine GVK", func(t *testing.T) {
		p := &fakeInspector{hasGVKs: true}
		filter := HasVirtualMachine(t.Context(), p)
		filter()

		if len(p.queriedGVKs) != 1 {
			t.Fatalf("expected 1 GVK query, got %d", len(p.queriedGVKs))
		}
		if p.queriedGVKs[0] != VirtualMachineGVK {
			t.Errorf("expected query for %v, got %v", VirtualMachineGVK, p.queriedGVKs[0])
		}
	})

	t.Run("returns true when provider has VirtualMachine GVK", func(t *testing.T) {
		filter := HasVirtualMachine(t.Context(), &fakeInspector{hasGVKs: true})
		if !filter() {
			t.Error("expected HasVirtualMachine to return true")
		}
	})

	t.Run("returns false when provider does not have VirtualMachine GVK", func(t *testing.T) {
		filter := HasVirtualMachine(t.Context(), &fakeInspector{hasGVKs: false})
		if filter() {
			t.Error("expected HasVirtualMachine to return false")
		}
	})
}
