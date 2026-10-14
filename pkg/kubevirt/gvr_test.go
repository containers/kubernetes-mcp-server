package kubevirt

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeInspector struct {
	available           bool
	queriedGroupVersion string
}

func (f *fakeInspector) Discovery() api.AggregateDiscovery       { return f }
func (f *fakeInspector) Unstructured() api.AggregateUnstructured { return nil }
func (f *fakeInspector) ServerResourcesForGroupVersion(ctx context.Context, groupVersion string) api.Results[*metav1.APIResourceList] {
	f.queriedGroupVersion = groupVersion
	return api.NewResults(ctx, f, func(context.Context, string) (*metav1.APIResourceList, error) {
		list := &metav1.APIResourceList{}
		if f.available {
			list.APIResources = []metav1.APIResource{{Kind: VirtualMachineGVK.Kind}, {Kind: VirtualMachineTemplateGVK.Kind}}
		}
		return list, nil
	})
}
func (f *fakeInspector) IsMultiTarget() bool                          { return false }
func (f *fakeInspector) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }
func (f *fakeInspector) GetDefaultTarget() string                     { return "" }
func (f *fakeInspector) GetTargetParameterName() string               { return "" }

func TestHasVirtualMachineTemplate(t *testing.T) {
	t.Run("queries for VirtualMachineTemplate GVK", func(t *testing.T) {
		p := &fakeInspector{available: true}
		filter := HasVirtualMachineTemplate(t.Context(), p)
		filter()

		if p.queriedGroupVersion != VirtualMachineTemplateGVK.GroupVersion().String() {
			t.Errorf("expected query for %v, got %v", VirtualMachineTemplateGVK.GroupVersion(), p.queriedGroupVersion)
		}
	})

	t.Run("returns true when provider has VirtualMachineTemplate GVK", func(t *testing.T) {
		filter := HasVirtualMachineTemplate(t.Context(), &fakeInspector{available: true})
		if !filter() {
			t.Error("expected HasVirtualMachineTemplate to return true")
		}
	})

	t.Run("returns false when provider does not have VirtualMachineTemplate GVK", func(t *testing.T) {
		filter := HasVirtualMachineTemplate(t.Context(), &fakeInspector{available: false})
		if filter() {
			t.Error("expected HasVirtualMachineTemplate to return false")
		}
	})
}

func TestHasVirtualMachine(t *testing.T) {
	t.Run("queries for VirtualMachine GVK", func(t *testing.T) {
		p := &fakeInspector{available: true}
		filter := HasVirtualMachine(t.Context(), p)
		filter()

		if p.queriedGroupVersion != VirtualMachineGVK.GroupVersion().String() {
			t.Errorf("expected query for %v, got %v", VirtualMachineGVK.GroupVersion(), p.queriedGroupVersion)
		}
	})

	t.Run("returns true when provider has VirtualMachine GVK", func(t *testing.T) {
		filter := HasVirtualMachine(t.Context(), &fakeInspector{available: true})
		if !filter() {
			t.Error("expected HasVirtualMachine to return true")
		}
	})

	t.Run("returns false when provider does not have VirtualMachine GVK", func(t *testing.T) {
		filter := HasVirtualMachine(t.Context(), &fakeInspector{available: false})
		if filter() {
			t.Error("expected HasVirtualMachine to return false")
		}
	})
}
