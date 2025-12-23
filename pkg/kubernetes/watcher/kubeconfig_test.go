package watcher

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

const (
	// kubeconfigTestTimeout is the maximum time to wait for watcher operations
	kubeconfigTestTimeout = 500 * time.Millisecond
	// kubeconfigEventuallyTick is the polling interval for Eventually assertions
	kubeconfigEventuallyTick = time.Millisecond
)

type KubeconfigTestSuite struct {
	suite.Suite
	kubeconfigFile string
	clientConfig   clientcmd.ClientConfig
}

func (s *KubeconfigTestSuite) SetupTest() {
	// Use a short debounce window for tests
	s.T().Setenv("KUBECONFIG_DEBOUNCE_WINDOW_MS", "50")
	s.kubeconfigFile = test.KubeconfigFile(s.T(), test.KubeConfigFake())
	s.clientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: s.kubeconfigFile},
		&clientcmd.ConfigOverrides{},
	)
}

func (s *KubeconfigTestSuite) TestNewKubeconfig() {
	s.Run("creates watcher with client config", func() {
		watcher := NewKubeconfig(s.clientConfig)

		s.Run("stores client config", func() {
			s.NotNil(watcher.ClientConfig)
			s.Equal(s.clientConfig, watcher.ClientConfig)
		})
		s.Run("initializes with started as false", func() {
			s.False(watcher.started)
		})
	})
}

func (s *KubeconfigTestSuite) TestWatch() {
	s.Run("triggers onChange callback on file modification", func() {
		watcher := NewKubeconfig(s.clientConfig)
		s.T().Cleanup(watcher.Close)

		var changeDetected atomic.Bool
		onChange := func() error {
			changeDetected.Store(true)
			return nil
		}

		watcher.Watch(onChange)

		// Wait for the watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for watcher to be ready")

		// Modify the kubeconfig file
		s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))

		// Wait for change detection
		s.Eventually(func() bool {
			return changeDetected.Load()
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for onChange callback")
	})

	s.Run("does not block when no kubeconfig files exist", func() {
		clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: ""},
			&clientcmd.ConfigOverrides{},
		)
		watcher := NewKubeconfig(clientConfig)

		var completed atomic.Bool
		go func() {
			watcher.Watch(func() error { return nil })
			completed.Store(true)
		}()

		s.Eventually(func() bool {
			return completed.Load()
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "Watch blocked when no kubeconfig files exist")
	})

	s.Run("handles multiple file changes with debouncing", func() {
		watcher := NewKubeconfig(s.clientConfig)
		s.T().Cleanup(watcher.Close)

		var callCount atomic.Int32
		onChange := func() error {
			callCount.Add(1)
			return nil
		}

		watcher.Watch(onChange)

		// Wait for the watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for watcher to be ready")

		// Modify the kubeconfig file multiple times, waiting for each callback
		// to ensure we're past the debounce window before the next write
		for i := 0; i < 3; i++ {
			expectedCount := int32(i + 1)
			s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))
			s.Eventuallyf(func() bool {
				return callCount.Load() >= expectedCount
			}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for onChange callback on iteration %d", i)
		}

		s.GreaterOrEqual(callCount.Load(), int32(3), "onChange should be called at least 3 times")
	})

	s.Run("handles onChange callback errors gracefully", func() {
		watcher := NewKubeconfig(s.clientConfig)
		s.T().Cleanup(watcher.Close)

		var errorReturned atomic.Bool
		onChange := func() error {
			errorReturned.Store(true)
			return os.ErrInvalid // Return an error
		}

		watcher.Watch(onChange)

		// Wait for the watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for watcher to be ready")

		// Modify the kubeconfig file
		s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))

		// Wait for error to be returned (watcher should not panic)
		s.Eventually(func() bool {
			return errorReturned.Load()
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for onChange callback")
	})

	s.Run("ignores subsequent Watch calls when already started", func() {
		watcher := NewKubeconfig(s.clientConfig)
		s.T().Cleanup(watcher.Close)

		var firstWatcherActive atomic.Bool
		var secondWatcherActive atomic.Bool

		// Start first watcher
		watcher.Watch(func() error {
			firstWatcherActive.Store(true)
			return nil
		})

		// Wait for the first watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for first watcher to be ready")

		// Try to start second watcher (should be ignored since already started)
		watcher.Watch(func() error {
			secondWatcherActive.Store(true)
			return nil
		})

		// Modify the kubeconfig file
		s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))

		// Wait for first watcher to trigger
		s.Eventually(func() bool {
			return firstWatcherActive.Load()
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for first watcher")

		// Verify second watcher was never activated
		s.False(secondWatcherActive.Load(), "second watcher should not be activated")
	})
}

