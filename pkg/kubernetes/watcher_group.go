package kubernetes

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
)

// WatcherGroup is the close/reload lifecycle shared by cluster providers.
// primary is the watcher whose callback rebuilds targets (kubeconfig or
// workspace). cluster receives the cluster-state callback.
//
// The group lock is separate from the provider lock. reset holds the provider
// lock and then Replace, so Close can wait for a watcher swap without
// re-entering that lock.
type WatcherGroup struct {
	mu      sync.RWMutex
	closed  bool
	reset   func(context.Context) error
	primary watcher.Watcher
	cluster watcher.Watcher
}

// errProviderClosed is returned by a provider reload that arrives after Close.
// Callers swallow it so shutdown does not surface as a reload failure.
var errProviderClosed = errors.New("provider closed")

// SetReset stores the provider callback WatchTargets runs to rebuild targets.
// Call it before the provider is shared. It is not synchronized with WatchTargets.
func (g *WatcherGroup) SetReset(reset func(context.Context) error) {
	g.reset = reset
}

// ErrClosed reports whether Close has already run.
func (g *WatcherGroup) ErrClosed() error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.closed {
		return errProviderClosed
	}
	return nil
}

// Replace stops the current watchers and installs the next pair.
// After Close, the next pair is stopped instead of installed.
func (g *WatcherGroup) Replace(primary, cluster watcher.Watcher) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		closeWatcher(primary)
		closeWatcher(cluster)
		return errProviderClosed
	}
	closeWatcher(g.primary)
	closeWatcher(g.cluster)
	g.primary = primary
	g.cluster = cluster
	return nil
}

// WatchTargets starts both watchers. primary's callback runs the reset stored
// by SetReset, then watches again. Embedding WatcherGroup promotes this method.
func (g *WatcherGroup) WatchTargets(ctx context.Context, reload McpReloader) {
	reset := g.reset
	reloadWithReset := func() error {
		return reload.Run(func() error {
			if err := reset(ctx); err != nil {
				if errors.Is(err, errProviderClosed) {
					return nil
				}
				return err
			}
			g.WatchTargets(ctx, reload)
			return reload.ApplyToolsets()
		})
	}
	// Copy under the lock. Replace writes these pointers, and Watch talks to
	// the API server, so it must not run with the group lock held.
	g.mu.RLock()
	closed := g.closed
	primary := g.primary
	cluster := g.cluster
	g.mu.RUnlock()
	if closed {
		return
	}
	if !watcherIsNil(primary) {
		primary.Watch(ctx, reloadWithReset)
	}
	if !watcherIsNil(cluster) {
		cluster.Watch(ctx, reload.ClusterStateCallback())
	}
	// Close can win the race after the snapshot above and return before Watch
	// runs. Stop anything that started in that window.
	g.mu.RLock()
	closed = g.closed
	g.mu.RUnlock()
	if closed {
		g.Close()
	}
}

// Close stops further reloads and the current watchers.
func (g *WatcherGroup) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
	closeWatcher(g.primary)
	closeWatcher(g.cluster)
}

func closeWatcher(w watcher.Watcher) {
	if watcherIsNil(w) {
		return
	}
	w.Close()
}

func watcherIsNil(w watcher.Watcher) bool {
	return w == nil || reflect.ValueOf(w).IsNil()
}
