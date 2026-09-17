package watcher

import (
	"context"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

const (
	// DefaultKubeconfigDebounceWindow is the default debounce window for kubeconfig file changes
	DefaultKubeconfigDebounceWindow = 100 * time.Millisecond
)

type Kubeconfig struct {
	clientcmd.ClientConfig
	debounceWindow time.Duration
	debounceTimer  *time.Timer
	mu             sync.Mutex
	stopCh         chan struct{}
	stoppedCh      chan struct{}
	started        bool
}

var _ Watcher = (*Kubeconfig)(nil)

func NewKubeconfig(ctx context.Context, clientConfig clientcmd.ClientConfig, debounceWindow time.Duration) *Kubeconfig {
	if debounceWindow <= 0 {
		debounceWindow = DefaultKubeconfigDebounceWindow
	}
	klogutil.FromContext(ctx).V(2).Info("Using kubeconfig debounce window", "debounce_window", debounceWindow)

	return &Kubeconfig{
		ClientConfig:   clientConfig,
		debounceWindow: debounceWindow,
		stopCh:         make(chan struct{}),
		stoppedCh:      make(chan struct{}),
	}
}

// Watch starts a background watcher that monitors kubeconfig file changes
// and triggers a debounced reload when changes are detected.
// It can only be called once per Kubeconfig instance.
func (w *Kubeconfig) Watch(ctx context.Context, onChange func() error) {
	logger := klogutil.FromContext(ctx)

	kubeConfigFiles := w.ConfigAccess().GetLoadingPrecedence()
	if len(kubeConfigFiles) == 0 {
		logger.V(2).Info("No kubeconfig files to watch")
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error(err, "Failed to create kubeconfig file watcher")
		return
	}
	for _, file := range kubeConfigFiles {
		_ = watcher.Add(file)
	}

	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		_ = watcher.Close()
		return
	}
	w.started = true
	go func() {
		defer close(w.stoppedCh)
		defer func() { _ = watcher.Close() }()

		logger.V(2).Info("Started kubeconfig watcher", "debounce_window", w.debounceWindow)

		for {
			select {
			case <-w.stopCh:
				logger.V(2).Info("Stopping kubeconfig watcher")
				return
			case _, ok := <-watcher.Events:
				if !ok {
					return
				}
				w.mu.Lock()
				logger.V(3).Info("Kubeconfig file change detected, scheduling debounced reload")
				if w.debounceTimer != nil {
					w.debounceTimer.Stop()
				}
				w.debounceTimer = time.AfterFunc(w.debounceWindow, func() {
					logger.V(2).Info("Kubeconfig debounce window expired, triggering reload")
					if err := onChange(); err != nil {
						logger.Error(err, "Failed to reload after kubeconfig change")
					}
				})
				w.mu.Unlock()
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	w.mu.Unlock()
}

// Close stops the kubeconfig watcher
func (w *Kubeconfig) Close() {
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
