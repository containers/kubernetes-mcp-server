package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
)

func captureOutput(f func() error) (string, error) {
	originalOut := os.Stdout
	defer func() {
		os.Stdout = originalOut
	}()
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := f()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out), err
}

func testStream() (genericiooptions.IOStreams, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return genericiooptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    out,
		ErrOut: io.Discard,
	}, out
}

func TestVersion(t *testing.T) {
	ioStreams, out := testStream()
	rootCmd := NewMCPServer(ioStreams)
	rootCmd.SetArgs([]string{"--version"})
	if err := rootCmd.Execute(); out.String() != "0.0.0\n" {
		t.Fatalf("Expected version 0.0.0, got %s %v", out.String(), err)
	}
}

func TestConfig(t *testing.T) {
	t.Run("defaults to none", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1"})
		expectedConfig := `" - Config: "`
		if err := rootCmd.Execute(); !strings.Contains(out.String(), expectedConfig) {
			t.Fatalf("Expected config to be %s, got %s %v", expectedConfig, out.String(), err)
		}
	})
	t.Run("set with --config", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		_, file, _, _ := runtime.Caller(0)
		emptyConfigPath := filepath.Join(filepath.Dir(file), "testdata", "empty-config.toml")
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--config", emptyConfigPath})
		_ = rootCmd.Execute()
		expected := `(?m)\" - Config\:[^\"]+empty-config\.toml\"`
		if m, err := regexp.MatchString(expected, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expected, out.String(), err)
		}
	})
	t.Run("invalid path throws error", func(t *testing.T) {
		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--config", "invalid-path-to-config.toml"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("Expected error for invalid config path, got nil")
		}
		expected := "failed to read and merge config files: failed to read config invalid-path-to-config.toml:"
		if !strings.HasPrefix(err.Error(), expected) {
			t.Fatalf("Expected error to be %s, got %s", expected, err.Error())
		}
	})
	t.Run("set with valid --config", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		_, file, _, _ := runtime.Caller(0)
		validConfigPath := filepath.Join(filepath.Dir(file), "testdata", "valid-config.toml")
		rootCmd.SetArgs([]string{"--version", "--config", validConfigPath})
		_ = rootCmd.Execute()
		expectedConfig := `(?m)\" - Config\:[^\"]+valid-config\.toml\"`
		if m, err := regexp.MatchString(expectedConfig, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedConfig, out.String(), err)
		}
		expectedListOutput := `(?m)\" - ListOutput\: yaml"`
		if m, err := regexp.MatchString(expectedListOutput, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedListOutput, out.String(), err)
		}
		expectedReadOnly := `(?m)\" - Read-only mode: true"`
		if m, err := regexp.MatchString(expectedReadOnly, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedReadOnly, out.String(), err)
		}
		expectedDisableDestruction := `(?m)\" - Disable destructive tools: true"`
		if m, err := regexp.MatchString(expectedDisableDestruction, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedDisableDestruction, out.String(), err)
		}
	})
	t.Run("set with valid --config, flags take precedence", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		_, file, _, _ := runtime.Caller(0)
		validConfigPath := filepath.Join(filepath.Dir(file), "testdata", "valid-config.toml")
		rootCmd.SetArgs([]string{"--version", "--list-output=table", "--disable-destructive=false", "--read-only=false", "--config", validConfigPath})
		_ = rootCmd.Execute()
		expected := `(?m)\" - Config\:[^\"]+valid-config\.toml\"`
		if m, err := regexp.MatchString(expected, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expected, out.String(), err)
		}
		expectedListOutput := `(?m)\" - ListOutput\: table"`
		if m, err := regexp.MatchString(expectedListOutput, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedListOutput, out.String(), err)
		}
		expectedReadOnly := `(?m)\" - Read-only mode: false"`
		if m, err := regexp.MatchString(expectedReadOnly, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedReadOnly, out.String(), err)
		}
		expectedDisableDestruction := `(?m)\" - Disable destructive tools: false"`
		if m, err := regexp.MatchString(expectedDisableDestruction, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedDisableDestruction, out.String(), err)
		}
	})
}

func TestToolsets(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--help"})
		o, err := captureOutput(rootCmd.Execute) // --help doesn't use logger/klog, cobra prints directly to stdout
		if !strings.Contains(o, "Comma-separated list of MCP toolsets to use (available toolsets: config, core, helm, kiali, kubevirt).") {
			t.Fatalf("Expected all available toolsets, got %s %v", o, err)
		}
	})
	t.Run("default", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1"})
		if err := rootCmd.Execute(); !strings.Contains(out.String(), "- Toolsets: core, config, helm") {
			t.Fatalf("Expected toolsets 'full', got %s %v", out, err)
		}
	})
	t.Run("set with --toolsets", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--toolsets", "helm,config"})
		_ = rootCmd.Execute()
		expected := `(?m)\" - Toolsets\: helm, config\"`
		if m, err := regexp.MatchString(expected, out.String()); !m || err != nil {
			t.Fatalf("Expected toolset to be %s, got %s %v", expected, out.String(), err)
		}
	})
}

