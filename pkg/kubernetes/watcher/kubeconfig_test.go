package watcher

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// kubeconfigTestTimeout is the maximum time to wait for watcher operations
	kubeconfigTestTimeout = 500 * time.Millisecond
)

type KubeconfigTestSuite struct {
	suite.Suite
	kubeconfigFile string
	clientConfig   clientcmd.ClientConfig
}

func (s *KubeconfigTestSuite) SetupTest() {
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
		s.Run("initializes with nil close function", func() {
			s.Nil(watcher.close)
		})
	})
}

func (s *KubeconfigTestSuite) TestWatch() {
	s.Run("triggers onChange callback on file modification", func() {
		watcher := NewKubeconfig(s.clientConfig)
		defer func() { _ = watcher.Close() }()

		var changeDetected atomic.Bool
		onChange := func() error {
			changeDetected.Store(true)
			return nil
		}

		watcher.Watch(onChange)

		// Wait for the watcher to be ready
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return watcher.close != nil
		}), "timeout waiting for watcher to be ready")

		// Modify the kubeconfig file
		s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))

		// Wait for change detection
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return changeDetected.Load()
		}), "timeout waiting for onChange callback")
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

		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return completed.Load()
		}), "Watch blocked when no kubeconfig files exist")
	})

	s.Run("handles multiple file changes", func() {
		watcher := NewKubeconfig(s.clientConfig)
		defer func() { _ = watcher.Close() }()

		var callCount atomic.Int32
		onChange := func() error {
			callCount.Add(1)
			return nil
		}

		watcher.Watch(onChange)

		// Wait for the watcher to be ready
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return watcher.close != nil
		}), "timeout waiting for watcher to be ready")

		// Modify the kubeconfig file multiple times and wait for each change
		for i := 0; i < 3; i++ {
			expectedCount := int32(i + 1)
			s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))
			s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
				return callCount.Load() >= expectedCount
			}), "timeout waiting for onChange callback on iteration %d", i)
		}

		s.GreaterOrEqual(callCount.Load(), int32(3), "onChange should be called at least 3 times")
	})

	s.Run("handles onChange callback errors gracefully", func() {
		watcher := NewKubeconfig(s.clientConfig)
		defer func() { _ = watcher.Close() }()

		var errorReturned atomic.Bool
		onChange := func() error {
			errorReturned.Store(true)
			return os.ErrInvalid // Return an error
		}

		watcher.Watch(onChange)

		// Wait for the watcher to be ready
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return watcher.close != nil
		}), "timeout waiting for watcher to be ready")

		// Modify the kubeconfig file
		s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))

		// Wait for error to be returned (watcher should not panic)
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return errorReturned.Load()
		}), "timeout waiting for onChange callback")
	})

	s.Run("replaces previous watcher on subsequent Watch calls", func() {
		watcher := NewKubeconfig(s.clientConfig)
		defer func() { _ = watcher.Close() }()

		var secondWatcherActive atomic.Bool

		// Start first watcher
		watcher.Watch(func() error {
			return nil
		})

		// Wait for the first watcher to be ready
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return watcher.close != nil
		}), "timeout waiting for first watcher to be ready")

		// Start second watcher (should close the first)
		watcher.Watch(func() error {
			secondWatcherActive.Store(true)
			return nil
		})

		// Wait for the second watcher to be ready
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return watcher.close != nil
		}), "timeout waiting for second watcher to be ready")

		// Modify the kubeconfig file
		s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))

		// Wait for second watcher to trigger
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return secondWatcherActive.Load()
		}), "timeout waiting for second watcher")
	})
}

func (s *KubeconfigTestSuite) TestClose() {
	s.Run("returns no error when close is nil", func() {
		watcher := &Kubeconfig{
			close: nil,
		}

		err := watcher.Close()

		s.NoError(err)
	})

	s.Run("closes watcher successfully", func() {
		watcher := NewKubeconfig(s.clientConfig)

		watcher.Watch(func() error { return nil })

		// Wait for the watcher to be ready
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return watcher.close != nil
		}), "timeout waiting for watcher to be ready")

		err := watcher.Close()

		s.NoError(err)
	})

	s.Run("stops triggering onChange after close", func() {
		watcher := NewKubeconfig(s.clientConfig)

		var callCount atomic.Int32
		watcher.Watch(func() error {
			callCount.Add(1)
			return nil
		})

		// Wait for the watcher to be ready
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return watcher.close != nil
		}), "timeout waiting for watcher to be ready")

		err := watcher.Close()
		s.NoError(err)

		countAfterClose := callCount.Load()

		// Modify the kubeconfig file after close
		s.Require().NoError(clientcmd.WriteToFile(*test.KubeConfigFake(), s.kubeconfigFile))

		// Wait a reasonable amount of time to verify no callbacks are triggered
		// Using WaitForCondition with a condition that should NOT become true
		err = test.WaitForCondition(50*time.Millisecond, func() bool {
			return callCount.Load() > countAfterClose
		})
		// We expect this to timeout (return error) because no callbacks should be triggered
		s.Error(err, "no callbacks should be triggered after close")
		s.Equal(countAfterClose, callCount.Load(), "call count should remain unchanged after close")
	})

	s.Run("handles multiple close calls", func() {
		watcher := NewKubeconfig(s.clientConfig)

		watcher.Watch(func() error { return nil })

		// Wait for the watcher to be ready
		s.Require().NoError(test.WaitForCondition(kubeconfigTestTimeout, func() bool {
			return watcher.close != nil
		}), "timeout waiting for watcher to be ready")

		err1 := watcher.Close()
		err2 := watcher.Close()

		s.NoError(err1, "first close should succeed")
		s.NoError(err2, "second close should succeed")
	})
}

func TestKubeconfig(t *testing.T) {
	suite.Run(t, new(KubeconfigTestSuite))
}
