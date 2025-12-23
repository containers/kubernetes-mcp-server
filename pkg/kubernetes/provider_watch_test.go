package kubernetes

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ProviderWatchTargetsTestSuite struct {
	suite.Suite
	mockServer             *test.MockServer
	discoveryClientHandler *test.DiscoveryClientHandler
	kubeconfig             *clientcmdapi.Config
	staticConfig           *config.StaticConfig
}

func (s *ProviderWatchTargetsTestSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
	s.discoveryClientHandler = test.NewDiscoveryClientHandler()
	s.mockServer.Handle(s.discoveryClientHandler)

	s.T().Setenv("CLUSTER_STATE_POLL_INTERVAL_MS", "100")
	s.T().Setenv("CLUSTER_STATE_DEBOUNCE_WINDOW_MS", "50")
	s.T().Setenv("KUBECONFIG_DEBOUNCE_WINDOW_MS", "50")

	// Add multiple fake contexts to allow testing of context changes
	s.kubeconfig = s.mockServer.Kubeconfig()
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("context-%d", i)
		s.kubeconfig.Contexts[name] = clientcmdapi.NewContext()
		s.kubeconfig.Contexts[name].Cluster = s.kubeconfig.Contexts[s.kubeconfig.CurrentContext].Cluster
		s.kubeconfig.Contexts[name].AuthInfo = s.kubeconfig.Contexts[s.kubeconfig.CurrentContext].AuthInfo
	}

	s.staticConfig = &config.StaticConfig{KubeConfig: test.KubeconfigFile(s.T(), s.kubeconfig)}
}

func (s *ProviderWatchTargetsTestSuite) TearDownTest() {
	s.mockServer.Close()
}

func (s *ProviderWatchTargetsTestSuite) TestClusterStateChanges() {
	testCases := []func() (Provider, error){
		func() (Provider, error) { return newKubeConfigClusterProvider(s.staticConfig) },
		func() (Provider, error) {
			return newSingleClusterProvider(api.ClusterProviderDisabled)(s.staticConfig)
		},
	}
	for _, tc := range testCases {
		provider, err := tc()
		s.Require().NoError(err, "Expected no error from provider creation")

		s.Run("With provider "+reflect.TypeOf(provider).String(), func() {
			callback, waitForCallback := CallbackWaiter()
			provider.WatchTargets(callback)
			s.Run("Reloads provider on cluster changes", func() {
				s.discoveryClientHandler.AddAPIResourceList(metav1.APIResourceList{GroupVersion: "alex.example.com/v1"})

				s.Require().NoError(waitForCallback(5 * time.Second))
				// Provider-wise the watcher.ClusterState which triggers the callback has no effect.
				// We might consider removing it at some point? (20251202)
			})
		})
	}
}

func (s *ProviderWatchTargetsTestSuite) TestKubeConfigClusterProvider() {
	provider, err := newKubeConfigClusterProvider(s.staticConfig)
	s.Require().NoError(err, "Expected no error from provider creation")

	callback, waitForCallback := CallbackWaiter()
	provider.WatchTargets(callback)

	s.Run("KubeConfigClusterProvider updates targets (reset) on kubeconfig change", func() {
		s.kubeconfig.CurrentContext = "context-1"
		s.Require().NoError(clientcmd.WriteToFile(*s.kubeconfig, s.staticConfig.KubeConfig))
		s.Require().NoError(waitForCallback(5 * time.Second))

		s.Run("Replaces default target with new context", func() {
			s.Equal("context-1", provider.GetDefaultTarget(), "Expected default target context to be updated")
		})
		s.Run("Adds new context to targets", func() {
			targets, err := provider.GetTargets(s.T().Context())
			s.Require().NoError(err, "Expected no error from GetTargets")
			s.Contains(targets, "context-1")
		})
		s.Run("Has derived Kubernetes for new context", func() {
			k, err := provider.GetDerivedKubernetes(s.T().Context(), "context-1")
			s.Require().NoError(err, "Expected no error from GetDerivedKubernetes for context-1")
			s.NotNil(k, "Expected Kubernetes from GetDerivedKubernetes for context-1")
			s.Run("Derived Kubernetes points to correct context", func() {
				cfg, err := k.ToRawKubeConfigLoader().RawConfig()
				s.Require().NoError(err, "Expected no error from ToRawKubeConfigLoader")
				s.Equal("context-1", cfg.CurrentContext, "Expected Kubernetes to point to changed-context")
			})
		})

		s.Run("Keeps watching for further changes", func() {
			s.kubeconfig.CurrentContext = "context-2"
			s.Require().NoError(clientcmd.WriteToFile(*s.kubeconfig, s.staticConfig.KubeConfig))
			s.Require().NoError(waitForCallback(5 * time.Second))

			s.Run("Replaces default target with new context", func() {
				s.Equal("context-2", provider.GetDefaultTarget(), "Expected default target context to be updated")
			})
		})
	})
}

