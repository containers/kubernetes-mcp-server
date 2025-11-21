package mcp

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
)

type ConfigReloadSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
	configFile string
	configDir  string
	server     *Server
}

func (s *ConfigReloadSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
	s.mockServer.Handle(&test.DiscoveryClientHandler{})

	tempDir := s.T().TempDir()
	s.configFile = filepath.Join(tempDir, "config.toml")
	s.configDir = filepath.Join(tempDir, "config.d")
	err := os.Mkdir(s.configDir, 0755)
	s.Require().NoError(err)

	// Write initial config (include kubeconfig so reload works)
	err = os.WriteFile(s.configFile, []byte(`
log_level = 1
list_output = "table"
toolsets = ["core", "config"]
kubeconfig = "`+s.Cfg.KubeConfig+`"
`), 0644)
	s.Require().NoError(err)
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

func (s *ConfigReloadSuite) TestDropInConfigurationReload() {
	// Initialize server - it will load from config files
	cfg, err := config.Read(s.configFile, s.configDir)
	s.Require().NoError(err)
	server, err := NewServer(Configuration{
		StaticConfig: cfg,
		ConfigPath:   s.configFile,
		ConfigDir:    s.configDir,
	})
	s.Require().NoError(err)
	s.Require().NotNil(server)
	s.server = server

	s.Run("initial configuration loaded correctly", func() {
		s.Equal(1, server.configuration.LogLevel)
		s.Equal("table", server.configuration.StaticConfig.ListOutput)
		s.Equal([]string{"core", "config"}, server.configuration.StaticConfig.Toolsets)
	})

	// Add first drop-in file
	dropIn1 := filepath.Join(s.configDir, "10-override.toml")
	err = os.WriteFile(dropIn1, []byte(`
log_level = 5
list_output = "yaml"
`), 0644)
	s.Require().NoError(err)

	err = server.reloadConfiguration()
	s.Require().NoError(err)

	s.Run("drop-in file overrides main config", func() {
		s.Equal(5, server.configuration.LogLevel)
		s.Equal("yaml", server.configuration.StaticConfig.ListOutput)
		s.Equal([]string{"core", "config"}, server.configuration.StaticConfig.Toolsets)
	})

	// Add second drop-in file with different priority
	dropIn2 := filepath.Join(s.configDir, "20-toolsets.toml")
	err = os.WriteFile(dropIn2, []byte(`
toolsets = ["core", "config", "helm"]
`), 0644)
	s.Require().NoError(err)

	err = server.reloadConfiguration()
	s.Require().NoError(err)

	s.Run("multiple drop-ins with correct precedence", func() {
		s.Equal(5, server.configuration.LogLevel)
		s.Equal("yaml", server.configuration.StaticConfig.ListOutput)
		s.Equal([]string{"core", "config", "helm"}, server.configuration.StaticConfig.Toolsets)
	})

	// Add third drop-in that partially overrides
	dropIn3 := filepath.Join(s.configDir, "30-partial.toml")
	err = os.WriteFile(dropIn3, []byte(`
log_level = 7
`), 0644)
	s.Require().NoError(err)

	err = server.reloadConfiguration()
	s.Require().NoError(err)

	s.Run("later drop-in overrides earlier with partial config", func() {
		s.Equal(7, server.configuration.LogLevel)
		s.Equal("yaml", server.configuration.StaticConfig.ListOutput)
		s.Equal([]string{"core", "config", "helm"}, server.configuration.StaticConfig.Toolsets)
	})

	// Remove all drop-in files to test empty directory
	err = os.Remove(dropIn1)
	s.Require().NoError(err)
	err = os.Remove(dropIn2)
	s.Require().NoError(err)
	err = os.Remove(dropIn3)
	s.Require().NoError(err)

	err = server.reloadConfiguration()
	s.Require().NoError(err)

	s.Run("empty drop-in directory reverts to main config", func() {
		s.Equal(1, server.configuration.LogLevel)
		s.Equal("table", server.configuration.StaticConfig.ListOutput)
		s.Equal([]string{"core", "config"}, server.configuration.StaticConfig.Toolsets)
	})

	// Add a drop-in and then remove it
	tempDropIn := filepath.Join(s.configDir, "10-temp.toml")
	err = os.WriteFile(tempDropIn, []byte(`
log_level = 8
`), 0644)
	s.Require().NoError(err)

	err = server.reloadConfiguration()
	s.Require().NoError(err)
	s.Equal(8, server.configuration.LogLevel)

	err = os.Remove(tempDropIn)
	s.Require().NoError(err)

	err = server.reloadConfiguration()
	s.Require().NoError(err)

	s.Run("removing drop-in file reverts to main config", func() {
		s.Equal(1, server.configuration.LogLevel)
	})
}

func (s *ConfigReloadSuite) TestConfigurationReloadErrors() {
	server, err := NewServer(Configuration{
		StaticConfig: s.Cfg,
		ConfigPath:   s.configFile,
		ConfigDir:    s.configDir,
	})
	s.Require().NoError(err)
	s.server = server

	initialLogLevel := server.configuration.LogLevel

	s.Run("invalid TOML in drop-in file", func() {
		dropIn := filepath.Join(s.configDir, "10-invalid.toml")
		err = os.WriteFile(dropIn, []byte(`
log_level = "invalid
`), 0644)
		s.Require().NoError(err)

		err = server.reloadConfiguration()
		s.Error(err, "should return error for invalid TOML")
		s.Equal(initialLogLevel, server.configuration.LogLevel, "config unchanged on error")

		// Cleanup
		_ = os.Remove(dropIn)
	})

	s.Run("missing main config file", func() {
		// Delete main config file
		err = os.Remove(s.configFile)
		s.Require().NoError(err)

		err = server.reloadConfiguration()
		s.Error(err, "should return error for missing config file")
		s.Equal(initialLogLevel, server.configuration.LogLevel, "config unchanged on error")
	})
}

func (s *ConfigReloadSuite) TestSIGHUPReload() {
	server, err := NewServer(Configuration{
		StaticConfig: s.Cfg,
		ConfigPath:   s.configFile,
		ConfigDir:    s.configDir,
	})
	s.Require().NoError(err)
	s.server = server

	initialLogLevel := server.configuration.LogLevel

	s.Run("single SIGHUP triggers reload", func() {
		dropIn := filepath.Join(s.configDir, "10-sighup.toml")
		err = os.WriteFile(dropIn, []byte(`
log_level = 9
`), 0644)
		s.Require().NoError(err)

		// Send SIGHUP signal to the channel
		server.sigHupCh <- syscall.SIGHUP
		time.Sleep(100 * time.Millisecond)

		s.NotEqual(initialLogLevel, server.configuration.LogLevel)
		s.Equal(9, server.configuration.LogLevel)

		// Cleanup for next test
		_ = os.Remove(dropIn)
	})

	s.Run("multiple SIGHUP signals in succession", func() {
		dropIn := filepath.Join(s.configDir, "10-multi.toml")

		// First SIGHUP
		err = os.WriteFile(dropIn, []byte(`log_level = 3`), 0644)
		s.Require().NoError(err)
		server.sigHupCh <- syscall.SIGHUP
		time.Sleep(50 * time.Millisecond)
		s.Equal(3, server.configuration.LogLevel)

		// Second SIGHUP
		err = os.WriteFile(dropIn, []byte(`log_level = 6`), 0644)
		s.Require().NoError(err)
		server.sigHupCh <- syscall.SIGHUP
		time.Sleep(50 * time.Millisecond)
		s.Equal(6, server.configuration.LogLevel)

		// Third SIGHUP
		err = os.WriteFile(dropIn, []byte(`log_level = 9`), 0644)
		s.Require().NoError(err)
		server.sigHupCh <- syscall.SIGHUP
		time.Sleep(50 * time.Millisecond)
		s.Equal(9, server.configuration.LogLevel)
	})
}

func (s *ConfigReloadSuite) TestReloadUpdatesToolsets() {
	server, err := NewServer(Configuration{
		StaticConfig: s.Cfg,
		ConfigPath:   s.configFile,
		ConfigDir:    s.configDir,
	})
	s.Require().NoError(err)
	s.server = server

	// Get initial tools
	s.InitMcpClient()
	initialTools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
	s.Require().NoError(err)
	s.Require().Greater(len(initialTools.Tools), 0)

	// Add helm toolset via drop-in
	dropIn := filepath.Join(s.configDir, "10-add-helm.toml")
	err = os.WriteFile(dropIn, []byte(`
toolsets = ["core", "config", "helm"]
`), 0644)
	s.Require().NoError(err)

	// Reload configuration
	err = server.reloadConfiguration()
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
	server, err := NewServer(Configuration{
		StaticConfig: s.Cfg,
		ConfigPath:   s.configFile,
		ConfigDir:    s.configDir,
	})
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
