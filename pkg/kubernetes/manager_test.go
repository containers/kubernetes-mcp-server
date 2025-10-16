package kubernetes

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ManagerTestSuite struct {
	suite.Suite
	originalEnv             []string
	originalInClusterConfig func() (*rest.Config, error)
	mockServer              *test.MockServer
}

func (s *ManagerTestSuite) SetupTest() {
	s.originalEnv = os.Environ()
	s.originalInClusterConfig = InClusterConfig
	s.mockServer = test.NewMockServer()
}

func (s *ManagerTestSuite) TearDownTest() {
	test.RestoreEnv(s.originalEnv)
	InClusterConfig = s.originalInClusterConfig
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *ManagerTestSuite) TestNewManagerInCluster() {
	InClusterConfig = func() (*rest.Config, error) {
		return &rest.Config{}, nil
	}
	s.Run("with default StaticConfig (empty kubeconfig)", func() {
		manager, err := NewManager(&config.StaticConfig{}, "")
		s.Require().NoError(err)
		s.Require().NotNil(manager)
		s.Run("behaves as in cluster", func() {
			s.True(manager.inCluster, "expected in cluster, got not in cluster")
		})
		s.Run("sets default user-agent", func() {
			s.Contains(manager.cfg.UserAgent, "("+runtime.GOOS+"/"+runtime.GOARCH+")")
		})
	})
	s.Run("with explicit kubeconfig", func() {
		manager, err := NewManager(&config.StaticConfig{
			KubeConfig: s.mockServer.KubeconfigFile(s.T()),
		}, "")
		s.Require().NoError(err)
		s.Require().NotNil(manager)
		s.Run("behaves as NOT in cluster", func() {
			s.False(manager.inCluster, "expected not in cluster, got in cluster")
		})
	})
}

func (s *ManagerTestSuite) TestNewManagerLocal() {
	InClusterConfig = func() (*rest.Config, error) {
		return nil, rest.ErrNotInCluster
	}
	s.Run("with valid kubeconfig in env", func() {
		kubeconfig := s.mockServer.KubeconfigFile(s.T())
		s.Require().NoError(os.Setenv("KUBECONFIG", kubeconfig))
		manager, err := NewManager(&config.StaticConfig{}, "")
		s.Require().NoError(err)
		s.Require().NotNil(manager)
		s.Run("behaves as NOT in cluster", func() {
			s.False(manager.inCluster, "expected not in cluster, got in cluster")
		})
		s.Run("loads correct config", func() {
			s.Contains(manager.clientCmdConfig.ConfigAccess().GetLoadingPrecedence(), kubeconfig, "expected kubeconfig path to match")
		})
		s.Run("sets default user-agent", func() {
			s.Contains(manager.cfg.UserAgent, "("+runtime.GOOS+"/"+runtime.GOARCH+")")
		})
		s.Run("rest config host points to mock server", func() {
			s.Equal(s.mockServer.Config().Host, manager.cfg.Host, "expected rest config host to match mock server")
		})
	})
	s.Run("with valid kubeconfig in env and explicit kubeconfig in config", func() {
		kubeconfigInEnv := s.mockServer.KubeconfigFile(s.T())
		s.Require().NoError(os.Setenv("KUBECONFIG", kubeconfigInEnv))
		kubeconfigExplicit := s.mockServer.KubeconfigFile(s.T())
		manager, err := NewManager(&config.StaticConfig{
			KubeConfig: kubeconfigExplicit,
		}, "")
		s.Require().NoError(err)
		s.Require().NotNil(manager)
		s.Run("behaves as NOT in cluster", func() {
			s.False(manager.inCluster, "expected not in cluster, got in cluster")
		})
		s.Run("loads correct config (explicit)", func() {
			s.NotContains(manager.clientCmdConfig.ConfigAccess().GetLoadingPrecedence(), kubeconfigInEnv, "expected kubeconfig path to NOT match env")
			s.Contains(manager.clientCmdConfig.ConfigAccess().GetLoadingPrecedence(), kubeconfigExplicit, "expected kubeconfig path to match explicit")
		})
		s.Run("rest config host points to mock server", func() {
			s.Equal(s.mockServer.Config().Host, manager.cfg.Host, "expected rest config host to match mock server")
		})
	})
	s.Run("with valid kubeconfig in env and explicit kubeconfig context (valid)", func() {
		kubeconfig := s.mockServer.Kubeconfig()
		kubeconfig.Contexts["not-the-mock-server"] = clientcmdapi.NewContext()
		kubeconfig.Contexts["not-the-mock-server"].Cluster = "not-the-mock-server"
		kubeconfig.Clusters["not-the-mock-server"] = clientcmdapi.NewCluster()
		kubeconfig.Clusters["not-the-mock-server"].Server = "https://not-the-mock-server:6443" // REST configuration should point to mock server, not this
		kubeconfig.CurrentContext = "not-the-mock-server"
		kubeconfigFile := test.KubeconfigFile(s.T(), kubeconfig)
		s.Require().NoError(os.Setenv("KUBECONFIG", kubeconfigFile))
		manager, err := NewManager(&config.StaticConfig{}, "fake-context") // fake-context is the one mock-server serves
		s.Require().NoError(err)
		s.Require().NotNil(manager)
		s.Run("behaves as NOT in cluster", func() {
			s.False(manager.inCluster, "expected not in cluster, got in cluster")
		})
		s.Run("loads correct config", func() {
			s.Contains(manager.clientCmdConfig.ConfigAccess().GetLoadingPrecedence(), kubeconfigFile, "expected kubeconfig path to match")
		})
		s.Run("rest config host points to mock server", func() {
			s.Equal(s.mockServer.Config().Host, manager.cfg.Host, "expected rest config host to match mock server")
		})
	})
	s.Run("with valid kubeconfig in env and explicit kubeconfig context (invalid)", func() {
		kubeconfigInEnv := s.mockServer.KubeconfigFile(s.T())
		s.Require().NoError(os.Setenv("KUBECONFIG", kubeconfigInEnv))
		manager, err := NewManager(&config.StaticConfig{}, "i-do-not-exist")
		s.Run("returns error", func() {
			s.Error(err)
			s.Nil(manager)
			s.ErrorContains(err, `failed to create kubernetes rest config: context "i-do-not-exist" does not exist`)
		})
	})
	s.Run("with invalid path kubeconfig in env", func() {
		s.Require().NoError(os.Setenv("KUBECONFIG", "i-dont-exist"))
		manager, err := NewManager(&config.StaticConfig{}, "")
		s.Run("returns error", func() {
			s.Error(err)
			s.Nil(manager)
			s.ErrorContains(err, "failed to create kubernetes rest config")
		})
	})
	s.Run("with empty kubeconfig in env", func() {
		kubeconfigPath := filepath.Join(s.T().TempDir(), "config")
		s.Require().NoError(os.WriteFile(kubeconfigPath, []byte(""), 0644))
		s.Require().NoError(os.Setenv("KUBECONFIG", kubeconfigPath))
		manager, err := NewManager(&config.StaticConfig{}, "")
		s.Run("returns error", func() {
			s.Error(err)
			s.Nil(manager)
			s.ErrorContains(err, "no configuration has been provided")
		})
	})
}

func TestManager(t *testing.T) {
	suite.Run(t, new(ManagerTestSuite))
}
