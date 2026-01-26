//go:build !windows

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
)

// SIGHUPSuite tests the SIGHUP configuration reload behavior
type SIGHUPSuite struct {
	suite.Suite
	mockServer      *test.MockServer
	server          *mcp.Server
	tempDir         string
	dropInConfigDir string
	logBuffer       *bytes.Buffer
}

func (s *SIGHUPSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	s.tempDir = s.T().TempDir()
	s.dropInConfigDir = filepath.Join(s.tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(s.dropInConfigDir, 0755))

	// Set up klog to write to our buffer so we can verify log messages
	s.logBuffer = &bytes.Buffer{}
	logger := textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(2), textlogger.Output(s.logBuffer)))
	klog.SetLoggerWithOptions(logger)
}

func (s *SIGHUPSuite) TearDownTest() {
	if s.server != nil {
		s.server.Close()
	}
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *SIGHUPSuite) InitServer(configPath, configDir string) {
	cfg, err := config.Read(configPath, configDir)
	s.Require().NoError(err)
	cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())

	provider, err := kubernetes.NewProvider(cfg)
	s.Require().NoError(err)
	s.server, err = mcp.NewServer(mcp.Configuration{
		StaticConfig: cfg,
	}, provider)
	s.Require().NoError(err)
	// Set up SIGHUP handler
	opts := &MCPServerOptions{
		ConfigPath: configPath,
		ConfigDir:  configDir,
	}
	opts.setupSIGHUPHandler(s.server)
}

func (s *SIGHUPSuite) TestSIGHUPReloadsConfigFromFile() {
	// Create initial config file - start with only core toolset (no helm)
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		toolsets = ["core", "config"]
	`), 0644))
	s.InitServer(configPath, "")

	s.Run("helm tools are not initially available", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Modify the config file to add helm toolset
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		toolsets = ["core", "config", "helm"]
	`), 0644))

	// Send SIGHUP to current process
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("helm tools become available after SIGHUP", func() {
		s.Require().Eventually(func() bool {
			return slices.Contains(s.server.GetEnabledTools(), "helm_list")
		}, 2*time.Second, 50*time.Millisecond)
	})
}

func (s *SIGHUPSuite) TestSIGHUPReloadsFromDropInDirectory() {
	// Create initial config file - with helm enabled
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		toolsets = ["core", "config", "helm"]
	`), 0644))

	// Create initial drop-in file that removes helm
	dropInPath := filepath.Join(s.dropInConfigDir, "10-override.toml")
	s.Require().NoError(os.WriteFile(dropInPath, []byte(`
		toolsets = ["core", "config"]
	`), 0644))

	s.InitServer(configPath, "")

	s.Run("drop-in override removes helm from initial config", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Update drop-in file to add helm back
	s.Require().NoError(os.WriteFile(dropInPath, []byte(`
		toolsets = ["core", "config", "helm"]
	`), 0644))

	// Send SIGHUP
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("helm tools become available after updating drop-in and sending SIGHUP", func() {
		s.Require().Eventually(func() bool {
			return slices.Contains(s.server.GetEnabledTools(), "helm_list")
		}, 2*time.Second, 50*time.Millisecond)
	})
}

func (s *SIGHUPSuite) TestSIGHUPWithInvalidConfigContinues() {
	// Create initial config file - start with only core toolset (no helm)
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		toolsets = ["core", "config"]
	`), 0644))
	s.InitServer(configPath, "")

	s.Run("helm tools are not initially available", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Write invalid TOML to config file
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		toolsets = "not a valid array
	`), 0644))

	// Send SIGHUP - should not panic, should continue with old config
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("logs error when config is invalid", func() {
		s.Require().Eventually(func() bool {
			return strings.Contains(s.logBuffer.String(), "Failed to reload configuration")
		}, 2*time.Second, 50*time.Millisecond)
	})

	s.Run("tools remain unchanged after failed reload", func() {
		s.True(slices.Contains(s.server.GetEnabledTools(), "events_list"))
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Now fix the config and add helm
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		toolsets = ["core", "config", "helm"]
	`), 0644))

	// Send another SIGHUP
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("helm tools become available after fixing config and sending SIGHUP", func() {
		s.Require().Eventually(func() bool {
			return slices.Contains(s.server.GetEnabledTools(), "helm_list")
		}, 2*time.Second, 50*time.Millisecond)
	})
}

func (s *SIGHUPSuite) TestSIGHUPWithConfigDirOnly() {
	// Create initial drop-in file without helm
	dropInPath := filepath.Join(s.dropInConfigDir, "10-settings.toml")
	s.Require().NoError(os.WriteFile(dropInPath, []byte(`
		toolsets = ["core", "config"]
	`), 0644))

	s.InitServer("", s.dropInConfigDir)

	s.Run("helm tools are not initially available", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Update drop-in file to add helm
	s.Require().NoError(os.WriteFile(dropInPath, []byte(`
		toolsets = ["core", "config", "helm"]
	`), 0644))

	// Send SIGHUP
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("helm tools become available after SIGHUP with config-dir only", func() {
		s.Require().Eventually(func() bool {
			return slices.Contains(s.server.GetEnabledTools(), "helm_list")
		}, 2*time.Second, 50*time.Millisecond)
	})
}

func (s *SIGHUPSuite) TestSIGHUPReloadsPrompts() {
	// Create initial config with one prompt
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(`
        [[prompts]]
        name = "initial-prompt"
        description = "Initial prompt"

        [[prompts.messages]]
        role = "user"
        content = "Initial message"
    `), 0644))
	s.InitServer(configPath, "")

	enabledPrompts := s.server.GetEnabledPrompts()
	s.GreaterOrEqual(len(enabledPrompts), 1)
	s.Contains(enabledPrompts, "initial-prompt")

	// Update config with new prompt
	s.Require().NoError(os.WriteFile(configPath, []byte(`
        [[prompts]]
        name = "updated-prompt"
        description = "Updated prompt"

        [[prompts.messages]]
        role = "user"
        content = "Updated message"
    `), 0644))

	// Send SIGHUP
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	// Verify prompts were reloaded
	s.Require().Eventually(func() bool {
		enabledPrompts = s.server.GetEnabledPrompts()
		return len(enabledPrompts) >= 1 && slices.Contains(enabledPrompts, "updated-prompt") && !slices.Contains(enabledPrompts, "initial-prompt")
	}, 2*time.Second, 50*time.Millisecond)
}

func TestSIGHUP(t *testing.T) {
	suite.Run(t, new(SIGHUPSuite))
}