func (s *ProviderWatchTargetsTestSuite) TestSingleClusterProvider() {
	provider, err := newSingleClusterProvider(api.ClusterProviderDisabled)(s.staticConfig)
	s.Require().NoError(err, "Expected no error from provider creation")

	callback, waitForCallback := CallbackWaiter()
	provider.WatchTargets(callback)

	s.Run("SingleClusterProvider reloads/resets on kubeconfig change", func() {
		s.kubeconfig.CurrentContext = "context-1"
		s.Require().NoError(clientcmd.WriteToFile(*s.kubeconfig, s.staticConfig.KubeConfig))
		s.Require().NoError(waitForCallback(5 * time.Second))

		s.Run("Derived Kubernetes points to updated context", func() {
			k, err := provider.GetDerivedKubernetes(s.T().Context(), "")
			s.Require().NoError(err, "Expected no error from GetDerivedKubernetes for context-1")
			s.NotNil(k, "Expected Kubernetes from GetDerivedKubernetes for context-1")
			s.Run("Derived Kubernetes points to correct context", func() {
				cfg, err := k.ToRawKubeConfigLoader().RawConfig()
				s.Require().NoError(err, "Expected no error from ToRawKubeConfigLoader")
				s.Equal("context-1", cfg.CurrentContext, "Expected Kubernetes to point to changed-context")
			})
		})

		s.Run("Keeps watching for further changes", func() {
			s.kubeconfig.CurrentContext = "context-2"
			s.Require().NoError(clientcmd.WriteToFile(*s.kubeconfig, s.staticConfig.KubeConfig))
			s.Require().NoError(waitForCallback(5 * time.Second))

			s.Run("Derived Kubernetes points to updated context", func() {
				k, err := provider.GetDerivedKubernetes(s.T().Context(), "")
				s.Require().NoError(err, "Expected no error from GetDerivedKubernetes for context-2")
				s.NotNil(k, "Expected Kubernetes from GetDerivedKubernetes for context-2")
				cfg, err := k.ToRawKubeConfigLoader().RawConfig()
				s.Require().NoError(err, "Expected no error from ToRawKubeConfigLoader")
				s.Equal("context-2", cfg.CurrentContext, "Expected Kubernetes to point to changed-context")
			})
		})
	})
}

// CallbackWaiter returns a callback and wait function that can be used multiple times.
func CallbackWaiter() (callback func() error, waitFunc func(timeout time.Duration) error) {
	signal := make(chan struct{}, 1)
	callback = func() error {
		select {
		case signal <- struct{}{}:
		default:
		}
		return nil
	}
	waitFunc = func(timeout time.Duration) error {
		select {
		case <-signal:
		case <-time.After(timeout):
			return errors.New("timeout waiting for callback")
		}
		return nil
	}
	return
}

func TestProviderWatchTargetsTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderWatchTargetsTestSuite))
}
