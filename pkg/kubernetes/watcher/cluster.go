package watcher

import (
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/openshift"
	"k8s.io/client-go/discovery"
	"k8s.io/klog/v2"
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

func NewClusterState(discoveryClient discovery.CachedDiscoveryInterface) *ClusterState {
	pollInterval := DefaultClusterStatePollInterval
	debounceWindow := DefaultClusterStateDebounceWindow

	// Allow override via environment variable for testing
	if envInterval := os.Getenv("CLUSTER_STATE_POLL_INTERVAL_MS"); envInterval != "" {
		if ms, err := strconv.Atoi(envInterval); err == nil && ms > 0 {
			pollInterval = time.Duration(ms) * time.Millisecond
			klog.V(2).Infof("Using custom cluster state poll interval: %v", pollInterval)
		}
	}
	if envDebounce := os.Getenv("CLUSTER_STATE_DEBOUNCE_WINDOW_MS"); envDebounce != "" {
		if ms, err := strconv.Atoi(envDebounce); err == nil && ms > 0 {
			debounceWindow = time.Duration(ms) * time.Millisecond
			klog.V(2).Infof("Using custom cluster state debounce window: %v", debounceWindow)
		}
	}

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
func (w *ClusterState) Watch(onChange func() error) {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.lastKnownState = w.captureState()
	w.mu.Unlock()

	// Start background monitoring
	go func() {
		defer close(w.stoppedCh)
		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()

		klog.V(2).Infof("Started cluster state watcher (poll interval: %v, debounce: %v)", w.pollInterval, w.debounceWindow)

		for {
			select {
			case <-w.stopCh:
				klog.V(2).Info("Stopping cluster state watcher")
				return
			case <-ticker.C:
				// Invalidate discovery cache to get fresh API groups
				w.discoveryClient.Invalidate()

				w.mu.Lock()
				current := w.captureState()
				klog.V(3).Infof("Polled cluster state: %d API groups, OpenShift=%v", len(current.apiGroups), current.isOpenShift)

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
					klog.V(2).Info("Cluster state changed, scheduling debounced reload")
					if w.debounceTimer != nil {
						w.debounceTimer.Stop()
					}
					w.debounceTimer = time.AfterFunc(w.debounceWindow, func() {
						klog.V(2).Info("Debounce window expired, triggering reload")
						if err := onChange(); err != nil {
							klog.Errorf("Failed to reload: %v", err)
						} else {
							w.mu.Lock()
							w.lastKnownState = w.captureState()
							w.mu.Unlock()
							klog.V(2).Info("Reload completed")
						}
					})
				}
				w.mu.Unlock()
			}
		}
	}()
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
	state := clusterState{apiGroups: []string{}}
	if groups, err := w.discoveryClient.ServerGroups(); err == nil {
		for _, group := range groups.Groups {
			state.apiGroups = append(state.apiGroups, group.Name)
		}
		sort.Strings(state.apiGroups)
	}
	state.isOpenShift = openshift.IsOpenshift(w.discoveryClient)
	return state
}