func TestListOutput(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--help"})
		o, err := captureOutput(rootCmd.Execute) // --help doesn't use logger/klog, cobra prints directly to stdout
		if !strings.Contains(o, "Output format for resource list operations (one of: yaml, table)") {
			t.Fatalf("Expected all available outputs, got %s %v", o, err)
		}
	})
	t.Run("defaults to table", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1"})
		if err := rootCmd.Execute(); !strings.Contains(out.String(), "- ListOutput: table") {
			t.Fatalf("Expected list-output 'table', got %s %v", out, err)
		}
	})
	t.Run("set with --list-output", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--list-output", "yaml"})
		_ = rootCmd.Execute()
		expected := `(?m)\" - ListOutput\: yaml\"`
		if m, err := regexp.MatchString(expected, out.String()); !m || err != nil {
			t.Fatalf("Expected list-output to be %s, got %s %v", expected, out.String(), err)
		}
	})
}

func TestReadOnly(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1"})
		if err := rootCmd.Execute(); !strings.Contains(out.String(), " - Read-only mode: false") {
			t.Fatalf("Expected read-only mode false, got %s %v", out, err)
		}
	})
	t.Run("set with --read-only", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--read-only"})
		_ = rootCmd.Execute()
		expected := `(?m)\" - Read-only mode\: true\"`
		if m, err := regexp.MatchString(expected, out.String()); !m || err != nil {
			t.Fatalf("Expected read-only mode to be %s, got %s %v", expected, out.String(), err)
		}
	})
}

func TestDisableDestructive(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1"})
		if err := rootCmd.Execute(); !strings.Contains(out.String(), " - Disable destructive tools: false") {
			t.Fatalf("Expected disable destructive false, got %s %v", out, err)
		}
	})
	t.Run("set with --disable-destructive", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--disable-destructive"})
		_ = rootCmd.Execute()
		expected := `(?m)\" - Disable destructive tools\: true\"`
		if m, err := regexp.MatchString(expected, out.String()); !m || err != nil {
			t.Fatalf("Expected disable-destructive mode to be %s, got %s %v", expected, out.String(), err)
		}
	})
}

func TestAuthorizationURL(t *testing.T) {
	t.Run("invalid authorization-url without protocol", func(t *testing.T) {
		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--require-oauth", "--port=8080", "--authorization-url", "example.com/auth", "--server-url", "https://example.com:8080"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("Expected error for invalid authorization-url without protocol, got nil")
		}
		expected := "--authorization-url must be a valid URL"
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("Expected error to contain %s, got %s", expected, err.Error())
		}
	})
	t.Run("valid authorization-url with https", func(t *testing.T) {
		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--require-oauth", "--port=8080", "--authorization-url", "https://example.com/auth", "--server-url", "https://example.com:8080"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Expected no error for valid https authorization-url, got %s", err.Error())
		}
	})
}

func TestStdioLogging(t *testing.T) {
	t.Run("stdio disables klog", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--log-level=1"})
		err := rootCmd.Execute()
		require.NoErrorf(t, err, "Expected no error executing command, got %v", err)
		assert.Equalf(t, "0.0.0\n", out.String(), "Expected only version output, got %s", out.String())
	})
	t.Run("http mode enables klog", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--log-level=1", "--port=1337"})
		err := rootCmd.Execute()
		require.NoErrorf(t, err, "Expected no error executing command, got %v", err)
		assert.Containsf(t, out.String(), "Starting kubernetes-mcp-server", "Expected klog output, got %s", out.String())
	})
}

func TestDisableMultiCluster(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1"})
		if err := rootCmd.Execute(); !strings.Contains(out.String(), " - ClusterProviderStrategy: auto-detect (it is recommended to set this explicitly in your Config)") {
			t.Fatalf("Expected ClusterProviderStrategy kubeconfig, got %s %v", out, err)
		}
	})
	t.Run("set with --disable-multi-cluster", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--disable-multi-cluster"})
		_ = rootCmd.Execute()
		expected := `(?m)\" - ClusterProviderStrategy\: disabled\"`
		if m, err := regexp.MatchString(expected, out.String()); !m || err != nil {
			t.Fatalf("Expected ClusterProviderStrategy %s, got %s %v", expected, out.String(), err)
		}
	})
}

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
	s.mockServer.Handle(&test.DiscoveryClientHandler{})
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

	s.server, err = mcp.NewServer(mcp.Configuration{
		StaticConfig: cfg,
	})
	s.Require().NoError(err)
	// Set up SIGHUP handler
	opts := &MCPServerOptions{
		ConfigPath: configPath,
		ConfigDir:  configDir,
	}
	opts.setupSIGHUPHandler(s.server)
}

func (s *SIGHUPSuite) TestSIGHUPReloadsConfigFromFile() {
	if runtime.GOOS == "windows" {
		s.T().Skip("SIGHUP is not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		s.T().Skip("SIGHUP is not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		s.T().Skip("SIGHUP is not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		s.T().Skip("SIGHUP is not supported on Windows")
	}

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

func TestSIGHUP(t *testing.T) {
	suite.Run(t, new(SIGHUPSuite))
}
