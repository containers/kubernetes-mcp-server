//go:build !windows

package cmd

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/logging"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/errgroup"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
)

// baseSIGHUPSetup contains common setup for SIGHUP tests
type baseSIGHUPSetup struct {
	mockServer     *test.MockServer
	kubeconfigPath string
	tempDir        string
	logBuffer      *test.SyncBuffer
	klogState      klog.State
}

// setupSIGHUPTest performs common SIGHUP test setup
func setupSIGHUPTest(t *testing.T) *baseSIGHUPSetup {
	s := &baseSIGHUPSetup{}
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	s.kubeconfigPath = s.mockServer.KubeconfigFile(t)
	s.tempDir = t.TempDir()

	// Capture klog state
	s.klogState = klog.CaptureState()

	// Set up klog to write to buffer
	s.logBuffer = &test.SyncBuffer{}
	logger := textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(2), textlogger.Output(s.logBuffer)))
	klog.SetLoggerWithOptions(logger)

	return s
}

func (s *baseSIGHUPSetup) teardown() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	s.klogState.Restore()
}

func (s *baseSIGHUPSetup) withKube(body string) string {
	return fmt.Sprintf("kubeconfig = %q\n%s", s.kubeconfigPath, body)
}

// SIGHUPSuite tests the SIGHUP configuration reload behavior for STDIO mode
type SIGHUPSuite struct {
	suite.Suite
	*baseSIGHUPSetup
	dropInConfigDir string
	server          *mcp.Server
	stopSIGHUP      func()
	cfgState        *config.ConfigState
	oauthState      *oauth.State
	reloadExited    atomic.Bool
	reloadExitCode  atomic.Int32
}

func (s *SIGHUPSuite) SetupTest() {
	s.baseSIGHUPSetup = setupSIGHUPTest(s.T())
	s.dropInConfigDir = filepath.Join(s.tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(s.dropInConfigDir, 0o755))
	s.reloadExited.Store(false)
	s.reloadExitCode.Store(0)
}

func (s *SIGHUPSuite) TearDownTest() {
	// Stop the SIGHUP handler goroutine before restoring klog
	if s.stopSIGHUP != nil {
		s.stopSIGHUP()
	}
	if s.server != nil {
		s.server.Close()
	}
	s.teardown()
}

func (s *SIGHUPSuite) InitServer(configPath, configDir string) *MCPServerOptions {
	cfg, err := config.Read(s.T().Context(), configPath, configDir)
	s.Require().NoError(err)

	provider, err := kubernetes.NewProvider(s.T().Context(), cfg)
	s.Require().NoError(err)
	s.server, err = mcp.NewServer(s.T().Context(), mcp.Configuration{
		Config: cfg,
	}, provider)
	s.Require().NoError(err)

	opts := &MCPServerOptions{
		ConfigPath: configPath,
		ConfigDir:  configDir,
		Config:     cfg,
		IOStreams: genericiooptions.IOStreams{
			Out:    s.logBuffer,
			ErrOut: s.logBuffer,
		},
		exit: func(code int) {
			s.reloadExitCode.Store(int32(code))
			s.reloadExited.Store(true)
		},
	}
	s.oauthState = oauth.NewState(&oauth.Snapshot{})

	s.cfgState = config.NewConfigState(cfg)
	s.stopSIGHUP = opts.setupSIGHUPHandler(s.T().Context(), s.server, s.oauthState, s.cfgState)
	return opts
}

