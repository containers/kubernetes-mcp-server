package kubernetes

import (
	"sync"
	"testing"
	"time"
)

func TestMcpReloaderApplyToolsetsDoesNotTakeLock(t *testing.T) {
	locked := false
	r := NewMcpReloader(func(fn func() error) error {
		locked = true
		return fn()
	}, func() error { return nil })
	if err := r.ApplyToolsets(); err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("ApplyToolsets must not take the reload lock")
	}
}

func TestMcpReloaderRunThenApplyDoesNotDeadlock(t *testing.T) {
	var mu sync.Mutex
	r := NewMcpReloader(func(fn func() error) error {
		mu.Lock()
		defer mu.Unlock()
		return fn()
	}, func() error { return nil })
	done := make(chan struct{})
	go func() {
		_ = r.Run(func() error { return r.ApplyToolsets() })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ApplyToolsets inside Run deadlocked")
	}
}
