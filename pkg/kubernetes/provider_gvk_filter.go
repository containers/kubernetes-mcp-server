package kubernetes

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ProviderGVKFilter provides GVK-based filtering capabilities for providers.
// It can be embedded in provider implementations to add AnyTargetHasGVKs functionality.
type ProviderGVKFilter struct {
	managerProvider ManagerProvider
}

// NewProviderGVKFilter creates a new ProviderGVKFilter that wraps a ManagerProvider.
func NewProviderGVKFilter(mp ManagerProvider) *ProviderGVKFilter {
	return &ProviderGVKFilter{
		managerProvider: mp,
	}
}

// AnyTargetHasGVKs reports whether every GVK in gvks is available on at least one target
// exposed by this provider. Returns true if an error occurs during discovery to avoid
// excluding tools due to transient issues.
func (f *ProviderGVKFilter) AnyTargetHasGVKs(ctx context.Context, gvks []schema.GroupVersionKind) bool {
	if len(gvks) == 0 {
		return true
	}

	logger := klogutil.FromContext(ctx)
	mgrs, err := f.managerProvider.GetTargetManagers(ctx)
	// If an error occurs, don't exclude tools
	if err != nil {
		klogutil.LogWarn(logger, "AnyTargetHasGVKs couldn't retrieve target managers; assuming all GVKs are available", klogutil.Err(err))
		return true
	}

	for _, mgr := range mgrs {
		k, err := mgr.Derived(ctx)
		if err != nil {
			// Can't get discovery client; assume target has the GVKs to avoid
			// hiding tools due to transient errors
			klogutil.LogWarn(logger, "AnyTargetHasGVKs couldn't derive a Kubernetes interface for a manager; assuming all GVKs are available", klogutil.Err(err))
			return true
		}

		hasGVKs, err := api.HasGVKs(k.DiscoveryClient(), gvks)
		if err != nil {
			// Discovery error; assume target has the GVKs to avoid hiding tools
			klogutil.LogWarn(logger, "AnyTargetHasGVKs couldn't query a client; assuming all GVKs are available", klogutil.Err(err))
			return true
		}
		if hasGVKs {
			// One target with all GVKs is enough
			return true
		}
	}
	return false
}