func (s *SIGHUPSuite) TestSIGHUPReloadsConfigFromFile() {
	// Create initial config file - start with only core toolset (no helm)
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		toolsets = ["core", "config"]
	`)), 0o644))
	_ = s.InitServer(configPath, "")

	s.Run("helm tools are not initially available", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Modify the config file to add helm toolset
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		toolsets = ["core", "config", "helm"]
	`)), 0o644))

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
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		toolsets = ["core", "config", "helm"]
	`)), 0o644))

	// Create initial drop-in file that removes helm
	dropInPath := filepath.Join(s.dropInConfigDir, "10-override.toml")
	s.Require().NoError(os.WriteFile(dropInPath, []byte(s.withKube(`
		toolsets = ["core", "config"]
	`)), 0o644))

	_ = s.InitServer(configPath, s.dropInConfigDir)

	s.Run("drop-in override removes helm from initial config", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Update drop-in file to add helm back
	s.Require().NoError(os.WriteFile(dropInPath, []byte(s.withKube(`
		toolsets = ["core", "config", "helm"]
	`)), 0o644))

	// Send SIGHUP
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("helm tools become available after updating drop-in and sending SIGHUP", func() {
		s.Require().Eventually(func() bool {
			return slices.Contains(s.server.GetEnabledTools(), "helm_list")
		}, 2*time.Second, 50*time.Millisecond)
	})
}

func (s *SIGHUPSuite) TestSIGHUPWithInvalidConfigExits() {
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		toolsets = ["core", "config"]
	`)), 0o644))
	_ = s.InitServer(configPath, "")

	s.Run("helm tools are not initially available", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		toolsets = "not a valid array
	`)), 0o644))
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Require().Eventually(func() bool {
		klog.Flush()
		return s.reloadExited.Load()
	}, 2*time.Second, 50*time.Millisecond)
	s.Equal(int32(1), s.reloadExitCode.Load())
	s.Contains(s.logBuffer.String(), "Failed to reload configuration")
	s.True(slices.Contains(s.server.GetEnabledTools(), "events_list"))
	s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
}

func (s *SIGHUPSuite) TestSIGHUPOIDCDiscoveryFailureKeepsPreviousConfig() {
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		port = "18080"
		toolsets = ["core", "config"]
		require_oauth = true
		skip_jwt_verification = true
	`)), 0o644))
	_ = s.InitServer(configPath, "")

	issuer := httptest.NewServer(http.NotFoundHandler())
	issuer.Close()

	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(fmt.Sprintf(`
		port = "18080"
		toolsets = ["core", "config", "helm"]
		require_oauth = true
		authorization_url = %q
	`, issuer.URL))), 0o644))
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Require().Eventually(func() bool {
		klog.Flush()
		return strings.Contains(s.logBuffer.String(), "Failed to recreate OIDC provider during reload")
	}, 2*time.Second, 50*time.Millisecond)

	s.Run("does not apply the candidate MCP config", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
		s.Equal([]string{"core", "config"}, s.cfgState.Load().Toolsets.Get())
	})
	s.Run("does not publish a new OAuth snapshot", func() {
		s.Empty(s.oauthState.Load().AuthorizationURL)
		s.Nil(s.oauthState.Load().OIDCProvider)
	})
	s.Run("does not exit", func() {
		s.False(s.reloadExited.Load())
	})
}

