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
	"github.com/containers/kubernetes-mcp-server/pkg/logging"
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

// TestSIGHUPInvokesLogSinkReload is the wiring smoke test for the
// SIGHUP-handler → sink.Reload call path. The Sink's own behavior is
// covered exhaustively in pkg/logging; this test exists only to ensure
// that if someone deletes m.logSink.Reload(newConfig) from
// setupSIGHUPHandler, at least one test fails. End-to-end behavior
// (rotation, stderr, etc.) is verified at the Sink layer.
//
// Unlike the other SIGHUPSuite tests, this one does not use InitServer:
// production order is logging.New (mutates klog) -> setupSIGHUPHandler
// (spawns goroutine), and reversing the order in tests would race against
// the goroutine's klog.V reads. Mirror production order here.
func TestSIGHUPInvokesLogSinkReload(t *testing.T) {
	klogState := klog.CaptureState()
	t.Cleanup(klogState.Restore)
	logBuffer := &test.SyncBuffer{}

	tempDir := t.TempDir()
	pathA := filepath.Join(tempDir, "a.log")
	pathB := filepath.Join(tempDir, "b.log")
	configPath := filepath.Join(tempDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(
		`log_file = "`+pathA+`"`+"\n"+`log_level = 1`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mockServer := test.NewMockServer()
	mockServer.Handle(test.NewDiscoveryClientHandler())
	t.Cleanup(mockServer.Close)

	cfg, err := config.Read(configPath, "")
	if err != nil {
		t.Fatal(err)
	}
	cfg.KubeConfig = mockServer.KubeconfigFile(t)

	// Install Sink BEFORE any goroutine that will read klog state
	// (kubernetes watchers spawned by mcp.NewServer, the SIGHUP handler).
	// Goroutine-creation is a happens-before edge for the race detector;
	// touching klog after a goroutine is spawned is what races. This
	// mirrors production order: cmd.Complete (sink) -> cmd.Run (server).
	sink, err := logging.New(cfg, logBuffer, logBuffer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	provider, err := kubernetes.NewProvider(cfg)
	if err != nil {
		t.Fatal(err)
	}
	mcpServer, err := mcp.NewServer(mcp.Configuration{StaticConfig: cfg, SDKLogger: sink.SDKLogger()}, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mcpServer.Close)

	opts := &MCPServerOptions{
		ConfigPath: configPath,
		IOStreams: genericiooptions.IOStreams{
			Out:    logBuffer,
			ErrOut: logBuffer,
		},
		logSink: sink,
	}
	cfgState := config.NewStaticConfigState(cfg)
	stop := opts.setupSIGHUPHandler(mcpServer, oauth.NewState(&oauth.Snapshot{}), cfgState)
	t.Cleanup(stop)

	if err := os.WriteFile(configPath, []byte(
		`log_file = "`+pathB+`"`+"\n"+`log_level = 1`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}

	// The handler emits "Configuration reloaded successfully via SIGHUP"
	// after sink.Reload returns. If the wiring is correct, that line lands
	// in pathB; if someone removed the Reload call, it would land in pathA.
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatalf("expected SIGHUP to invoke sink.Reload, redirecting logs to %s", pathB)
		case <-ticker.C:
			klog.Flush()
			content, err := os.ReadFile(pathB)
			if err == nil && strings.Contains(string(content), "Configuration reloaded successfully") {
				// Also pin the negative: a regression that wrote to both
				// the old and the new destinations would have passed the
				// success check above. Assert pathA did not receive the
				// post-reload line.
				if oldContent, _ := os.ReadFile(pathA); strings.Contains(string(oldContent), "Configuration reloaded successfully") {
					t.Fatalf("expected the post-reload line to land only in %s, but it also appeared in %s", pathB, pathA)
				}
				return
			}
		}
	}
}

func TestSIGHUP(t *testing.T) {
	suite.Run(t, new(SIGHUPSuite))
}
