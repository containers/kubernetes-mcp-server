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
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	// For this test suite we back the kubeconfig context with a mock API server.
	s.mockServer = test.NewMockServer()
	// Default discovery simulates a vanilla (non-OpenShift) cluster.
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	provider, err := NewProvider(s.T().Context(), &config.StaticConfig{KubeConfig: s.mockServer.KubeconfigFile(s.T())})
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

func (s *ProviderKubeconfigTestSuite) TestWithOpenShiftCluster() {
	// Serve the OpenShift discovery document so the Project GVK is present.
	s.mockServer.ResetHandlers()
	s.mockServer.Handle(test.NewInOpenShiftHandler())
	s.Run("has OpenShift Project GVK", func() {
		hasProjects := s.provider.AnyTargetHasGVKs(s.T().Context(), []schema.GroupVersionKind{
			{Group: "project.openshift.io", Version: "v1", Kind: "Project"},
		})
		s.True(hasProjects, "Expected provider to report OpenShift Project GVK available")
	})
}

func (s *ProviderKubeconfigTestSuite) TestWithNonOpenShiftGVK() {
	s.Run("does not have non-existent GVK", func() {
		// Default (non-OpenShift) discovery returns a 404 for the missing GroupVersion.
		hasGVK := s.provider.AnyTargetHasGVKs(s.T().Context(), []schema.GroupVersionKind{
			{Group: "nonexistent.example.com", Version: "v1", Kind: "Foo"},
		})
		s.False(hasGVK, "Expected provider to report no nonexistent GVK")
	})
}

func (s *ProviderKubeconfigTestSuite) TestGetTargets() {
	s.Run("GetTargets returns all contexts defined in kubeconfig", func() {
		targets, err := s.provider.GetTargets(s.T().Context())
		s.Require().NoError(err, "Expected no error from GetTargets")
		s.NotEmpty(targets, "Expected at least one target from GetTargets")
		s.Contains(targets, "fake-context", "Expected fake-context in targets from GetTargets")
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
		kubeconfig := test.KubeConfigFake()
		kubeconfig.CurrentContext = ""
		provider, err := NewProvider(s.T().Context(), &config.StaticConfig{KubeConfig: test.KubeconfigFile(s.T(), kubeconfig)})
		s.Require().NoError(err, "Expected no error creating provider with empty current-context and single context")
		s.Equal("fake-context", provider.GetDefaultTarget(), "Expected auto-selected fake-context as default target")
	})
	s.Run("with multiple contexts returns error", func() {
		kubeconfig := test.KubeConfigFake()
		kubeconfig.CurrentContext = ""
		kubeconfig.Contexts["another-context"] = clientcmdapi.NewContext()
		_, err := NewProvider(s.T().Context(), &config.StaticConfig{KubeConfig: test.KubeconfigFile(s.T(), kubeconfig)})
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
			func() {
				_ = s.provider.AnyTargetHasGVKs(s.T().Context(), []schema.GroupVersionKind{
					{Group: "project.openshift.io", Version: "v1", Kind: "Project"},
				})
			},
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
		kubeconfig := test.KubeConfigFake()
		const extraContexts = 5
		for i := range extraContexts {
			ctx := clientcmdapi.NewContext()
			ctx.Cluster = "fake"
			ctx.AuthInfo = "fake"
			kubeconfig.Contexts[fmt.Sprintf("lazy-context-%d", i)] = ctx
		}

		provider, err := NewProvider(s.T().Context(), &config.StaticConfig{KubeConfig: test.KubeconfigFile(s.T(), kubeconfig)})
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

		kubeconfig := test.KubeConfigFake()
		const extraContexts = 5
		for i := range extraContexts {
			ctx := clientcmdapi.NewContext()
			ctx.Cluster = "fake"
			ctx.AuthInfo = "fake"
			kubeconfig.Contexts[fmt.Sprintf("lazy-context-%d", i)] = ctx
		}

		kubeconfigPath := test.KubeconfigFile(s.T(), kubeconfig)
		provider, err := NewProvider(s.T().Context(), &config.StaticConfig{KubeConfig: kubeconfigPath})
		s.Require().NoError(err, "Expected no error creating provider")
		s.T().Cleanup(provider.Close)

		callback, waitForCallback := CallbackWaiter()
		provider.WatchTargets(s.T().Context(), callback)

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
						_ = provider.AnyTargetHasGVKs(s.T().Context(), []schema.GroupVersionKind{
							{Group: "project.openshift.io", Version: "v1", Kind: "Project"},
						})
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
