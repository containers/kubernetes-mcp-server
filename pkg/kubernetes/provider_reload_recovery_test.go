package kubernetes

import (
	"os"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ProviderReloadRecoveryTestSuite struct {
	suite.Suite
	mockServer     *test.MockServer
	kubeconfig     *clientcmdapi.Config
	kubeconfigPath string
	cfg            *config.Config
}

func (s *ProviderReloadRecoveryTestSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())

	s.T().Setenv("KUBECONFIG_DEBOUNCE_WINDOW_MS", "10")
	s.T().Setenv("CLUSTER_STATE_POLL_INTERVAL_MS", "600000")

	s.kubeconfig = s.mockServer.Kubeconfig()
	for _, name := range []string{"context-1", "context-2"} {
		s.kubeconfig.Contexts[name] = &clientcmdapi.Context{
			Cluster:  s.kubeconfig.Contexts[s.kubeconfig.CurrentContext].Cluster,
			AuthInfo: s.kubeconfig.Contexts[s.kubeconfig.CurrentContext].AuthInfo,
		}
	}
	s.kubeconfigPath = test.KubeconfigFile(s.T(), s.kubeconfig)

	cfg, err := config.ReadToml(s.T().Context(), nil)
	s.Require().NoError(err)
	cfg.KubeConfig.SetForTest(s.kubeconfigPath)
	s.cfg = cfg
}

func (s *ProviderReloadRecoveryTestSuite) TearDownTest() {
	s.mockServer.Close()
}

func (s *ProviderReloadRecoveryTestSuite) TestKubeconfigProviderReloadsAfterInvalidKubeconfig() {
	provider, err := newKubeConfigClusterProvider(s.T().Context(), s.cfg)
	s.Require().NoError(err)
	s.T().Cleanup(provider.Close)
	attempts := s.watchReloadAttempts(provider)

	s.Require().NoError(os.WriteFile(s.kubeconfigPath, []byte("invalid: ["), 0o600))
	s.Require().Error(s.nextReloadAttempt(attempts))
	s.Equal("fake-context", provider.GetDefaultTarget())
	s.requireProviderContext(provider, "fake-context", "fake-context")

	s.kubeconfig.CurrentContext = "context-1"
	s.Require().NoError(clientcmd.WriteToFile(*s.kubeconfig, s.kubeconfigPath))
	s.Require().NoError(s.nextSuccessfulReload(attempts))
	s.Equal("context-1", provider.GetDefaultTarget())
	s.requireProviderContext(provider, "context-1", "context-1")
}

func (s *ProviderReloadRecoveryTestSuite) TestSingleProviderReloadsAfterInvalidKubeconfig() {
	provider, err := newSingleClusterProvider(config.ClusterProviderDisabled)(s.T().Context(), s.cfg)
	s.Require().NoError(err)
	s.T().Cleanup(provider.Close)
	attempts := s.watchReloadAttempts(provider)

	s.Require().NoError(os.WriteFile(s.kubeconfigPath, []byte("invalid: ["), 0o600))
	s.Require().Error(s.nextReloadAttempt(attempts))
	s.requireProviderContext(provider, "", "fake-context")

	s.kubeconfig.CurrentContext = "context-2"
	s.Require().NoError(clientcmd.WriteToFile(*s.kubeconfig, s.kubeconfigPath))
	s.Require().NoError(s.nextSuccessfulReload(attempts))
	s.requireProviderContext(provider, "", "context-2")
}

func (s *ProviderReloadRecoveryTestSuite) requireProviderContext(provider Provider, target, expected string) {
	k8s, err := provider.GetDerivedKubernetes(s.T().Context(), target)
	s.Require().NoError(err)
	rawConfig, err := k8s.ToRawKubeConfigLoader().RawConfig()
	s.Require().NoError(err)
	s.Equal(expected, rawConfig.CurrentContext)
}

func (s *ProviderReloadRecoveryTestSuite) watchReloadAttempts(provider Provider) <-chan error {
	attempts := make(chan error, 16)
	provider.WatchTargets(s.T().Context(), NewMcpReloader(func(fn func() error) error {
		err := fn()
		attempts <- err
		return err
	}, nil))
	return attempts
}

func (s *ProviderReloadRecoveryTestSuite) nextReloadAttempt(attempts <-chan error) error {
	select {
	case err := <-attempts:
		return err
	case <-time.After(5 * time.Second):
		s.T().Fatal("timeout waiting for reload attempt")
		return nil
	}
}

func (s *ProviderReloadRecoveryTestSuite) nextSuccessfulReload(attempts <-chan error) error {
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case err := <-attempts:
			if err == nil {
				return nil
			}
		case <-timeout.C:
			s.T().Fatal("timeout waiting for successful reload")
			return nil
		}
	}
}

func TestProviderReloadRecovery(t *testing.T) {
	suite.Run(t, new(ProviderReloadRecoveryTestSuite))
}
