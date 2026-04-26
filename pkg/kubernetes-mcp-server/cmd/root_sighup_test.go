//go:build !windows

package cmd

import (
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
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/stretchr/testify/suite"
	"k8s.io/cli-runtime/pkg/genericiooptions"
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
	logBuffer       *test.SyncBuffer
	klogState       klog.State
	stopSIGHUP      func()
}

func (s *SIGHUPSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	s.tempDir = s.T().TempDir()
	s.dropInConfigDir = filepath.Join(s.tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(s.dropInConfigDir, 0o755))

	// Capture klog state so we can restore it after the test
	s.klogState = klog.CaptureState()

	// Set up klog to write to our buffer so we can verify log messages
	s.logBuffer = &test.SyncBuffer{}
	logger := textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(2), textlogger.Output(s.logBuffer)))
	klog.SetLoggerWithOptions(logger)
}

func (s *SIGHUPSuite) TearDownTest() {
	// Stop the SIGHUP handler goroutine before restoring klog
	if s.stopSIGHUP != nil {
		s.stopSIGHUP()
	}
	if s.server != nil {
		s.server.Close()
	}
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	s.klogState.Restore()
}

func (s *SIGHUPSuite) InitServer(configPath, configDir string) *MCPServerOptions {
	cfg, err := config.Read(configPath, configDir)
	s.Require().NoError(err)
	cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())

	provider, err := kubernetes.NewProvider(cfg)
	s.Require().NoError(err)
	s.server, err = mcp.NewServer(mcp.Configuration{
		StaticConfig: cfg,
	}, provider)
	s.Require().NoError(err)

	opts := &MCPServerOptions{
		ConfigPath: configPath,
		ConfigDir:  configDir,
		IOStreams: genericiooptions.IOStreams{
			Out:    s.logBuffer,
			ErrOut: s.logBuffer,
		},
	}
	oauthState := oauth.NewState(&oauth.Snapshot{})

	cfgState := config.NewStaticConfigState(cfg)
	s.stopSIGHUP = opts.setupSIGHUPHandler(s.server, oauthState, cfgState)
	s.T().Cleanup(func() {
		if opts.logFileHandle != nil {
			_ = opts.logFileHandle.Close()
		}
	})
	return opts
}

