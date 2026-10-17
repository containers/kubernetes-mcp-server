package watcher

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

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
	apiResources []string
	isOpenShift  bool
}

// DiscoveryClients returns the discovery cache for every configured target.
type DiscoveryClients func(context.Context) map[string]discovery.CachedDiscoveryInterface

// ClusterState monitors cluster state changes and triggers debounced reloads
type ClusterState struct {
	discoveryClients DiscoveryClients
	pollInterval     time.Duration
	debounceWindow   time.Duration
	lastKnownState   clusterState
	debounceTimer    *time.Timer
	mu               sync.Mutex
	stopCh           chan struct{}
	stoppedCh        chan struct{}
	started          bool
}

var _ Watcher = (*ClusterState)(nil)

func NewClusterState(ctx context.Context, client discovery.CachedDiscoveryInterface, pollInterval, debounceWindow time.Duration) *ClusterState {
	return NewClusterStateForTargets(ctx, func(context.Context) map[string]discovery.CachedDiscoveryInterface {
		return map[string]discovery.CachedDiscoveryInterface{"": client}
	}, pollInterval, debounceWindow)
}

// NewClusterStateForTargets creates a watcher that compares resources across every configured target.
func NewClusterStateForTargets(ctx context.Context, clients DiscoveryClients, pollInterval, debounceWindow time.Duration) *ClusterState {
	if pollInterval <= 0 {
		pollInterval = DefaultClusterStatePollInterval
	}
	if debounceWindow <= 0 {
		debounceWindow = DefaultClusterStateDebounceWindow
	}
	logger := klogutil.FromContext(ctx)
	logger.V(2).Info("Using cluster state watcher timings", "poll_interval", pollInterval, "debounce_window", debounceWindow)

	return &ClusterState{
		discoveryClients: clients,
		pollInterval:     pollInterval,
		debounceWindow:   debounceWindow,
		stopCh:           make(chan struct{}),
		stoppedCh:        make(chan struct{}),
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

	// captureState talks to the API server; do not hold mu across it.
	// Capture before Watch returns so callers that mutate cluster state
	// immediately afterwards are compared against the pre-watch snapshot.
	initial := w.captureState(ctx, false)

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
				current := w.captureState(ctx, true)
				w.mu.Lock()
				logger.V(3).Info("Polled cluster state",
					"cluster.api_resources.count", len(current.apiResources),
					"cluster.is_openshift", current.isOpenShift,
				)

				changed := current.isOpenShift != w.lastKnownState.isOpenShift ||
					!slices.Equal(current.apiResources, w.lastKnownState.apiResources)

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
							w.mu.Lock()
							w.lastKnownState = current
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

func (w *ClusterState) captureState(ctx context.Context, invalidate bool) clusterState {
	state := clusterState{apiResources: []string{}}
	clients := w.discoveryClients(ctx)
	targets := make([]string, 0, len(clients))
	for target := range clients {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	openShiftGVK := schema.GroupVersionKind{Group: "project.openshift.io", Version: "v1", Kind: "Project"}
	for _, target := range targets {
		client := clients[target]
		if client == nil {
			state.apiResources = append(state.apiResources, target+"\x00<unknown>")
			continue
		}
		if invalidate {
			client.Invalidate()
		}
		_, resources, err := client.ServerGroupsAndResources()
		if err != nil {
			state.apiResources = append(state.apiResources, target+"\x00<partial>")
		}
		for _, list := range resources {
			for _, resource := range list.APIResources {
				state.apiResources = append(state.apiResources, fmt.Sprintf(
					"%s\x00%s\x00%s\x00%s",
					target,
					list.GroupVersion,
					resource.Name,
					resource.Kind,
				))
				if list.GroupVersion == openShiftGVK.GroupVersion().String() && resource.Kind == openShiftGVK.Kind {
					state.isOpenShift = true
				}
			}
		}
	}
	sort.Strings(state.apiResources)
	return state
}
