package mcp

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
)

type ConfigReloadSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
	server     *Server
}

func (s *ConfigReloadSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
}

func (s *ConfigReloadSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.server != nil {
		s.server.Close()
	}
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *ConfigReloadSuite) TestConfigurationReload() {
	// Initialize server with initial config
	provider, err := kubernetes.NewProvider(s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(Configuration{
		StaticConfig: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.Require().NotNil(server)
	s.server = server

	s.Run("initial configuration loaded correctly", func() {
		s.Equal(s.Cfg.LogLevel, server.configuration.LogLevel)
		s.Equal(s.Cfg.ListOutput, server.configuration.StaticConfig.ListOutput)
		s.Equal(s.Cfg.Toolsets, server.configuration.StaticConfig.Toolsets)
	})

	s.Run("reload with new log level", func() {
		newConfig := config.Default()
		newConfig.LogLevel = 5
		newConfig.ListOutput = "yaml"
		newConfig.Toolsets = []string{"core", "config"}
		newConfig.KubeConfig = s.Cfg.KubeConfig

		err = server.ReloadConfiguration(newConfig)
		s.Require().NoError(err)

		s.Equal(5, server.configuration.LogLevel)
		s.Equal("yaml", server.configuration.StaticConfig.ListOutput)
		s.Equal([]string{"core", "config"}, server.configuration.StaticConfig.Toolsets)
	})

	s.Run("reload with additional toolsets", func() {
		newConfig := config.Default()
		newConfig.LogLevel = 5
		newConfig.ListOutput = "yaml"
		newConfig.Toolsets = []string{"core", "config", "helm"}
		newConfig.KubeConfig = s.Cfg.KubeConfig

		err = server.ReloadConfiguration(newConfig)
		s.Require().NoError(err)

		s.Equal(5, server.configuration.LogLevel)
		s.Equal("yaml", server.configuration.StaticConfig.ListOutput)
		s.Equal([]string{"core", "config", "helm"}, server.configuration.StaticConfig.Toolsets)
	})

	s.Run("reload with partial changes", func() {
		newConfig := config.Default()
		newConfig.LogLevel = 7
		newConfig.ListOutput = "yaml"
		newConfig.Toolsets = []string{"core", "config", "helm"}
		newConfig.KubeConfig = s.Cfg.KubeConfig

		err = server.ReloadConfiguration(newConfig)
		s.Require().NoError(err)

		s.Equal(7, server.configuration.LogLevel)
		s.Equal("yaml", server.configuration.StaticConfig.ListOutput)
		s.Equal([]string{"core", "config", "helm"}, server.configuration.StaticConfig.Toolsets)
	})

	s.Run("reload back to defaults", func() {
		newConfig := config.Default()
		newConfig.LogLevel = 0
		newConfig.ListOutput = "table"
		newConfig.Toolsets = []string{"core", "config"}
		newConfig.KubeConfig = s.Cfg.KubeConfig

		err = server.ReloadConfiguration(newConfig)
		s.Require().NoError(err)

		s.Equal(0, server.configuration.LogLevel)
		s.Equal("table", server.configuration.StaticConfig.ListOutput)
		s.Equal([]string{"core", "config"}, server.configuration.StaticConfig.Toolsets)
	})
}

func (s *ConfigReloadSuite) TestConfigurationValues() {
	provider, err := kubernetes.NewProvider(s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(Configuration{
		StaticConfig: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.server = server

	s.Run("reload updates configuration values", func() {
		// Verify initial values
		initialLogLevel := server.configuration.LogLevel

		newConfig := config.Default()
		newConfig.LogLevel = 9
		newConfig.ListOutput = "yaml"
		newConfig.Toolsets = []string{"core", "config", "helm"}
		newConfig.KubeConfig = s.Cfg.KubeConfig

		err = server.ReloadConfiguration(newConfig)
		s.Require().NoError(err)

		// Verify configuration was updated
		s.NotEqual(initialLogLevel, server.configuration.LogLevel)
		s.Equal(9, server.configuration.LogLevel)
		s.Equal([]string{"core", "config", "helm"}, server.configuration.StaticConfig.Toolsets)
		s.Equal("yaml", server.configuration.StaticConfig.ListOutput)
	})
}

func (s *ConfigReloadSuite) TestMultipleReloads() {
	provider, err := kubernetes.NewProvider(s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(Configuration{
		StaticConfig: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.server = server

	s.Run("multiple reloads in succession", func() {
		// First reload
		cfg1 := config.Default()
		cfg1.LogLevel = 3
		cfg1.KubeConfig = s.Cfg.KubeConfig
		cfg1.Toolsets = []string{"core"}
		err = server.ReloadConfiguration(cfg1)
		s.Require().NoError(err)
		s.Equal(3, server.configuration.LogLevel)

		// Second reload
		cfg2 := config.Default()
		cfg2.LogLevel = 6
		cfg2.KubeConfig = s.Cfg.KubeConfig
		cfg2.Toolsets = []string{"core", "config"}
		err = server.ReloadConfiguration(cfg2)
		s.Require().NoError(err)
		s.Equal(6, server.configuration.LogLevel)

		// Third reload
		cfg3 := config.Default()
		cfg3.LogLevel = 9
		cfg3.KubeConfig = s.Cfg.KubeConfig
		cfg3.Toolsets = []string{"core", "config", "helm"}
		err = server.ReloadConfiguration(cfg3)
		s.Require().NoError(err)
		s.Equal(9, server.configuration.LogLevel)
	})
}

func (s *ConfigReloadSuite) TestReloadUpdatesToolsets() {
	provider, err := kubernetes.NewProvider(s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(Configuration{
		StaticConfig: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.server = server

	// Get initial tools
	s.InitMcpClient()
	initialTools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
	s.Require().NoError(err)
	s.Require().Greater(len(initialTools.Tools), 0)

	// Add helm toolset via reload
	newConfig := config.Default()
	newConfig.Toolsets = []string{"core", "config", "helm"}
	newConfig.KubeConfig = s.Cfg.KubeConfig

	// Reload configuration
	err = server.ReloadConfiguration(newConfig)
	s.Require().NoError(err)

	// Verify helm tools are available
	reloadedTools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
	s.Require().NoError(err)

	helmToolFound := false
	for _, tool := range reloadedTools.Tools {
		if tool.Name == "helm_list" {
			helmToolFound = true
			break
		}
	}
	s.True(helmToolFound, "helm tools should be available after reload")
}

func (s *ConfigReloadSuite) TestServerLifecycle() {
	provider, err := kubernetes.NewProvider(s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(Configuration{
		StaticConfig: s.Cfg,
	}, provider)
	s.Require().NoError(err)

	s.Run("server closes without panic", func() {
		s.NotPanics(func() {
			server.Close()
		})
	})

	s.Run("double close does not panic", func() {
		s.NotPanics(func() {
			server.Close()
		})
	})
}

func TestConfigReload(t *testing.T) {
	suite.Run(t, new(ConfigReloadSuite))
}
