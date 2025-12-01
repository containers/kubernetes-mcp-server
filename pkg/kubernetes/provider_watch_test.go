package kubernetes

import (
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type ProviderWatchTargetsTestSuite struct {
	suite.Suite
	mockServer *test.MockServer
}

func (s *ProviderWatchTargetsTestSuite) SetupTest() {
	s.mockServer = test.NewMockServer()

	s.T().Setenv("CLUSTER_STATE_POLL_INTERVAL_MS", "100")
	s.T().Setenv("CLUSTER_STATE_DEBOUNCE_WINDOW_MS", "50")
}

func (s *ProviderWatchTargetsTestSuite) TearDownTest() {
	s.mockServer.Close()
}

// WaitForWatchTargets sets up a WatchTargets callback, executes the provided function, and waits for the callback to be invoked.
func (s *ProviderWatchTargetsTestSuite) WaitForWatchTargets(timeout time.Duration, provider Provider, fn func()) {
	callbackInvoked := make(chan struct{})
	var once sync.Once
	provider.WatchTargets(func() error {
		once.Do(func() {
			close(callbackInvoked)
		})
		return nil
	})
	fn()
	select {
	case <-callbackInvoked:
		// Callback was invoked
	case <-time.After(timeout):
		s.Fail("Timeout waiting for callback to be invoked")
	}
}

// WriteKubeconfig appends a newline to the kubeconfig file to trigger the file watcher.
func (s *ProviderWatchTargetsTestSuite) WriteKubeconfig(k *Kubernetes) {
	f, err := os.OpenFile(k.ToRawKubeConfigLoader().ConfigAccess().GetExplicitFile(), os.O_APPEND|os.O_WRONLY, 0644)
	s.Require().NoError(err, "Expected no error opening kubeconfig file")
	_, err = f.WriteString("\n")
	s.Require().NoError(err, "Expected no error writing to kubeconfig file")
	s.Require().NoError(f.Close(), "Expected no error closing kubeconfig file")
}

func (s *ProviderWatchTargetsTestSuite) TestKubeconfigCacheInvalidation() {
	s.mockServer.Handle(&test.DiscoveryClientHandler{})
	staticConfig := &config.StaticConfig{KubeConfig: test.KubeconfigFile(s.T(), s.mockServer.Kubeconfig())}

	testCases := []func() (Provider, error){
		func() (Provider, error) { return newKubeConfigClusterProvider(staticConfig) },
		func() (Provider, error) {
			return newSingleClusterProvider(config.ClusterProviderDisabled)(staticConfig)
		},
	}
	for _, tc := range testCases {
		provider, err := tc()
		s.Require().NoError(err, "Expected no error from provider creation")

		s.Run("With provider "+reflect.TypeOf(provider).String(), func() {
			k, err := provider.GetDerivedKubernetes(s.T().Context(), provider.GetDefaultTarget())
			s.Require().NoError(err, "Expected no error from GetDerivedKubernetes")

			s.Run("given a fresh cache", func() {
				_, err := k.AccessControlClientset().DiscoveryClient().ServerGroups()
				s.Require().NoError(err, "Expected no error from AccessControlClientset")
				s.Require().True(k.AccessControlClientset().DiscoveryClient().Fresh())
			})

			s.Run("invalidates caches (fresh==false) when kubeconfig is changed", func() {
				s.WaitForWatchTargets(5*time.Second, provider, func() {
					s.WriteKubeconfig(k)
				})
				s.Require().False(k.AccessControlClientset().DiscoveryClient().Fresh())
			})
		})
	}
}

func TestProviderWatchTargetsTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderWatchTargetsTestSuite))
}
