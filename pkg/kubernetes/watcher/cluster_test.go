package watcher

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

const (
	// eventuallyTick is the polling interval for Eventually assertions
	eventuallyTick = time.Millisecond
	// watcherStateTimeout is the maximum time to wait for the watcher to capture initial state
	watcherStateTimeout = 100 * time.Millisecond
	watcherPollTimeout  = 250 * time.Millisecond
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

// waitForWatcherInitialState waits for the watcher to capture initial state
func (s *ClusterStateTestSuite) waitForWatcherInitialState(watcher *ClusterState) {
	s.Eventually(func() bool {
		watcher.mu.Lock()
		defer watcher.mu.Unlock()
		return len(watcher.lastKnownState.apiGroups) > 0
	}, watcherStateTimeout, eventuallyTick, "timeout waiting for watcher to capture initial state")
}

func (s *ClusterStateTestSuite) TestNewClusterState() {
	s.Run("creates watcher with default settings", func() {
		s.mockServer.Handle(test.NewDiscoveryClientHandler())
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
		s.mockServer.Handle(test.NewDiscoveryClientHandler())
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
		s.mockServer.Handle(test.NewDiscoveryClientHandler())
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
		s.mockServer.Handle(test.NewDiscoveryClientHandler())
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
		s.mockServer.Handle(test.NewDiscoveryClientHandler())
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
		s.mockServer.Handle(test.NewDiscoveryClientHandler())
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
		s.mockServer.Handle(test.NewDiscoveryClientHandler())
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
		s.T().Cleanup(watcher.Close)

		s.waitForWatcherInitialState(watcher)

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
		s.mockServer.ResetHandlers()
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		// Create watcher with very short intervals for testing
		watcher := NewClusterState(discoveryClient)
		watcher.pollInterval = 50 * time.Millisecond
		watcher.debounceWindow = 20 * time.Millisecond

		var callCount atomic.Int32
		onChange := func() error {
			callCount.Add(1)
			return nil
		}

		go func() {
			watcher.Watch(onChange)
		}()
		s.T().Cleanup(watcher.Close)

		s.waitForWatcherInitialState(watcher)

		// Modify the handler to add new API groups
		handler.AddAPIResourceList(metav1.APIResourceList{GroupVersion: "custom.example.com/v1"})

		// Wait for change detection - the watcher invalidates the cache on each poll
		s.Eventually(func() bool {
			return callCount.Load() >= 1
		}, watcherPollTimeout, eventuallyTick, "timeout waiting for onChange callback")

		s.GreaterOrEqual(callCount.Load(), int32(1), "onChange should be called at least once")
	})

	s.Run("detects OpenShift cluster", func() {
		s.mockServer.ResetHandlers()
		s.mockServer.Handle(test.NewInOpenShiftHandler())
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
		s.T().Cleanup(watcher.Close)

		// Wait for the watcher to capture initial state
		s.waitForWatcherInitialState(watcher)

		s.Run("detects OpenShift via API groups", func() {
			s.True(watcher.lastKnownState.isOpenShift)
		})
		s.Run("captures OpenShift API groups", func() {
			s.Contains(watcher.lastKnownState.apiGroups, "project.openshift.io")
		})
	})

	s.Run("handles onChange callback errors gracefully", func() {
		s.mockServer.ResetHandlers()
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		watcher.pollInterval = 50 * time.Millisecond
		watcher.debounceWindow = 20 * time.Millisecond

		var errorCallCount atomic.Int32
		expectedErr := errors.New("reload failed")
		onChange := func() error {
			errorCallCount.Add(1)
			return expectedErr
		}

		go func() {
			watcher.Watch(onChange)
		}()
		s.T().Cleanup(watcher.Close)

		// Wait for the watcher to start and capture initial state
		s.waitForWatcherInitialState(watcher)

		// Modify the handler to trigger a change
		handler.AddAPIResourceList(metav1.APIResourceList{GroupVersion: "error.trigger/v1"})

		// Wait for onChange to be called (which returns an error)
		s.Eventually(func() bool {
			return errorCallCount.Load() >= 1
		}, watcherPollTimeout, eventuallyTick, "timeout waiting for onChange callback")

		s.GreaterOrEqual(errorCallCount.Load(), int32(1), "onChange should be called even when it returns an error")
	})
}

func (s *ClusterStateTestSuite) TestClose() {
	s.Run("stops watcher gracefully", func() {
		s.mockServer.Handle(test.NewDiscoveryClientHandler())
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		watcher.pollInterval = 50 * time.Millisecond
		watcher.debounceWindow = 10 * time.Millisecond

		var callCount atomic.Int32
		onChange := func() error {
			callCount.Add(1)
			return nil
		}

		go func() {
			watcher.Watch(onChange)
		}()

		// Wait for the watcher to start
		s.waitForWatcherInitialState(watcher)

		watcher.Close()

		s.Run("stops polling", func() {
			beforeCount := callCount.Load()
			// Wait longer than poll interval to verify no more polling
			// We expect this to never happen because no callbacks should be triggered after close
			s.Never(func() bool {
				return callCount.Load() > beforeCount
			}, watcherPollTimeout, eventuallyTick, "should not poll after close")
			afterCount := callCount.Load()
			s.Equal(beforeCount, afterCount, "should not poll after close")
		})
	})

	s.Run("handles multiple close calls", func() {
		s.mockServer.Handle(test.NewDiscoveryClientHandler())
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		onChange := func() error { return nil }
		watcher.Watch(onChange)

		s.NotPanics(func() {
			watcher.Close()
			watcher.Close()
		})
	})

	s.Run("stops debounce timer on close", func() {
		s.mockServer.ResetHandlers()
		handler := test.NewDiscoveryClientHandler()
		s.mockServer.Handle(handler)
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		watcher.pollInterval = 30 * time.Millisecond
		watcher.debounceWindow = 500 * time.Millisecond // Long debounce window

		var callCount atomic.Int32
		onChange := func() error {
			callCount.Add(1)
			return nil
		}

		go func() {
			watcher.Watch(onChange)
		}()

		// Wait for the watcher to start
		s.waitForWatcherInitialState(watcher)

		// Modify the handler to trigger a change and start the debounce timer
		handler.AddAPIResourceList(metav1.APIResourceList{GroupVersion: "trigger.change/v1"})

		// Wait for the change to be detected (debounce timer starts)
		s.Eventually(func() bool {
			watcher.mu.Lock()
			defer watcher.mu.Unlock()
			return watcher.debounceTimer != nil
		}, watcherPollTimeout, eventuallyTick, "timeout waiting for debounce timer to start")

		// Close the watcher before debounce window expires
		watcher.Close()

		s.Run("debounce timer is stopped", func() {
			// Verify onChange was not called (debounce timer was stopped)
			s.Equal(int32(0), callCount.Load(), "onChange should not be called because debounce timer was stopped")
		})
	})

	s.Run("handles close with nil channels", func() {
		watcher := &ClusterState{
			stopCh:    nil,
			stoppedCh: nil,
		}

		s.NotPanics(watcher.Close)
	})

	s.Run("handles close on unstarted watcher", func() {
		s.mockServer.Handle(test.NewDiscoveryClientHandler())
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(s.mockServer.Config()))

		watcher := NewClusterState(discoveryClient)
		// Don't call Watch() - the watcher goroutine is never started

		// Close the stoppedCh channel since the goroutine never started
		close(watcher.stoppedCh)

		s.NotPanics(watcher.Close)
	})
}

func (s *ClusterStateTestSuite) TestCaptureState() {
	s.Run("captures API groups sorted alphabetically", func() {
		handler := test.NewDiscoveryClientHandler(
			metav1.APIResourceList{GroupVersion: "zebra.example.com/v1"},
			metav1.APIResourceList{GroupVersion: "alpha.example.com/v1"},
		)
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
		handler1 := test.NewDiscoveryClientHandler()
		mockServer1.Handle(handler1)
		discoveryClient1 := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(mockServer1.Config()))

		watcher := &ClusterState{discoveryClient: discoveryClient1}
		state1 := watcher.captureState()

		// Create second mock server with additional groups
		mockServer2 := test.NewMockServer()
		defer mockServer2.Close()
		handler2 := test.NewDiscoveryClientHandler(
			metav1.APIResourceList{GroupVersion: "new.group/v1"},
		)
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
