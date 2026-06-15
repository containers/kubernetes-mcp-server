//go:build !windows

package cmd

import (
	"context"
	"fmt"
	"net/http"
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
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/errgroup"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
)

// baseSIGHUPSetup contains common setup for SIGHUP tests
type baseSIGHUPSetup struct {
	mockServer *test.MockServer
	tempDir    string
	logBuffer  *test.SyncBuffer
	klogState  klog.State
}

// setupSIGHUPTest performs common SIGHUP test setup
func setupSIGHUPTest(t *testing.T) *baseSIGHUPSetup {
	s := &baseSIGHUPSetup{}
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
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

// SIGHUPSuite tests the SIGHUP configuration reload behavior for STDIO mode
type SIGHUPSuite struct {
	suite.Suite
	*baseSIGHUPSetup
	dropInConfigDir string
	server          *mcp.Server
	stopSIGHUP      func()
}

func (s *SIGHUPSuite) SetupTest() {
	s.baseSIGHUPSetup = setupSIGHUPTest(s.T())
	s.dropInConfigDir = filepath.Join(s.tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(s.dropInConfigDir, 0o755))
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

// HTTPSIGHUPSuite tests the SIGHUP configuration reload behavior for the HTTP server
type HTTPSIGHUPSuite struct {
	suite.Suite
	*baseSIGHUPSetup
	configPath      string
	httpClient      *http.Client
	httpAddress     string
	timeoutCancel   context.CancelFunc
	stopServer      func()
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
		_ = s.waitForShutdown()
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
	`, tcpAddr.Port, s.mockServer.KubeconfigFile(s.T()))), 0o644))

	// Create MCPServerOptions with config file set to trigger HTTP mode
	opts := &MCPServerOptions{
		ConfigPath: s.configPath,
		IOStreams: genericiooptions.IOStreams{
			Out:    s.logBuffer,
			ErrOut: s.logBuffer,
		},
	}

	// Start the server in a goroutine (going through root.go's Run path)
	var timeoutCtx context.Context
	timeoutCtx, s.timeoutCancel = context.WithTimeout(s.T().Context(), 10*time.Second)
	group, _ := errgroup.WithContext(timeoutCtx)

	group.Go(func() error {
		rootCmd := NewMCPServer(opts.IOStreams)
		if err := opts.Complete(rootCmd); err != nil {
			return err
		}
		return opts.Run()
	})
	s.waitForShutdown = group.Wait

	// We can't cancel via context, so we'll use SIGTERM at the end
	s.stopServer = func() {
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}

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
	`, tcpAddr.Port, s.mockServer.KubeconfigFile(s.T()))), 0o644))

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

		s.True(len(toolsAfter) > len(toolsBefore), "Should have more tools after adding helm toolset")
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
		time.Sleep(100 * time.Millisecond)
		logOutput := s.logBuffer.String()
		s.False(strings.Contains(logOutput, "Received signal hangup"), "Should not receive signal shutdown message for SIGHUP")
		s.False(strings.Contains(logOutput, "initiating graceful shutdown"), "Should not initiate shutdown")
		s.False(strings.Contains(logOutput, "Shutting down HTTP server"), "Should not shut down HTTP server")
	})
}

func TestHTTPSIGHUP(t *testing.T) {
	suite.Run(t, new(HTTPSIGHUPSuite))
}
