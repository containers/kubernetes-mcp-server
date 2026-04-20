package kubernetes

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ProviderKubeconfigTestSuite struct {
	BaseProviderSuite
	mockServer *test.MockServer
	provider   Provider
}

func (s *ProviderKubeconfigTestSuite) SetupTest() {
	// Kubeconfig provider is used when the multi-cluster feature is enabled with the kubeconfig strategy.
	// For this test suite we simulate a kubeconfig with multiple contexts.
	s.mockServer = test.NewMockServer()
	kubeconfig := s.mockServer.Kubeconfig()
	for i := 0; i < 10; i++ {
		// Add multiple fake contexts to force multi-cluster behavior
		kubeconfig.Contexts[fmt.Sprintf("context-%d", i)] = clientcmdapi.NewContext()
	}
	provider, err := NewProvider(&config.StaticConfig{KubeConfig: test.KubeconfigFile(s.T(), kubeconfig)})
	s.Require().NoError(err, "Expected no error creating provider with kubeconfig")
	s.provider = provider
}

func (s *ProviderKubeconfigTestSuite) TearDownTest() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *ProviderKubeconfigTestSuite) TestType() {
	s.IsType(&kubeConfigClusterProvider{}, s.provider)
}

func (s *ProviderKubeconfigTestSuite) TestWithNonOpenShiftCluster() {
	s.Run("IsOpenShift returns false", func() {
		inOpenShift := s.provider.IsOpenShift(s.T().Context())
		s.False(inOpenShift, "Expected InOpenShift to return false")
	})
}

func (s *ProviderKubeconfigTestSuite) TestWithOpenShiftCluster() {
	s.mockServer.Handle(test.NewInOpenShiftHandler())
	s.Run("IsOpenShift returns true", func() {
		inOpenShift := s.provider.IsOpenShift(s.T().Context())
		s.True(inOpenShift, "Expected InOpenShift to return true")
	})
}

func (s *ProviderKubeconfigTestSuite) TestGetTargets() {
	s.Run("GetTargets returns all contexts defined in kubeconfig", func() {
		targets, err := s.provider.GetTargets(s.T().Context())
		s.Require().NoError(err, "Expected no error from GetTargets")
		s.Len(targets, 11, "Expected 11 targets from GetTargets")
		s.Contains(targets, "fake-context", "Expected fake-context in targets from GetTargets")
		for i := 0; i < 10; i++ {
			s.Contains(targets, fmt.Sprintf("context-%d", i), "Expected context-%d in targets from GetTargets", i)
		}
	})
}

func (s *ProviderKubeconfigTestSuite) TestGetDerivedKubernetes() {
	s.Run("GetDerivedKubernetes returns Kubernetes for valid context", func() {
		k8s, err := s.provider.GetDerivedKubernetes(s.T().Context(), "fake-context")
		s.Require().NoError(err, "Expected no error from GetDerivedKubernetes with valid context")
		s.NotNil(k8s, "Expected Kubernetes from GetDerivedKubernetes with valid context")
	})
	s.Run("GetDerivedKubernetes returns Kubernetes for empty context (default)", func() {
		k8s, err := s.provider.GetDerivedKubernetes(s.T().Context(), "")
		s.Require().NoError(err, "Expected no error from GetDerivedKubernetes with empty context")
		s.NotNil(k8s, "Expected Kubernetes from GetDerivedKubernetes with empty context")
	})
	s.Run("GetDerivedKubernetes returns error for invalid context", func() {
		k8s, err := s.provider.GetDerivedKubernetes(s.T().Context(), "invalid-context")
		s.Require().Error(err, "Expected error from GetDerivedKubernetes with invalid context")
		s.ErrorContainsf(err, `context "invalid-context" does not exist`, "Expected context does not exist error, got: %v", err)
		s.Nil(k8s, "Expected no Kubernetes from GetDerivedKubernetes with invalid context")
	})
}

func (s *ProviderKubeconfigTestSuite) TestGetDefaultTarget() {
	s.Run("GetDefaultTarget returns current-context defined in kubeconfig", func() {
		s.Equal("fake-context", s.provider.GetDefaultTarget(), "Expected fake-context as default target")
	})
}

func (s *ProviderKubeconfigTestSuite) TestEmptyCurrentContext() {
	s.Run("with single context auto-selects it as default target", func() {
		kubeconfig := s.mockServer.Kubeconfig()
		kubeconfig.CurrentContext = ""
		provider, err := NewProvider(&config.StaticConfig{KubeConfig: test.KubeconfigFile(s.T(), kubeconfig)})
		s.Require().NoError(err, "Expected no error creating provider with empty current-context and single context")
		s.Equal("fake-context", provider.GetDefaultTarget(), "Expected auto-selected fake-context as default target")
	})
	s.Run("with multiple contexts returns error", func() {
		kubeconfig := s.mockServer.Kubeconfig()
		kubeconfig.CurrentContext = ""
		kubeconfig.Contexts["another-context"] = clientcmdapi.NewContext()
		_, err := NewProvider(&config.StaticConfig{KubeConfig: test.KubeconfigFile(s.T(), kubeconfig)})
		s.Require().Error(err, "Expected error creating provider with empty current-context and multiple contexts")
		s.ErrorContains(err, "current-context is not set")
		s.ErrorContains(err, "kubectl config use-context")
	})
}

