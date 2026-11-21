package watcher

import (
	"context"
	"sort"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"

	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

const (
	// DefaultClusterStatePollInterval is the default interval for polling cluster state changes
	DefaultClusterStatePollInterval = 30 * time.Second
	// DefaultClusterStateDebounceWindow is the default debounce window for cluster state changes
	DefaultClusterStateDebounceWindow = 5 * time.Second
)

// clusterState represents the cached state of the cluster
type clusterState struct {
	apiGroups   []string
	isOpenShift bool
}

// ClusterState monitors cluster state changes and triggers debounced reloads
type ClusterState struct {
	discoveryClient discovery.CachedDiscoveryInterface
	pollInterval    time.Duration
	debounceWindow  time.Duration
	lastKnownState  clusterState
	debounceTimer   *time.Timer
	mu              sync.Mutex
	stopCh          chan struct{}
	stoppedCh       chan struct{}
	started         bool
}

var _ Watcher = (*ClusterState)(nil)

func NewClusterState(ctx context.Context, discoveryClient discovery.CachedDiscoveryInterface, pollInterval, debounceWindow time.Duration) *ClusterState {
	if pollInterval <= 0 {
		pollInterval = DefaultClusterStatePollInterval
	}
	if debounceWindow <= 0 {
		debounceWindow = DefaultClusterStateDebounceWindow
	}
	logger := klogutil.FromContext(ctx)
	logger.V(2).Info("Using cluster state watcher timings", "poll_interval", pollInterval, "debounce_window", debounceWindow)

	return &ClusterState{
		discoveryClient: discoveryClient,
		pollInterval:    pollInterval,
		debounceWindow:  debounceWindow,
		stopCh:          make(chan struct{}),
		stoppedCh:       make(chan struct{}),
	}
}

// Watch starts a background watcher that periodically polls for cluster state changes
// and triggers a debounced reload when changes are detected.
// It can only be called once per ClusterState instance.
func (w *ClusterState) Watch(ctx context.Context, onChange func() error) {
	logger := klogutil.FromContext(ctx)

	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	// capture talks to the API server; do not hold mu across it.
	// Capture before Watch returns so callers that mutate cluster state
	// immediately afterwards are compared against the pre-watch snapshot.
	initial, err := w.capture()
	if isAuthError(err) {
		// Decision: stop instead of polling with credentials the API server refuses.
		// This happens when the server has no identity of its own, e.g. passthrough
		// auth with a kubeconfig without credentials. The provider builds a new
		// watcher when the kubeconfig changes, which checks again.
		logger.Info("Cluster state watcher disabled: the API server refused discovery", "error", err)
		return
	}

	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.lastKnownState = initial
	go func() {
		defer close(w.stoppedCh)
		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()

		logger.V(2).Info("Started cluster state watcher",
			"poll_interval", w.pollInterval,
			"debounce_window", w.debounceWindow,
		)

		for {
			select {
			case <-w.stopCh:
				logger.V(2).Info("Stopping cluster state watcher")
				return
			case <-ticker.C:
				// Invalidate discovery cache to get fresh API groups
				w.discoveryClient.Invalidate()

				current, err := w.capture()
				if isAuthError(err) {
					logger.Info("Stopping cluster state watcher: the API server refused discovery", "error", err)
					return
				}
				w.mu.Lock()
				logger.V(3).Info("Polled cluster state",
					"cluster.api_groups.count", len(current.apiGroups),
					"cluster.is_openshift", current.isOpenShift,
				)

				changed := current.isOpenShift != w.lastKnownState.isOpenShift ||
					len(current.apiGroups) != len(w.lastKnownState.apiGroups)

				if !changed {
					for i := range current.apiGroups {
						if current.apiGroups[i] != w.lastKnownState.apiGroups[i] {
							changed = true
							break
						}
					}
				}

				if changed {
					logger.V(2).Info("Cluster state changed, scheduling debounced reload")
					if w.debounceTimer != nil {
						w.debounceTimer.Stop()
					}
					w.debounceTimer = time.AfterFunc(w.debounceWindow, func() {
						logger.V(2).Info("Debounce window expired, triggering reload")
						if err := onChange(); err != nil {
							logger.Error(err, "Failed to reload")
						} else {
							next := w.captureState()
							w.mu.Lock()
							w.lastKnownState = next
							w.mu.Unlock()
							logger.V(2).Info("Reload completed")
						}
					})
				}
				w.mu.Unlock()
			}
		}
	}()
	w.mu.Unlock()
}

// Close stops the cluster state watcher
func (w *ClusterState) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	if w.stopCh == nil || w.stoppedCh == nil {
		return // Already closed
	}

	if !w.started {
		return
	}

	select {
	case <-w.stopCh:
		// Already closed or stopped
		return
	default:
		close(w.stopCh)
		w.mu.Unlock()
		<-w.stoppedCh
		w.mu.Lock()
		w.started = false
		// Recreate channels for potential restart
		w.stopCh = make(chan struct{})
		w.stoppedCh = make(chan struct{})
	}
}

func (w *ClusterState) captureState() clusterState {
	state, _ := w.capture()
	return state
}

// capture reads the cluster state and returns the error from listing the API
// groups, so the caller can tell when the API server refuses discovery.
func (w *ClusterState) capture() (clusterState, error) {
	state := clusterState{apiGroups: []string{}}
	groups, groupsErr := w.discoveryClient.ServerGroups()
	if groupsErr == nil {
		for _, group := range groups.Groups {
			state.apiGroups = append(state.apiGroups, group.Name)
		}
		sort.Strings(state.apiGroups)
	} else if isAuthError(groupsErr) {
		return state, groupsErr
	}
	// Unlike compatibility filters, the watcher treats discovery errors as "not OpenShift".
	// Failing closed avoids flapping reloads and imposing unnecessary OpenShift RBAC.
	gvk := schema.GroupVersionKind{Group: "project.openshift.io", Version: "v1", Kind: "Project"}
	resources, err := w.discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err == nil {
		for _, resource := range resources.APIResources {
			if resource.Kind == gvk.Kind {
				state.isOpenShift = true
				break
			}
		}
	}
	return state, groupsErr
}

// isAuthError reports whether the API server refused the request because the
// client is not authenticated or not allowed to run discovery. Polling again
// with the same credentials cannot succeed.
func isAuthError(err error) bool {
	return apierrors.IsUnauthorized(err) || apierrors.IsForbidden(err)
}
