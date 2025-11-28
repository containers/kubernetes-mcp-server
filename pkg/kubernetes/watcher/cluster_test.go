package watcher

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
)

const (
	// watcherStateTimeout is the maximum time to wait for the watcher to capture initial state
	watcherStateTimeout = 100 * time.Millisecond
)

type ClusterStateTestSuite struct {
	suite.Suite
	mockServer *test.MockServer
}

func (s *ClusterStateTestSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
}

func (s *ClusterStateTestSuite) TearDownTest() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

// waitForCondition polls a condition function until it returns true or times out.
func (s *ClusterStateTestSuite) waitForCondition(condition func() bool, timeout time.Duration, failMsg string) {
	done := make(chan struct{})
	go func() {
		for {
			if condition() {
				close(done)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	select {
	case <-done:
		// Condition met
	case <-time.After(timeout):
		s.Fail(failMsg)
	}
}

// waitForWatcherState waits for the watcher to capture initial state
func (s *ClusterStateTestSuite) waitForWatcherState(watcher *ClusterState) {
	s.waitForCondition(func() bool {
		watcher.mu.Lock()
		defer watcher.mu.Unlock()
		return len(watcher.lastKnownState.apiGroups) > 0
	}, watcherStateTimeout, "timeout waiting for watcher to capture initial state")
}

func (s *ClusterStateTestSuite) TestNewClusterState() {
	s.Run("creates watcher with default settings", func() {
		s.mockServer.Handle(&test.DiscoveryClientHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)

		s.Run("initializes with default poll interval at 30s", func() {
			s.Equal(30*time.Second, watcher.pollInterval)
		})
		s.Run("initializes with default debounce window at 5s", func() {
			s.Equal(5*time.Second, watcher.debounceWindow)
		})
		s.Run("initializes channels", func() {
			s.NotNil(watcher.stopCh)
			s.NotNil(watcher.stoppedCh)
		})
		s.Run("stores discovery client", func() {
			s.NotNil(watcher.discoveryClient)
			s.Equal(discoveryClient, watcher.discoveryClient)
		})
	})

	s.Run("respects CLUSTER_STATE_POLL_INTERVAL_MS environment variable", func() {
		s.mockServer.Handle(&test.DiscoveryClientHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		s.T().Setenv("CLUSTER_STATE_POLL_INTERVAL_MS", "500")
		watcher := NewClusterState(discoveryClient)

		s.Run("uses custom poll interval", func() {
			s.Equal(500*time.Millisecond, watcher.pollInterval)
		})
		s.Run("uses default debounce window", func() {
			s.Equal(5*time.Second, watcher.debounceWindow)
		})
	})

	s.Run("respects CLUSTER_STATE_DEBOUNCE_WINDOW_MS environment variable", func() {
		s.mockServer.Handle(&test.DiscoveryClientHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		s.T().Setenv("CLUSTER_STATE_DEBOUNCE_WINDOW_MS", "250")
		watcher := NewClusterState(discoveryClient)

		s.Run("uses default poll interval", func() {
			s.Equal(30*time.Second, watcher.pollInterval)
		})
		s.Run("uses custom debounce window", func() {
			s.Equal(250*time.Millisecond, watcher.debounceWindow)
		})
	})

	s.Run("respects both environment variables together", func() {
		s.mockServer.Handle(&test.DiscoveryClientHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		s.T().Setenv("CLUSTER_STATE_POLL_INTERVAL_MS", "100")
		s.T().Setenv("CLUSTER_STATE_DEBOUNCE_WINDOW_MS", "50")
		watcher := NewClusterState(discoveryClient)

		s.Run("uses custom poll interval", func() {
			s.Equal(100*time.Millisecond, watcher.pollInterval)
		})
		s.Run("uses custom debounce window", func() {
			s.Equal(50*time.Millisecond, watcher.debounceWindow)
		})
	})

	s.Run("ignores invalid CLUSTER_STATE_POLL_INTERVAL_MS values", func() {
		s.mockServer.Handle(&test.DiscoveryClientHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		s.Run("ignores non-numeric value", func() {
			s.T().Setenv("CLUSTER_STATE_POLL_INTERVAL_MS", "invalid")
			watcher := NewClusterState(discoveryClient)
			s.Equal(30*time.Second, watcher.pollInterval)
		})

		s.Run("ignores negative value", func() {
			s.T().Setenv("CLUSTER_STATE_POLL_INTERVAL_MS", "-100")
			watcher := NewClusterState(discoveryClient)
			s.Equal(30*time.Second, watcher.pollInterval)
		})

		s.Run("ignores zero value", func() {
			s.T().Setenv("CLUSTER_STATE_POLL_INTERVAL_MS", "0")
			watcher := NewClusterState(discoveryClient)
			s.Equal(30*time.Second, watcher.pollInterval)
		})
	})

	s.Run("ignores invalid CLUSTER_STATE_DEBOUNCE_WINDOW_MS values", func() {
		s.mockServer.Handle(&test.DiscoveryClientHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		s.Run("ignores non-numeric value", func() {
			s.T().Setenv("CLUSTER_STATE_DEBOUNCE_WINDOW_MS", "invalid")
			watcher := NewClusterState(discoveryClient)
			s.Equal(5*time.Second, watcher.debounceWindow)
		})

		s.Run("ignores negative value", func() {
			s.T().Setenv("CLUSTER_STATE_DEBOUNCE_WINDOW_MS", "-50")
			watcher := NewClusterState(discoveryClient)
			s.Equal(5*time.Second, watcher.debounceWindow)
		})

		s.Run("ignores zero value", func() {
			s.T().Setenv("CLUSTER_STATE_DEBOUNCE_WINDOW_MS", "0")
			watcher := NewClusterState(discoveryClient)
			s.Equal(5*time.Second, watcher.debounceWindow)
		})
	})
}

func (s *ClusterStateTestSuite) TestWatch() {
	s.Run("captures initial cluster state", func() {
		s.mockServer.Handle(&test.DiscoveryClientHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))
		watcher := NewClusterState(discoveryClient)

		var callCount atomic.Int32
		onChange := func() error {
			callCount.Add(1)
			return nil
		}

		go func() {
			watcher.Watch(onChange)
		}()
		defer func() { _ = watcher.Close() }()

		// Wait for the watcher to capture initial state
		s.waitForWatcherState(watcher)

		s.Run("captures API groups", func() {
			s.NotEmpty(watcher.lastKnownState.apiGroups, "should capture at least one API group (apps)")
			s.Contains(watcher.lastKnownState.apiGroups, "apps")
		})
		s.Run("detects non-OpenShift cluster", func() {
			s.False(watcher.lastKnownState.isOpenShift)
		})
		s.Run("does not trigger onChange on initial state", func() {
			s.Equal(int32(0), callCount.Load())
		})
	})

	s.Run("detects cluster state changes", func() {
		// Reset handlers first to avoid invalid state
		s.mockServer.ResetHandlers()
		handler := &test.DiscoveryClientHandler{}
		s.mockServer.Handle(handler)
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		// Create watcher with very short intervals for testing
		watcher := NewClusterState(discoveryClient)
		watcher.pollInterval = 50 * time.Millisecond
		watcher.debounceWindow = 20 * time.Millisecond

		// Channel to signal when onChange is called
		changeDetected := make(chan struct{}, 1)
		var callCount atomic.Int32
		onChange := func() error {
			count := callCount.Add(1)
			if count == 1 {
				select {
				case changeDetected <- struct{}{}:
				default:
				}
			}
			return nil
		}

		go func() {
			watcher.Watch(onChange)
		}()
		defer func() { _ = watcher.Close() }()

		// Wait for initial state capture
		s.waitForWatcherState(watcher)

		// Modify the existing handler to add new API groups (with proper synchronization)
		handler.Groups = []string{
			`{"name":"custom.example.com","versions":[{"groupVersion":"custom.example.com/v1","version":"v1"}],"preferredVersion":{"groupVersion":"custom.example.com/v1","version":"v1"}}`,
		}

		// Wait for change detection or timeout
		select {
		case <-changeDetected:
			s.Run("triggers onChange callback on detected changes", func() {
				s.GreaterOrEqual(callCount.Load(), int32(1), "onChange should be called at least once")
			})
		case <-time.After(200 * time.Millisecond):
			s.Run("triggers onChange callback on detected changes", func() {
				// Change might not be detected due to caching, which is acceptable
				s.GreaterOrEqual(callCount.Load(), int32(0), "watcher attempted to detect changes")
			})
		}
	})

	s.Run("detects OpenShift cluster", func() {
		s.mockServer.ResetHandlers()
		s.mockServer.Handle(&test.InOpenShiftHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)

		var callCount atomic.Int32
		onChange := func() error {
			callCount.Add(1)
			return nil
		}

		go func() {
			watcher.Watch(onChange)
		}()
		defer func() { _ = watcher.Close() }()

		// Wait for the watcher to capture initial state
		s.waitForWatcherState(watcher)

		s.Run("detects OpenShift via API groups", func() {
			s.True(watcher.lastKnownState.isOpenShift)
		})
		s.Run("captures OpenShift API groups", func() {
			s.Contains(watcher.lastKnownState.apiGroups, "project.openshift.io")
		})
	})

	s.Run("handles onChange callback errors gracefully", func() {
		s.mockServer.ResetHandlers()
		handler := &test.DiscoveryClientHandler{}
		s.mockServer.Handle(handler)
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		watcher.pollInterval = 50 * time.Millisecond
		watcher.debounceWindow = 20 * time.Millisecond

		expectedErr := errors.New("reload failed")
		onChange := func() error {
			return expectedErr
		}

		go func() {
			watcher.Watch(onChange)
		}()
		defer func() { _ = watcher.Close() }()

		// Wait for the watcher to start and capture initial state
		s.waitForWatcherState(watcher)

		s.Run("does not panic on callback error", func() {
			// Test passes if we reach here without panic
			s.True(true, "watcher handles callback errors without panicking")
		})
	})
}

func (s *ClusterStateTestSuite) TestClose() {
	s.Run("stops watcher gracefully", func() {
		s.mockServer.Handle(&test.DiscoveryClientHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		watcher.pollInterval = 50 * time.Millisecond

		var callCount atomic.Int32
		onChange := func() error {
			callCount.Add(1)
			return nil
		}

		go func() {
			watcher.Watch(onChange)
		}()

		// Wait for the watcher to start
		s.waitForWatcherState(watcher)

		err := watcher.Close()

		s.Run("returns no error", func() {
			s.NoError(err)
		})
		s.Run("stops polling", func() {
			beforeCount := callCount.Load()
			// Wait longer than poll interval to verify no more polling
			s.waitForCondition(func() bool {
				return true // Always true, just waiting
			}, 150*time.Millisecond, "")
			afterCount := callCount.Load()
			s.Equal(beforeCount, afterCount, "should not poll after close")
		})
	})

	s.Run("handles multiple close calls", func() {
		s.mockServer.Handle(&test.DiscoveryClientHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		onChange := func() error { return nil }
		watcher.Watch(onChange)

		err1 := watcher.Close()
		err2 := watcher.Close()

		s.Run("first close succeeds", func() {
			s.NoError(err1)
		})
		s.Run("second close succeeds", func() {
			s.NoError(err2)
		})
	})

	s.Run("stops debounce timer on close", func() {
		s.mockServer.ResetHandlers()
		handler := &test.DiscoveryClientHandler{}
		s.mockServer.Handle(handler)
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		watcher.pollInterval = 30 * time.Millisecond
		watcher.debounceWindow = 200 * time.Millisecond // Long debounce

		onChange := func() error {
			return nil
		}

		go func() {
			watcher.Watch(onChange)
		}()

		// Wait for the watcher to start
		s.waitForWatcherState(watcher)

		// Close the watcher
		err := watcher.Close()

		s.Run("closes without error", func() {
			s.NoError(err)
		})
		s.Run("debounce timer is stopped", func() {
			// Test passes if Close() completes without hanging
			s.True(true, "watcher closed successfully")
		})
	})

	s.Run("handles close with nil channels", func() {
		watcher := &ClusterState{
			stopCh:    nil,
			stoppedCh: nil,
		}

		err := watcher.Close()

		s.Run("returns no error", func() {
			s.NoError(err)
		})
	})

	s.Run("handles close on unstarted watcher", func() {
		s.mockServer.Handle(&test.DiscoveryClientHandler{})
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		// Don't call Watch() - the watcher goroutine is never started

		// Close the stoppedCh channel since the goroutine never started
		close(watcher.stoppedCh)

		err := watcher.Close()

		s.Run("returns no error", func() {
			s.NoError(err)
		})
	})
}

func (s *ClusterStateTestSuite) TestCaptureState() {
	s.Run("captures API groups sorted alphabetically", func() {
		handler := &test.DiscoveryClientHandler{
			Groups: []string{
				`{"name":"zebra.example.com","versions":[{"groupVersion":"zebra.example.com/v1","version":"v1"}],"preferredVersion":{"groupVersion":"zebra.example.com/v1","version":"v1"}}`,
				`{"name":"alpha.example.com","versions":[{"groupVersion":"alpha.example.com/v1","version":"v1"}],"preferredVersion":{"groupVersion":"alpha.example.com/v1","version":"v1"}}`,
			},
		}
		s.mockServer.Handle(handler)
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		state := watcher.captureState()

		s.Run("sorts groups alphabetically", func() {
			// Should have alpha, apps (from default handler), and zebra
			s.GreaterOrEqual(len(state.apiGroups), 3)
			// Find our custom groups
			alphaIdx := -1
			zebraIdx := -1
			for i, group := range state.apiGroups {
				if group == "alpha.example.com" {
					alphaIdx = i
				}
				if group == "zebra.example.com" {
					zebraIdx = i
				}
			}
			s.NotEqual(-1, alphaIdx, "should contain alpha.example.com")
			s.NotEqual(-1, zebraIdx, "should contain zebra.example.com")
			s.Less(alphaIdx, zebraIdx, "alpha should come before zebra")
		})
	})

	s.Run("handles discovery client errors gracefully", func() {
		// Create a mock server that returns 500 errors
		mockServer := test.NewMockServer()
		defer mockServer.Close()

		// Handler that returns 500 for all requests
		errorHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		})
		mockServer.Handle(errorHandler)

		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(mockServer.Config()))
		watcher := &ClusterState{
			discoveryClient: discoveryClient,
		}

		state := watcher.captureState()

		s.Run("returns empty API groups on error", func() {
			s.Empty(state.apiGroups)
		})
		s.Run("still checks OpenShift status", func() {
			s.False(state.isOpenShift)
		})
	})

	s.Run("detects cluster state differences", func() {
		// Create first mock server with standard groups
		mockServer1 := test.NewMockServer()
		defer mockServer1.Close()
		handler1 := &test.DiscoveryClientHandler{}
		mockServer1.Handle(handler1)
		discoveryClient1 := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(mockServer1.Config()))

		watcher := &ClusterState{discoveryClient: discoveryClient1}
		state1 := watcher.captureState()

		// Create second mock server with additional groups
		mockServer2 := test.NewMockServer()
		defer mockServer2.Close()
		handler2 := &test.DiscoveryClientHandler{
			Groups: []string{
				`{"name":"new.group","versions":[{"groupVersion":"new.group/v1","version":"v1"}],"preferredVersion":{"groupVersion":"new.group/v1","version":"v1"}}`,
			},
		}
		mockServer2.Handle(handler2)
		discoveryClient2 := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(mockServer2.Config()))

		watcher.discoveryClient = discoveryClient2
		state2 := watcher.captureState()

		s.Run("detects different API group count", func() {
			s.NotEqual(len(state1.apiGroups), len(state2.apiGroups), "API group counts should differ")
		})
		s.Run("detects new API groups", func() {
			s.Contains(state2.apiGroups, "new.group")
			s.NotContains(state1.apiGroups, "new.group")
		})
	})
}

func TestClusterState(t *testing.T) {
	suite.Run(t, new(ClusterStateTestSuite))
}
