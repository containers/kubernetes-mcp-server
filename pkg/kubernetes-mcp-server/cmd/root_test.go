package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/cli-runtime/pkg/genericiooptions"
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
		expectedStateless := `(?m)\" - Stateless mode: true"`
		if m, err := regexp.MatchString(expectedStateless, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedStateless, out.String(), err)
		}
	})
	t.Run("set with valid --config, flags take precedence", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		_, file, _, _ := runtime.Caller(0)
		validConfigPath := filepath.Join(filepath.Dir(file), "testdata", "valid-config.toml")
		rootCmd.SetArgs([]string{"--version", "--list-output=table", "--disable-destructive=false", "--read-only=false", "--stateless=false", "--config", validConfigPath})
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
		expectedStateless := `(?m)\" - Stateless mode: false"`
		if m, err := regexp.MatchString(expectedStateless, out.String()); !m || err != nil {
			t.Fatalf("Expected stateless mode to be false (flag overrides config), got %s %v", out.String(), err)
		}
	})
	t.Run("stateless flag defaults to false", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1"})
		_ = rootCmd.Execute()
		expectedStateless := `(?m)\" - Stateless mode: false"`
		if m, err := regexp.MatchString(expectedStateless, out.String()); !m || err != nil {
			t.Fatalf("Expected stateless mode to be false by default, got %s %v", out.String(), err)
		}
	})
	t.Run("stateless flag set to true", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--stateless=true"})
		_ = rootCmd.Execute()
		expectedStateless := `(?m)\" - Stateless mode: true"`
		if m, err := regexp.MatchString(expectedStateless, out.String()); !m || err != nil {
			t.Fatalf("Expected stateless mode to be true, got %s %v", out.String(), err)
		}
	})
	t.Run("stateless flag set to false explicitly", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--stateless=false"})
		_ = rootCmd.Execute()
		expectedStateless := `(?m)\" - Stateless mode: false"`
		if m, err := regexp.MatchString(expectedStateless, out.String()); !m || err != nil {
			t.Fatalf("Expected stateless mode to be false, got %s %v", out.String(), err)
		}
	})
}

type CmdSuite struct {
	suite.Suite
	testDataDir string
}

func (s *CmdSuite) SetupSuite() {
	_, file, _, _ := runtime.Caller(0)
	s.testDataDir = filepath.Join(filepath.Dir(file), "testdata")
}

func (s *CmdSuite) TestConfigDir() {
	s.Run("set with --config-dir standalone", func() {
		dropInDir := s.T().TempDir()
		s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "10-config.toml"), []byte(`
			list_output = "yaml"
			read_only = true
			disable_destructive = true
		`), 0644))

		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--config-dir", dropInDir})
		s.Require().NoError(rootCmd.Execute())
		s.Contains(out.String(), "ListOutput: yaml")
		s.Contains(out.String(), "Read-only mode: true")
		s.Contains(out.String(), "Disable destructive tools: true")
	})
	s.Run("--config-dir path is a file throws error", func() {
		tempDir := s.T().TempDir()
		filePath := filepath.Join(tempDir, "not-a-directory.toml")
		s.Require().NoError(os.WriteFile(filePath, []byte("log_level = 1"), 0644))

		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--config-dir", filePath})
		err := rootCmd.Execute()
		s.Require().Error(err)
		s.Contains(err.Error(), "drop-in config path is not a directory")
	})
	s.Run("nonexistent --config-dir is silently skipped", func() {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--config-dir", "/nonexistent/path/to/config-dir"})
		err := rootCmd.Execute()
		s.Require().NoError(err, "Nonexistent directories should be gracefully skipped")
		s.Contains(out.String(), "ListOutput: table", "Default values should be used")
	})
	s.Run("--config with --config-dir merges configs", func() {
		tempDir := s.T().TempDir()
		mainConfigPath := filepath.Join(tempDir, "config.toml")
		s.Require().NoError(os.WriteFile(mainConfigPath, []byte(`
			list_output = "table"
			read_only = false
		`), 0644))

		dropInDir := filepath.Join(tempDir, "conf.d")
		s.Require().NoError(os.Mkdir(dropInDir, 0755))
		s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "10-override.toml"), []byte(`
			read_only = true
			disable_destructive = true
			stateless = true
		`), 0644))

		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--config", mainConfigPath, "--config-dir", dropInDir})
		s.Require().NoError(rootCmd.Execute())
		s.Contains(out.String(), "ListOutput: table", "list_output from main config")
		s.Contains(out.String(), "Read-only mode: true", "read_only overridden by drop-in")
		s.Contains(out.String(), "Disable destructive tools: true", "disable_destructive from drop-in")
		s.Contains(out.String(), "Stateless mode: true", "stateless from drop-in")
	})
	s.Run("multiple drop-in files are merged in order", func() {
		dropInDir := s.T().TempDir()
		s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "10-first.toml"), []byte(`
			list_output = "yaml"
			read_only = true
		`), 0644))
		s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "20-second.toml"), []byte(`
			list_output = "table"
			disable_destructive = true
		`), 0644))

		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--config-dir", dropInDir})
		s.Require().NoError(rootCmd.Execute())
		s.Contains(out.String(), "ListOutput: table", "list_output from 20-second.toml (last wins)")
		s.Contains(out.String(), "Read-only mode: true", "read_only from 10-first.toml")
		s.Contains(out.String(), "Disable destructive tools: true", "disable_destructive from 20-second.toml")
	})
	s.Run("flags take precedence over --config-dir", func() {
		dropInDir := s.T().TempDir()
		s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "10-config.toml"), []byte(`
			list_output = "yaml"
			read_only = true
			disable_destructive = true
			stateless = true
		`), 0644))

		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--list-output=table", "--read-only=false", "--disable-destructive=false", "--stateless=false", "--config-dir", dropInDir})
		s.Require().NoError(rootCmd.Execute())
		s.Contains(out.String(), "ListOutput: table", "flag takes precedence")
		s.Contains(out.String(), "Read-only mode: false", "flag takes precedence")
		s.Contains(out.String(), "Disable destructive tools: false", "flag takes precedence")
		s.Contains(out.String(), "Stateless mode: false", "flag takes precedence")
	})
}

func TestCmd(t *testing.T) {
	suite.Run(t, new(CmdSuite))
}

func TestToolsets(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--help"})
		o, err := captureOutput(rootCmd.Execute) // --help doesn't use logger/klog, cobra prints directly to stdout
		expected := fmt.Sprintf("Comma-separated list of MCP toolsets to use (available toolsets: %s).", strings.Join(toolsets.ToolsetNames(), ", "))
		if !strings.Contains(o, expected) {
			t.Fatalf("Expected all available toolsets, got %s %v", o, err)
		}
	})
	t.Run("default", func(t *testing.T) {
		if config.HasDefaultOverrides() {
			t.Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
		}
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
		if config.HasDefaultOverrides() {
			t.Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
		}
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

func TestStateless(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1"})
		if err := rootCmd.Execute(); !strings.Contains(out.String(), " - Stateless mode: false") {
			t.Fatalf("Expected stateless mode false, got %s %v", out, err)
		}
	})
	t.Run("set with --stateless", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--port=1337", "--log-level=1", "--stateless"})
		_ = rootCmd.Execute()
		expected := `(?m)\" - Stateless mode\: true\"`
		if m, err := regexp.MatchString(expected, out.String()); !m || err != nil {
			t.Fatalf("Expected stateless mode to be %s, got %s %v", expected, out.String(), err)
		}
	})
}