func (s *SIGHUPSuite) TestSIGHUPReloadsConfigFromFile() {
	// Create initial config file - start with only core toolset (no helm)
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		toolsets = ["core", "config"]
	`), 0o644))
	_ = s.InitServer(configPath, "")

	s.Run("helm tools are not initially available", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Modify the config file to add helm toolset
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		toolsets = ["core", "config", "helm"]
	`), 0o644))

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
	`), 0o644))

	// Create initial drop-in file that removes helm
	dropInPath := filepath.Join(s.dropInConfigDir, "10-override.toml")
	s.Require().NoError(os.WriteFile(dropInPath, []byte(`
		toolsets = ["core", "config"]
	`), 0o644))

	_ = s.InitServer(configPath, "")

	s.Run("drop-in override removes helm from initial config", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Update drop-in file to add helm back
	s.Require().NoError(os.WriteFile(dropInPath, []byte(`
		toolsets = ["core", "config", "helm"]
	`), 0o644))

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
	`), 0o644))
	_ = s.InitServer(configPath, "")

	s.Run("helm tools are not initially available", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Write invalid TOML to config file
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		toolsets = "not a valid array
	`), 0o644))

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
	`), 0o644))

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
	`), 0o644))

	_ = s.InitServer("", s.dropInConfigDir)

	s.Run("helm tools are not initially available", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Update drop-in file to add helm
	s.Require().NoError(os.WriteFile(dropInPath, []byte(`
		toolsets = ["core", "config", "helm"]
	`), 0o644))

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
    `), 0o644))
	_ = s.InitServer(configPath, "")

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
    `), 0o644))

	// Send SIGHUP
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	// Verify prompts were reloaded
	s.Require().Eventually(func() bool {
		enabledPrompts = s.server.GetEnabledPrompts()
		return len(enabledPrompts) >= 1 && slices.Contains(enabledPrompts, "updated-prompt") && !slices.Contains(enabledPrompts, "initial-prompt")
	}, 2*time.Second, 50*time.Millisecond)
}

func (s *SIGHUPSuite) TestSIGHUPRedirectsLogsToNewFile() {
	// Start with log_file pointing to file A
	logFileA := filepath.Join(s.tempDir, "a.log")
	logFileB := filepath.Join(s.tempDir, "b.log")
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		log_file = "`+logFileA+`"
	`), 0o644))

	opts := s.InitServer(configPath, "")

	s.Run("initial log file handle is nil before initializeLogging", func() {
		// InitServer does NOT call initializeLogging, so logFileHandle
		// is nil initially.
		s.Nil(opts.logFileHandle)
	})

	// Update config to redirect logs to file B
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		log_file = "`+logFileB+`"
	`), 0o644))

	// Send SIGHUP
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("log file handle points to new file after SIGHUP", func() {
		s.Require().Eventually(func() bool {
			return opts.logFileHandle != nil && opts.logFileHandle.Name() == logFileB
		}, 2*time.Second, 50*time.Millisecond)
	})

	s.Run("new log file exists", func() {
		_, err := os.Stat(logFileB)
		s.NoError(err)
	})
}

func (s *SIGHUPSuite) TestSIGHUPReopensLogFileAfterRotation() {
	// Simulate the real log rotation sequence:
	//   1. Server writes to server.log (inode A)
	//   2. logrotate renames server.log -> server.log.1 (inode A)
	//   3. SIGHUP -> reloadLogFile reopens server.log (creates inode B)
	//   4. Subsequent writes land in inode B, not the old inode A
	logFile := filepath.Join(s.tempDir, "server.log")
	rotatedFile := logFile + ".1"
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		log_file = "`+logFile+`"
	`), 0o644))

	opts := s.InitServer(configPath, "")

	// Simulate initializeLogging having opened the file
	initialHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	s.Require().NoError(err)
	opts.logFileHandle = initialHandle

	// Step 2: logrotate renames the file
	s.Require().NoError(os.Rename(logFile, rotatedFile))

	// Step 3: send SIGHUP - reloadLogFile should create a new file at the original path
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("reopen creates a new file at the original path with a different inode", func() {
		// Wait for the new file to appear on disk (proves the reopen happened).
		s.Require().Eventually(func() bool {
			_, err := os.Stat(logFile)
			return err == nil
		}, 2*time.Second, 50*time.Millisecond)

		newInfo, err := os.Stat(logFile)
		s.Require().NoError(err)
		rotatedInfo, err := os.Stat(rotatedFile)
		s.Require().NoError(err)
		s.False(os.SameFile(newInfo, rotatedInfo), "new log file should be a different inode than the rotated file")
	})
}

func (s *SIGHUPSuite) TestSIGHUPKeepsOldLogOnInvalidPath() {
	// Start with a valid log file
	logFileA := filepath.Join(s.tempDir, "valid.log")
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		log_file = "`+logFileA+`"
	`), 0o644))

	opts := s.InitServer(configPath, "")

	// Manually set up a log file handle to simulate the initial state
	initialHandle, err := os.OpenFile(logFileA, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	s.Require().NoError(err)
	opts.logFileHandle = initialHandle

	// Update config to point to an invalid path (directory that doesn't exist)
	invalidPath := filepath.Join(s.tempDir, "missing", "server.log")
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		log_file = "`+invalidPath+`"
	`), 0o644))

	// Send SIGHUP
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("logs error about failed reopen", func() {
		s.Require().Eventually(func() bool {
			return strings.Contains(s.logBuffer.String(), "Failed to reopen log file")
		}, 2*time.Second, 50*time.Millisecond)
	})

	s.Run("old log file handle is preserved", func() {
		s.Equal(logFileA, opts.logFileHandle.Name())
	})
}

func (s *SIGHUPSuite) TestSIGHUPLogsWSwitchesToStderr() {
	logFileA := filepath.Join(s.tempDir, "file.log")
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		log_file = "`+logFileA+`"
		log_level = 1
	`), 0o644))

	opts := s.InitServer(configPath, "")

	// Simulate initializeLogging having opened the file
	initialHandle, err := os.OpenFile(logFileA, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	s.Require().NoError(err)
	opts.logFileHandle = initialHandle

	// Switch to stderr via config update + SIGHUP
	s.Require().NoError(os.WriteFile(configPath, []byte(`
		log_file = "stderr"
		log_level = 1
	`), 0o644))
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("log file handle is cleared after switching to stderr", func() {
		s.Require().Eventually(func() bool {
			return opts.logFileHandle == nil
		}, 2*time.Second, 50*time.Millisecond)
	})

	s.Run("old file handle is closed", func() {
		_, writeErr := initialHandle.Write([]byte("test"))
		s.Error(writeErr)
	})
}

func TestSIGHUP(t *testing.T) {
	suite.Run(t, new(SIGHUPSuite))
}
