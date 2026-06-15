package kcp

import (
	"context"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

const (
	// DefaultWorkspacePollInterval is the default interval for polling kcp workspaces
	DefaultWorkspacePollInterval = 60 * time.Second
	// DefaultWorkspaceDebounceWindow is the default debounce window for workspace changes
	DefaultWorkspaceDebounceWindow = 5 * time.Second
)

type workspaceState struct {
	workspaces []string
}

// WorkspaceWatcher watches for changes in kcp workspaces by polling the tenancy API.
type WorkspaceWatcher struct {
	dynamicClient  dynamic.Interface
	rootWorkspace  string
	pollInterval   time.Duration
	debounceWindow time.Duration
	lastKnownState workspaceState
	debounceTimer  *time.Timer
	mu             sync.Mutex
	stopCh         chan struct{}
	stoppedCh      chan struct{}
	started        bool
}

var _ watcher.Watcher = (*WorkspaceWatcher)(nil)

// NewWorkspaceWatcher creates a new workspace watcher that polls the kcp tenancy API
// for workspace changes.
func NewWorkspaceWatcher(ctx context.Context, dynamicClient dynamic.Interface, rootWorkspace string) *WorkspaceWatcher {
	pollInterval := DefaultWorkspacePollInterval
	debounceWindow := DefaultWorkspaceDebounceWindow

	logger := klog.FromContext(ctx)

	// Allow override via environment variable for testing
	if envInterval := os.Getenv("WORKSPACE_POLL_INTERVAL_MS"); envInterval != "" {
		if ms, err := strconv.Atoi(envInterval); err == nil && ms > 0 {
			pollInterval = time.Duration(ms) * time.Millisecond
			logger.V(2).Info("Using custom workspace poll interval", "poll_interval", pollInterval)
		}
	}
	if envDebounce := os.Getenv("WORKSPACE_DEBOUNCE_WINDOW_MS"); envDebounce != "" {
		if ms, err := strconv.Atoi(envDebounce); err == nil && ms > 0 {
			debounceWindow = time.Duration(ms) * time.Millisecond
			logger.V(2).Info("Using custom workspace debounce window", "debounce_window", debounceWindow)
		}
	}

	return &WorkspaceWatcher{
		dynamicClient:  dynamicClient,
		rootWorkspace:  rootWorkspace,
		pollInterval:   pollInterval,
		debounceWindow: debounceWindow,
		stopCh:         make(chan struct{}),
		stoppedCh:      make(chan struct{}),
	}
}

// Watch starts watching for workspace changes. The onChange callback is called
// when workspace changes are detected after debouncing.
// This can only be called once per WorkspaceWatcher instance.
func (w *WorkspaceWatcher) Watch(ctx context.Context, onChange func() error) {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.lastKnownState = w.captureState(ctx)
	w.mu.Unlock()

	logger := klog.FromContext(ctx)

	go func() {
		defer close(w.stoppedCh)
		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()

		logger.V(2).Info("Started workspace watcher",
			"poll_interval", w.pollInterval,
			"debounce_window", w.debounceWindow,
		)

		for {
			select {
			case <-w.stopCh:
				logger.V(2).Info("Stopping workspace watcher")
				return
			case <-ticker.C:
				w.mu.Lock()
				current := w.captureState(ctx)
				logger.V(3).Info("Polled workspaces", "cluster.workspaces.count", len(current.workspaces))

				changed := len(current.workspaces) != len(w.lastKnownState.workspaces)
				if !changed {
					for i := range current.workspaces {
						if current.workspaces[i] != w.lastKnownState.workspaces[i] {
							changed = true
							break
						}
					}
				}

				if changed {
					logger.V(2).Info("Workspace state changed, scheduling debounced reload")
					if w.debounceTimer != nil {
						w.debounceTimer.Stop()
					}
					w.debounceTimer = time.AfterFunc(w.debounceWindow, func() {
						logger.V(2).Info("Workspace debounce window expired, triggering reload")
						if err := onChange(); err != nil {
							logger.Error(err, "Failed to reload")
						} else {
							w.mu.Lock()
							w.lastKnownState = w.captureState(ctx)
							w.mu.Unlock()
							logger.V(2).Info("Reload completed")
						}
					})
				}
				w.mu.Unlock()
			}
		}
	}()
}

// Close stops the workspace watcher and cleans up resources.
func (w *WorkspaceWatcher) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	if w.stopCh == nil || w.stoppedCh == nil {
		return
	}

	if !w.started {
		return
	}

	select {
	case <-w.stopCh:
		return
	default:
		close(w.stopCh)
		w.mu.Unlock()
		<-w.stoppedCh
		w.mu.Lock()
		w.started = false
		w.stopCh = make(chan struct{})
		w.stoppedCh = make(chan struct{})
	}
}

// captureState queries the current workspace list from the kcp tenancy API.
func (w *WorkspaceWatcher) captureState(ctx context.Context) workspaceState {
	logger := klog.FromContext(ctx)
	state := workspaceState{workspaces: []string{}}

	list, err := w.dynamicClient.Resource(WorkspaceGVR).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.V(2).Info("Unable to list workspaces from kcp API (this is expected if tenancy API is not available)", "exception.message", err.Error())
		// Return empty state - this means workspace watching won't work,
		// but the provider will still function using kubeconfig-based discovery
		return state
	}

	for _, item := range list.Items {
		// Extract workspace name
		name := item.GetName()
		if name != "" {
			state.workspaces = append(state.workspaces, name)
		}
	}

	sort.Strings(state.workspaces)
	return state
}
