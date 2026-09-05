package kubernetes

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// ProviderGVKFilter provides GVK-based filtering capabilities for providers.
// It can be embedded in provider implementations to add AnyTargetHasGVKs functionality.
type ProviderGVKFilter struct {
	managerProvider  ManagerProvider
	filteringEnabled func() bool
}

// NewProviderGVKFilter creates a new ProviderGVKFilter that wraps a ManagerProvider.
// The filteringEnabled function controls whether filtering is active; when it returns
// false, AnyTargetHasGVKs unconditionally returns true so that all tools remain visible.
func NewProviderGVKFilter(mp ManagerProvider, filteringEnabled func() bool) *ProviderGVKFilter {
	return &ProviderGVKFilter{
		managerProvider:  mp,
		filteringEnabled: filteringEnabled,
	}
}

// AnyTargetHasGVKs reports whether every GVK in gvks is available on at least one target
// exposed by this provider. Returns true when filtering is disabled or if an error occurs
// during discovery to avoid excluding tools due to transient issues.
func (f *ProviderGVKFilter) AnyTargetHasGVKs(ctx context.Context, gvks []schema.GroupVersionKind) bool {
	if !f.filteringEnabled() {
		return true
	}
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Buffered so workers can finish after we return on the first true.
	results := make(chan bool, len(mgrs))
	for _, mgr := range mgrs {
		go func() {
			results <- targetHasGVKs(ctx, logger, mgr, gvks)
		}()
	}

	for range mgrs {
		if <-results {
			// One target with all GVKs (or a discovery error) is enough.
			// Workers that have not reached HasGVKs yet will stop via the deferred cancel() above.
			// client-go's DiscoveryClient uses context.TODO() (at the time of this writing), so
			// in-flight ServerResourcesForGroupVersion HTTP is not aborted by this cancel.
			return true
		}
	}
	return false
}

func targetHasGVKs(ctx context.Context, logger klog.Logger, mgr *Manager, gvks []schema.GroupVersionKind) bool {
	// Context errors can happen two ways:
	// - Another thread returned `true`, triggering cancelation. It doesn't matter what this thread
	//   returns, as the caller short-circuits `true` regardless.
	// - The parent context was canceled for some other reason before anyone reported `true` but
	//   also before everyone finished. We want to treat that like "all threads errored" and fail
	//   *open* (don't hide tools).
	if ctx.Err() != nil {
		return true
	}

	k, err := mgr.Derived(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return true
		}
		// Can't get discovery client; assume target has the GVKs to avoid
		// hiding tools due to transient errors
		klogutil.LogWarn(logger, "AnyTargetHasGVKs couldn't derive a Kubernetes interface for a manager; assuming all GVKs are available", klogutil.Err(err))
		return true
	}

	hasGVKs, err := api.HasGVKs(k.DiscoveryClient(), gvks)
	if err != nil {
		if ctx.Err() != nil {
			return true
		}
		// Discovery error; assume target has the GVKs to avoid hiding tools
		klogutil.LogWarn(logger, "AnyTargetHasGVKs couldn't query a client; assuming all GVKs are available", klogutil.Err(err))
		return true
	}
	return hasGVKs
}