func (s *ProviderKubeconfigTestSuite) TestGetTargetParameterName() {
	s.Equal("context", s.provider.GetTargetParameterName(), "Expected context as target parameter name")
}

func (s *ProviderKubeconfigTestSuite) TestConcurrentReads() {
	s.Run("all read-only methods can be called concurrently without racing", func() {
		const goroutines = 20
		var wg sync.WaitGroup
		start := make(chan struct{})

		ops := []func(){
			func() { _, _ = s.provider.GetTargets(context.Background()) },
			func() { _ = s.provider.GetDefaultTarget() },
			func() { _ = s.provider.IsMultiTarget() },
			func() { _ = s.provider.IsOpenShift(context.Background()) },
			func() { _, _ = s.provider.GetDerivedKubernetes(context.Background(), "fake-context") },
			func() { _ = s.provider.GetTargetParameterName() },
		}

		wg.Add(goroutines)
		for i := range goroutines {
			op := ops[i%len(ops)]
			go func() {
				defer wg.Done()
				<-start
				op()
			}()
		}
		close(start)
		wg.Wait()
	})
}

func (s *ProviderKubeconfigTestSuite) TestConcurrentLazyManagerInit() {
	s.Run("concurrent GetDerivedKubernetes for unitialized contexts does not race", func() {
		// Build a kubeconfig with several valid but unitialized contexts.
		// Calling GetDerivedKubernetes for them simultaneusly exercises the write-lock
		// upgrade path inside managerForContext.
		kubeconfig := s.mockServer.Kubeconfig()
		const extraContexts = 5
		for i := range extraContexts {
			ctx := clientcmdapi.NewContext()
			ctx.Cluster = "fake"
			ctx.AuthInfo = "fake"
			kubeconfig.Contexts[fmt.Sprintf("lazy-context-%d", i)] = ctx
		}

		provider, err := NewProvider(&config.StaticConfig{KubeConfig: test.KubeconfigFile(s.T(), kubeconfig)})
		s.Require().NoError(err, "Expected no error creating provider")

		const goroutines = 20
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(goroutines)
		for i := range goroutines {
			name := fmt.Sprintf("lazy-context-%d", i%extraContexts)
			go func() {
				defer wg.Done()
				<-start
				_, _ = provider.GetDerivedKubernetes(context.Background(), name)
			}()
		}
		close(start)
		wg.Wait()
	})
}

func (s *ProviderKubeconfigTestSuite) TestWatchTargetsWithConcurrentReaders() {
	s.Run("reset via WatchTargets does not race with concurrent reads", func() {
		s.T().Setenv("KUBECONFIG_DEBOUNCE_WINDOW_MS", "10")
		s.T().Setenv("CLUSTER_STATE_POLL_INTERVAL_MS", "50")
		s.T().Setenv("CLUSTER_STATE_DEBOUNCE_WINDOW_MS", "10")

		kubeconfig := s.mockServer.Kubeconfig()
		const extraContexts = 5
		for i := range extraContexts {
			ctx := clientcmdapi.NewContext()
			ctx.Cluster = "fake"
			ctx.AuthInfo = "fake"
			kubeconfig.Contexts[fmt.Sprintf("lazy-context-%d", i)] = ctx
		}

		kubeconfigPath := test.KubeconfigFile(s.T(), kubeconfig)
		provider, err := NewProvider(&config.StaticConfig{KubeConfig: kubeconfigPath})
		s.Require().NoError(err, "Expected no error creating provider")
		s.T().Cleanup(provider.Close)

		callback, waitForCallback := CallbackWaiter()
		provider.WatchTargets(callback)

		const readers = 10
		stop := make(chan struct{})
		var readerWg sync.WaitGroup
		readerWg.Add(readers)

		for range readers {
			go func() {
				defer readerWg.Done()
				for {
					select {
					case <-stop:
						return
					default:
						_, _ = provider.GetTargets(context.Background())
						_ = provider.GetDefaultTarget()
						_ = provider.IsMultiTarget()
						_ = provider.IsOpenShift(context.Background())
						_, _ = provider.GetDerivedKubernetes(context.Background(), "fake-context")
					}
				}
			}()
		}

		for i := range extraContexts {
			kubeconfig.CurrentContext = fmt.Sprintf("lazy-context-%d", i)
			s.Require().NoError(clientcmd.WriteToFile(*kubeconfig, kubeconfigPath))
			s.Require().NoError(waitForCallback(5 * time.Second))
		}

		close(stop)
		readerWg.Wait()
	})
}

func TestProviderKubeconfig(t *testing.T) {
	suite.Run(t, new(ProviderKubeconfigTestSuite))
}