func (s *SIGHUPSuite) TestSIGHUPValidatesBeforeOAuthDiscovery() {
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		toolsets = ["core", "config"]
	`)), 0o644))
	_ = s.InitServer(configPath, "")

	var discoveryRequests atomic.Int32
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		discoveryRequests.Add(1)
		http.NotFound(w, nil)
	}))
	s.T().Cleanup(issuer.Close)

	// authorization_url without require_oauth is invalid and must be rejected
	// before OIDC discovery or either live state is published.
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(fmt.Sprintf(`
		toolsets = ["core", "config", "helm"]
		authorization_url = %q
	`, issuer.URL))), 0o644))
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Require().Eventually(func() bool {
		klog.Flush()
		return s.reloadExited.Load()
	}, 2*time.Second, 50*time.Millisecond)

	s.Run("does not attempt discovery", func() {
		s.Equal(int32(0), discoveryRequests.Load())
	})
	s.Run("does not publish candidate state", func() {
		s.Equal([]string{"core", "config"}, s.cfgState.Load().Toolsets.Get())
		s.Empty(s.oauthState.Load().AuthorizationURL)
	})
}

func (s *SIGHUPSuite) enableVerboseKlog() {
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	s.Require().NoError(fs.Set("v", "1"))
}

func (s *SIGHUPSuite) TestSIGHUPDumpAfterSuccessfulReload() {
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		list_output = "table"
		toolsets = ["core", "config"]
	`)), 0o644))
	_ = s.InitServer(configPath, "")
	s.enableVerboseKlog()
	s.logBuffer.Reset()

	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		list_output = "yaml"
		toolsets = ["core", "config"]
	`)), 0o644))
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Require().Eventually(func() bool {
		klog.Flush()
		return strings.Contains(s.logBuffer.String(), "Configuration reloaded successfully via SIGHUP")
	}, 2*time.Second, 50*time.Millisecond)
	logs := s.logBuffer.String()
	s.Contains(logs, `option="list_output"`)
	s.Contains(logs, "changed=true")
	s.Contains(logs, "yaml")
}

func (s *SIGHUPSuite) TestSIGHUPRejectedReloadExits() {
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		toolsets = ["core", "config"]
	`)), 0o644))
	_ = s.InitServer(configPath, "")
	s.enableVerboseKlog()
	s.logBuffer.Reset()

	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		toolsets = ["not-a-real-toolset"]
	`)), 0o644))
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Require().Eventually(func() bool {
		klog.Flush()
		return s.reloadExited.Load()
	}, 2*time.Second, 50*time.Millisecond)
	s.Equal(int32(1), s.reloadExitCode.Load())
	logs := s.logBuffer.String()
	s.Contains(logs, "Failed to apply reloaded configuration")
	s.Contains(logs, "config option")
	s.Contains(logs, `option="toolsets"`)
	s.Contains(logs, "changed=true")
	s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
}

func (s *SIGHUPSuite) TestSIGHUPWithConfigDirOnly() {
	// Create initial drop-in file without helm
	dropInPath := filepath.Join(s.dropInConfigDir, "10-settings.toml")
	s.Require().NoError(os.WriteFile(dropInPath, []byte(s.withKube(`
		toolsets = ["core", "config"]
	`)), 0o644))

	_ = s.InitServer("", s.dropInConfigDir)

	s.Run("helm tools are not initially available", func() {
		s.False(slices.Contains(s.server.GetEnabledTools(), "helm_list"))
	})

	// Update drop-in file to add helm
	s.Require().NoError(os.WriteFile(dropInPath, []byte(s.withKube(`
		toolsets = ["core", "config", "helm"]
	`)), 0o644))

	// Send SIGHUP
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("helm tools become available after SIGHUP with config-dir only", func() {
		s.Require().Eventually(func() bool {
			return slices.Contains(s.server.GetEnabledTools(), "helm_list")
		}, 2*time.Second, 50*time.Millisecond)
	})
}

func (s *SIGHUPSuite) TestSIGHUPPreservesMetricsPort() {
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		port = "18080"
		metrics_port = "9090"
		toolsets = ["core", "config"]
	`)), 0o644))
	_ = s.InitServer(configPath, "")

	s.Run("metrics_port is set at startup", func() {
		s.Equal("9090", s.cfgState.Load().MetricsPort.Get())
	})

	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		port = "18080"
		metrics_port = "9090"
		toolsets = ["core", "config", "helm"]
	`)), 0o644))
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("unchanged metrics_port reloads other fields", func() {
		s.Require().Eventually(func() bool {
			return slices.Contains(s.server.GetEnabledTools(), "helm_list")
		}, 2*time.Second, 50*time.Millisecond)
		s.Equal("9090", s.cfgState.Load().MetricsPort.Get())
		s.False(s.reloadExited.Load())
	})
}

func (s *SIGHUPSuite) TestSIGHUPRejectsNonReloadableMetricsPort() {
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		port = "18080"
		metrics_port = "9090"
		toolsets = ["core", "config"]
	`)), 0o644))
	_ = s.InitServer(configPath, "")

	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
		port = "18080"
		metrics_port = "9091"
		toolsets = ["core", "config"]
	`)), 0o644))
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Require().Eventually(func() bool {
		klog.Flush()
		return s.reloadExited.Load()
	}, 2*time.Second, 50*time.Millisecond)
	s.Equal(int32(1), s.reloadExitCode.Load())
	s.Contains(s.logBuffer.String(), "non-reloadable option metrics_port changed")
	s.Equal("9090", s.cfgState.Load().MetricsPort.Get())
}

func (s *SIGHUPSuite) TestSIGHUPIgnoresWhitespaceOnNonReloadableTLSPaths() {
	certPath := filepath.Join(s.tempDir, "tls.crt")
	keyPath := filepath.Join(s.tempDir, "tls.key")
	s.Require().NoError(os.WriteFile(certPath, []byte("cert"), 0o644))
	s.Require().NoError(os.WriteFile(keyPath, []byte("key"), 0o644))

	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(fmt.Sprintf(`
		port = "18080"
		tls_cert = %q
		tls_key = %q
		toolsets = ["core", "config"]
	`, certPath, keyPath))), 0o644))
	_ = s.InitServer(configPath, "")

	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(fmt.Sprintf(`
		port = "18080"
		tls_cert = " %s "
		tls_key = " %s "
		toolsets = ["core", "config", "helm"]
	`, certPath, keyPath))), 0o644))
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("reloadable change is applied", func() {
		s.Require().Eventually(func() bool {
			return slices.Contains(s.server.GetEnabledTools(), "helm_list")
		}, 2*time.Second, 50*time.Millisecond)
	})
	s.Run("does not exit or rewrite the TLS paths", func() {
		s.False(s.reloadExited.Load())
		s.Equal(certPath, s.cfgState.Load().TLSCert.Get())
		s.Equal(keyPath, s.cfgState.Load().TLSKey.Get())
	})
}

func (s *SIGHUPSuite) TestSIGHUPReloadsPrompts() {
	// Create initial config with one prompt
	configPath := filepath.Join(s.tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
        [[prompts]]
        name = "initial-prompt"
        description = "Initial prompt"

        [[prompts.messages]]
        role = "user"
        content = "Initial message"
    `)), 0o644))
	_ = s.InitServer(configPath, "")

	enabledPrompts := s.server.GetEnabledPrompts()
	s.GreaterOrEqual(len(enabledPrompts), 1)
	s.Contains(enabledPrompts, "initial-prompt")

	// Update config with new prompt
	s.Require().NoError(os.WriteFile(configPath, []byte(s.withKube(`
        [[prompts]]
        name = "updated-prompt"
        description = "Updated prompt"

        [[prompts.messages]]
        role = "user"
        content = "Updated message"
    `)), 0o644))

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
	mockServer := test.NewMockServer()
	mockServer.Handle(test.NewDiscoveryClientHandler())
	t.Cleanup(mockServer.Close)
	kubeconfigPath := mockServer.KubeconfigFile(t)
	write := func(logFile string) error {
		return os.WriteFile(configPath, []byte(fmt.Sprintf(
			"kubeconfig = %q\nlog_file = %q\nlog_level = 1\n", kubeconfigPath, logFile)), 0o644)
	}
	if err := write(pathA); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Read(t.Context(), configPath, "")
	if err != nil {
		t.Fatal(err)
	}

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

	provider, err := kubernetes.NewProvider(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	mcpServer, err := mcp.NewServer(t.Context(), mcp.Configuration{Config: cfg, SDKLogger: sink.SDKLogger()}, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mcpServer.Close)

	opts := &MCPServerOptions{
		ConfigPath: configPath,
		Config:     cfg,
		IOStreams: genericiooptions.IOStreams{
			Out:    logBuffer,
			ErrOut: logBuffer,
		},
		logSink: sink,
		exit:    func(int) {},
	}
	cfgState := config.NewConfigState(cfg)
	stop := opts.setupSIGHUPHandler(t.Context(), mcpServer, oauth.NewState(&oauth.Snapshot{}), cfgState)
	t.Cleanup(stop)

	if err := write(pathB); err != nil {
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

// HTTPSIGHUPSuite tests the SIGHUP configuration reload behavior for the HTTP server
type HTTPSIGHUPSuite struct {
	suite.Suite
	*baseSIGHUPSetup
	configPath      string
	httpClient      *http.Client
	httpAddress     string
	timeoutCancel   context.CancelFunc
	stopServer      context.CancelFunc
	waitForShutdown func() error
}

func (s *HTTPSIGHUPSuite) SetupTest() {
	s.baseSIGHUPSetup = setupSIGHUPTest(s.T())
	s.configPath = filepath.Join(s.tempDir, "config.toml")
	s.httpClient = &http.Client{Timeout: 10 * time.Second}
}

func (s *HTTPSIGHUPSuite) TearDownTest() {
	defer s.teardown()

	if s.stopServer == nil {
		return
	}
	s.stopServer()
	if s.waitForShutdown != nil {
		// Non-fatal: let the rest of teardown run even if shutdown regressed.
		s.NoError(s.waitForShutdown(), "HTTP server did not shut down gracefully")
	}
	if s.timeoutCancel != nil {
		s.timeoutCancel()
		s.timeoutCancel = nil
	}
	s.stopServer = nil
	s.waitForShutdown = nil
}

// getToolsList queries the MCP server via HTTP to get the list of available tools
func (s *HTTPSIGHUPSuite) getToolsList() ([]string, error) {
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1.0.0"}, nil)
	transport := &sdk.StreamableClientTransport{
		Endpoint: fmt.Sprintf("http://%s/mcp", s.httpAddress),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(ctx, &sdk.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	toolNames := make([]string, len(tools.Tools))
	for i, tool := range tools.Tools {
		toolNames[i] = tool.Name
	}
	return toolNames, nil
}

func (s *HTTPSIGHUPSuite) TestSIGHUPReloadsConfigFromFile() {
	// Create initial config file - start with only core toolset (no helm)
	tcpAddr, err := test.RandomPortAddress()
	s.Require().NoError(err)
	s.httpAddress = fmt.Sprintf("127.0.0.1:%d", tcpAddr.Port)

	s.Require().NoError(os.WriteFile(s.configPath, []byte(fmt.Sprintf(`
		port = "%d"
		kubeconfig = "%s"
		toolsets = ["core", "config"]
	`, tcpAddr.Port, s.kubeconfigPath)), 0o644))

	// Create MCPServerOptions with config file set to trigger HTTP mode
	opts := &MCPServerOptions{
		ConfigPath: s.configPath,
		IOStreams: genericiooptions.IOStreams{
			Out:    s.logBuffer,
			ErrOut: s.logBuffer,
		},
		exit: func(int) {},
	}

	// Run via root.go in a goroutine. A cancelable context (like http_test.go)
	// stops the server instead of a process-wide SIGTERM; the 10s timeout context
	// is the backstop so group.Wait can't hang if Serve does.
	var timeoutCtx, cancelCtx context.Context
	timeoutCtx, s.timeoutCancel = context.WithTimeout(s.T().Context(), 10*time.Second)
	group, gc := errgroup.WithContext(timeoutCtx)
	cancelCtx, s.stopServer = context.WithCancel(gc)

	group.Go(func() error {
		rootCmd := NewMCPServer(opts.IOStreams)
		if err := opts.Complete(cancelCtx, rootCmd); err != nil {
			return err
		}
		return opts.Run(cancelCtx)
	})
	s.waitForShutdown = group.Wait

	// Wait for server to start
	s.Require().NoError(test.WaitForServer(tcpAddr), "HTTP server did not start in time")
	s.Require().NoError(test.WaitForHealthz(tcpAddr), "HTTP server /healthz endpoint did not respond in time")

	// Get initial tools list - should NOT have helm tools
	toolsBefore, err := s.getToolsList()
	s.Require().NoError(err, "Should be able to query tools list")
	s.False(slices.ContainsFunc(toolsBefore, func(t string) bool {
		return strings.HasPrefix(t, "helm_")
	}), "Should not have helm tools initially")

	// Modify the config file to add helm toolset
	s.Require().NoError(os.WriteFile(s.configPath, []byte(fmt.Sprintf(`
		port = "%d"
		kubeconfig = "%s"
		toolsets = ["core", "config", "helm"]
	`, tcpAddr.Port, s.kubeconfigPath)), 0o644))

	// Send SIGHUP to current process
	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("helm tools become available after SIGHUP", func() {
		var toolsAfter []string
		s.Require().Eventually(func() bool {
			var err error
			toolsAfter, err = s.getToolsList()
			if err != nil {
				return false
			}
			return slices.ContainsFunc(toolsAfter, func(t string) bool {
				return strings.HasPrefix(t, "helm_")
			})
		}, 3*time.Second, 200*time.Millisecond, "Helm tools should appear after SIGHUP and config reload")

		// Reload must be additive: len(after) > len(before) would still pass if a
		// core tool were dropped as helm was added, so require the full superset.
		for _, tool := range toolsBefore {
			s.Contains(toolsAfter, tool, "reload should not drop previously-available tool %q", tool)
		}
	})

	s.Run("server continues to respond after SIGHUP", func() {
		s.Require().Eventually(func() bool {
			resp, err := s.httpClient.Get(fmt.Sprintf("http://%s/healthz", s.httpAddress))
			if err != nil {
				return false
			}
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode == http.StatusOK
		}, 2*time.Second, 50*time.Millisecond, "Server should continue responding after SIGHUP")
	})

	s.Run("no shutdown messages in logs", func() {
		// Poll the negative over a window (not a fixed sleep + single sample):
		// Never fails the moment a shutdown line appears.
		// Capture the buffer so Never's leftover ticker goroutine does not
		// race SetupTest replacing s.baseSIGHUPSetup.
		logBuffer := s.logBuffer
		s.Never(func() bool {
			logOutput := logBuffer.String()
			return strings.Contains(logOutput, "initiating graceful shutdown") ||
				strings.Contains(logOutput, "Shutting down HTTP server")
		}, 500*time.Millisecond, 50*time.Millisecond, "SIGHUP must not trigger shutdown of the HTTP server")
	})
}

// TestSIGHUPIgnoredWithoutConfig drives the no-config HTTP path end-to-end:
// Complete always loads defaults/env (no --config), then we set port for the
// test. root.go registers no reload handler unless ConfigPath/ConfigDir is
// set, so Serve's SIGHUP registration alone must keep the process alive.
func (s *HTTPSIGHUPSuite) TestSIGHUPIgnoredWithoutConfig() {
	tcpAddr, err := test.RandomPortAddress()
	s.Require().NoError(err)
	s.httpAddress = fmt.Sprintf("127.0.0.1:%d", tcpAddr.Port)

	opts := &MCPServerOptions{
		IOStreams: genericiooptions.IOStreams{Out: s.logBuffer, ErrOut: s.logBuffer},
	}

	var timeoutCtx, cancelCtx context.Context
	timeoutCtx, s.timeoutCancel = context.WithTimeout(s.T().Context(), 10*time.Second)
	group, gc := errgroup.WithContext(timeoutCtx)
	cancelCtx, s.stopServer = context.WithCancel(gc)
	group.Go(func() error {
		rootCmd := NewMCPServer(opts.IOStreams)
		if err := opts.Complete(cancelCtx, rootCmd); err != nil {
			return err
		}
		opts.Config.Port.SetForTest(fmt.Sprintf("%d", tcpAddr.Port))
		opts.Config.KubeConfig.SetForTest(s.mockServer.KubeconfigFile(s.T()))
		return opts.Run(cancelCtx)
	})
	s.waitForShutdown = group.Wait

	s.Require().NoError(test.WaitForServer(tcpAddr), "HTTP server did not start in time")
	s.Require().NoError(test.WaitForHealthz(tcpAddr), "HTTP server /healthz endpoint did not respond in time")

	s.Require().NoError(syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	s.Run("no-config server keeps serving after SIGHUP", func() {
		logBuffer := s.logBuffer
		s.Never(func() bool {
			logOutput := logBuffer.String()
			return strings.Contains(logOutput, "initiating graceful shutdown") ||
				strings.Contains(logOutput, "Shutting down HTTP server")
		}, 500*time.Millisecond, 50*time.Millisecond, "SIGHUP must not shut down the no-config HTTP server")

		resp, err := s.httpClient.Get(fmt.Sprintf("http://%s/healthz", s.httpAddress))
		s.Require().NoError(err, "server should keep serving after SIGHUP")
		defer func() { _ = resp.Body.Close() }()
		s.Equal(http.StatusOK, resp.StatusCode)
	})
}

func TestHTTPSIGHUP(t *testing.T) {
	suite.Run(t, new(HTTPSIGHUPSuite))
}