func (s *KubeconfigTestSuite) TestClose() {
	s.Run("does not panic when watcher not started", func() {
		watcher := NewKubeconfig(s.clientConfig)

		s.NotPanics(func() {
			watcher.Close()
		})
	})

	s.Run("closes watcher successfully", func() {
		watcher := NewKubeconfig(s.clientConfig)

		watcher.Watch(func() error { return nil })

		// Wait for the watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for watcher to be ready")

		s.NotPanics(func() {
			watcher.Close()
		})
	})

	s.Run("stops triggering onChange after close", func() {
		watcher := NewKubeconfig(s.clientConfig)

		var callCount atomic.Int32
		watcher.Watch(func() error {
			callCount.Add(1)
			return nil
		})

		// Wait for the watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for watcher to be ready")

		watcher.Close()

		countAfterClose := callCount.Load()

		// Modify the kubeconfig file after close
		s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))

		// Wait a reasonable amount of time to verify no callbacks are triggered
		// We expect this to never happen because no callbacks should be triggered after close
		s.Never(func() bool {
			return callCount.Load() > countAfterClose
		}, 50*time.Millisecond, kubeconfigEventuallyTick, "no callbacks should be triggered after close")
		s.Equal(countAfterClose, callCount.Load(), "call count should remain unchanged after close")
	})

	s.Run("handles multiple close calls", func() {
		watcher := NewKubeconfig(s.clientConfig)

		watcher.Watch(func() error { return nil })

		// Wait for the watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for watcher to be ready")

		s.NotPanics(func() {
			watcher.Close()
			watcher.Close()
		})
	})

	s.Run("handles close when stopCh is already closed", func() {
		watcher := NewKubeconfig(s.clientConfig)

		watcher.Watch(func() error { return nil })

		// Wait for the watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for watcher to be ready")

		// First close - normal operation
		watcher.Close()

		// Second close - should hit the "case <-w.stopCh" branch (already closed)
		s.NotPanics(func() {
			watcher.Close()
		})

		// Verify watcher is in stopped state
		s.False(watcher.started, "watcher should be stopped after close")
	})

	s.Run("handles close with nil channels", func() {
		watcher := &Kubeconfig{
			ClientConfig: s.clientConfig,
			stopCh:       nil,
			stoppedCh:    nil,
		}

		s.NotPanics(func() {
			watcher.Close()
		})
	})

	s.Run("stops debounce timer on close", func() {
		watcher := NewKubeconfig(s.clientConfig)

		var callCount atomic.Int32
		watcher.Watch(func() error {
			callCount.Add(1)
			return nil
		})

		// Wait for the watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for watcher to be ready")

		// Trigger a file change to start the debounce timer
		s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))

		// Close immediately before debounce timer fires
		time.Sleep(10 * time.Millisecond) // Small delay to ensure event is received
		watcher.Close()

		countAfterClose := callCount.Load()

		// Wait longer than debounce window to verify timer was stopped
		time.Sleep(100 * time.Millisecond)

		// The callback should not have been called after close
		// (or at most once if it was already in flight)
		s.LessOrEqual(callCount.Load(), countAfterClose+1,
			"debounce timer should be stopped on close")
	})

	s.Run("can restart watcher after close", func() {
		watcher := NewKubeconfig(s.clientConfig)

		var firstCallbackTriggered atomic.Bool
		watcher.Watch(func() error {
			firstCallbackTriggered.Store(true)
			return nil
		})

		// Wait for the watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for watcher to be ready")

		// Close the watcher
		watcher.Close()
		s.False(watcher.started, "watcher should be stopped after close")

		s.NotNil(watcher.stopCh, "stopCh should be recreated after close")
		s.NotNil(watcher.stoppedCh, "stoppedCh should be recreated after close")

		// Start a new watcher
		var secondCallbackTriggered atomic.Bool
		watcher.Watch(func() error {
			secondCallbackTriggered.Store(true)
			return nil
		})

		// Wait for the new watcher to be ready
		s.Eventually(func() bool {
			return watcher.started
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for restarted watcher to be ready")

		// Trigger a file change
		s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))

		// Wait for callback
		s.Eventually(func() bool {
			return secondCallbackTriggered.Load()
		}, kubeconfigTestTimeout, kubeconfigEventuallyTick, "timeout waiting for restarted watcher callback")

		watcher.Close()
	})
}

func TestKubeconfig(t *testing.T) {
	suite.Run(t, new(KubeconfigTestSuite))
}
