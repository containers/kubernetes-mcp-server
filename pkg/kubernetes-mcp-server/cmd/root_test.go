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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
)

// dumpTOML enables HTTP mode and verbose logs so the startup dump appears on Out.
const dumpTOML = "port = \"1337\"\nlog_level = 1\n"

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

func writeTOML(t *testing.T, contents string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(p, []byte(contents), 0o600))
	return p
}

func executeVersion(t *testing.T, toml string, extraArgs ...string) (string, error) {
	t.Helper()
	ioStreams, out := testStream()
	rootCmd := NewMCPServer(ioStreams)
	args := []string{"--version"}
	if toml != "" {
		args = append(args, "--config", writeTOML(t, toml))
	}
	args = append(args, extraArgs...)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func TestVersion(t *testing.T) {
	ioStreams, out := testStream()
	rootCmd := NewMCPServer(ioStreams)
	rootCmd.SetArgs([]string{"--version"})
	if err := rootCmd.Execute(); out.String() != "0.0.0\n" {
		t.Fatalf("Expected version 0.0.0, got %s %v", out.String(), err)
	}
}

func TestDroppedFlagsAreRejected(t *testing.T) {
	for _, flag := range []string{"--port=8080", "--kubeconfig=/tmp/x", "--toolsets=core", "--log-level=1"} {
		t.Run(flag, func(t *testing.T) {
			ioStreams, _ := testStream()
			rootCmd := NewMCPServer(ioStreams)
			rootCmd.SetArgs([]string{"--version", flag})
			err := rootCmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown flag")
		})
	}
}

func TestConfig(t *testing.T) {
	t.Run("defaults to none", func(t *testing.T) {
		dropInDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dropInDir, "00-dump.toml"), []byte(dumpTOML), 0o644))
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config-dir", dropInDir})
		err := rootCmd.Execute()
		expectedConfig := `config.path=""`
		if err != nil || !strings.Contains(out.String(), expectedConfig) {
			t.Fatalf("Expected config to be %s, got %s %v", expectedConfig, out.String(), err)
		}
	})
	t.Run("set with --config", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		configPath := writeTOML(t, dumpTOML)
		rootCmd.SetArgs([]string{"--version", "--config", configPath})
		_ = rootCmd.Execute()
		expected := `config\.path="[^"]*config\.toml"`
		if m, err := regexp.MatchString(expected, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expected, out.String(), err)
		}
	})
	t.Run("invalid path throws error", func(t *testing.T) {
		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config", "invalid-path-to-config.toml"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("Expected error for invalid config path, got nil")
		}
		expected := "failed to read config invalid-path-to-config.toml:"
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("Expected error to contain %s, got %s", expected, err.Error())
		}
	})
	t.Run("set with valid --config", func(t *testing.T) {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		_, file, _, _ := runtime.Caller(0)
		validConfigPath := filepath.Join(filepath.Dir(file), "testdata", "valid-config.toml")
		rootCmd.SetArgs([]string{"--version", "--config", validConfigPath})
		_ = rootCmd.Execute()
		expectedConfig := `config\.path="[^"]*valid-config\.toml"`
		if m, err := regexp.MatchString(expectedConfig, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedConfig, out.String(), err)
		}
		expectedListOutput := `config\.list_output="yaml"`
		if m, err := regexp.MatchString(expectedListOutput, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedListOutput, out.String(), err)
		}
		expectedReadOnly := `config\.read_only=true`
		if m, err := regexp.MatchString(expectedReadOnly, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedReadOnly, out.String(), err)
		}
		expectedDisableDestruction := `config\.disable_destructive=true`
		if m, err := regexp.MatchString(expectedDisableDestruction, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedDisableDestruction, out.String(), err)
		}
		expectedStateless := `config\.stateless=true`
		if m, err := regexp.MatchString(expectedStateless, out.String()); !m || err != nil {
			t.Fatalf("Expected config to be %s, got %s %v", expectedStateless, out.String(), err)
		}
	})
	t.Run("stateless defaults to false", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML)
		require.NoError(t, err)
		expectedStateless := `config\.stateless=false`
		if m, matchErr := regexp.MatchString(expectedStateless, out); !m || matchErr != nil {
			t.Fatalf("Expected stateless mode to be false by default, got %s %v", out, err)
		}
	})
	t.Run("stateless set to true in TOML", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML+"stateless = true\n")
		require.NoError(t, err)
		expectedStateless := `config\.stateless=true`
		if m, matchErr := regexp.MatchString(expectedStateless, out); !m || matchErr != nil {
			t.Fatalf("Expected stateless mode to be true, got %s %v", out, err)
		}
	})
}

type CmdSuite struct {
	suite.Suite
	testDataDir string
	klogState   klog.State
}

func (s *CmdSuite) SetupSuite() {
	_, file, _, _ := runtime.Caller(0)
	s.testDataDir = filepath.Join(filepath.Dir(file), "testdata")
}

// SetupTest captures klog's package-level state before each test so that
// each test method starts from a clean slate. The tests run rootCmd.Execute,
// which constructs a logging.Sink and mutates klog globals; without this,
// per-method state would bleed and (under -race) could surface as flakes.
func (s *CmdSuite) SetupTest() {
	s.klogState = klog.CaptureState()
}

func (s *CmdSuite) TearDownTest() {
	s.klogState.Restore()
}

func (s *CmdSuite) TestConfigDir() {
	s.Run("set with --config-dir standalone", func() {
		dropInDir := s.T().TempDir()
		s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "10-config.toml"), []byte(dumpTOML+`
			list_output = "yaml"
			read_only = true
			disable_destructive = true
		`), 0o644))

		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config-dir", dropInDir})
		s.Require().NoError(rootCmd.Execute())
		s.Contains(out.String(), `config.list_output="yaml"`)
		s.Contains(out.String(), "config.read_only=true")
		s.Contains(out.String(), "config.disable_destructive=true")
	})
	s.Run("--config-dir path is a file throws error", func() {
		tempDir := s.T().TempDir()
		filePath := filepath.Join(tempDir, "not-a-directory.toml")
		s.Require().NoError(os.WriteFile(filePath, []byte("log_level = 1"), 0o644))

		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config-dir", filePath})
		err := rootCmd.Execute()
		s.Require().Error(err)
		s.Contains(err.Error(), "drop-in config path is not a directory")
	})
	s.Run("nonexistent --config-dir is silently skipped", func() {
		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config", writeTOML(s.T(), dumpTOML), "--config-dir", "/nonexistent/path/to/config-dir"})
		err := rootCmd.Execute()
		s.Require().NoError(err, "Nonexistent directories should be gracefully skipped")
		s.Contains(out.String(), fmt.Sprintf(`config.list_output="%s"`, config.New().ListOutput.Get()), "Default values should be used")
	})
	s.Run("--config with --config-dir merges configs", func() {
		tempDir := s.T().TempDir()
		mainConfigPath := filepath.Join(tempDir, "config.toml")
		s.Require().NoError(os.WriteFile(mainConfigPath, []byte(dumpTOML+`
			list_output = "table"
			read_only = false
		`), 0o644))

		dropInDir := filepath.Join(tempDir, "conf.d")
		s.Require().NoError(os.Mkdir(dropInDir, 0o755))
		s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "10-override.toml"), []byte(`
			read_only = true
			disable_destructive = true
			stateless = true
		`), 0o644))

		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config", mainConfigPath, "--config-dir", dropInDir})
		s.Require().NoError(rootCmd.Execute())
		s.Contains(out.String(), `config.list_output="table"`, "list_output from main config")
		s.Contains(out.String(), "config.read_only=true", "read_only overridden by drop-in")
		s.Contains(out.String(), "config.disable_destructive=true", "disable_destructive from drop-in")
		s.Contains(out.String(), "config.stateless=true", "stateless from drop-in")
	})
	s.Run("multiple drop-in files are merged in order", func() {
		dropInDir := s.T().TempDir()
		s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "10-first.toml"), []byte(dumpTOML+`
			list_output = "yaml"
			read_only = true
		`), 0o644))
		s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "20-second.toml"), []byte(`
			list_output = "table"
			disable_destructive = true
		`), 0o644))

		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config-dir", dropInDir})
		s.Require().NoError(rootCmd.Execute())
		s.Contains(out.String(), `config.list_output="table"`, "list_output from 20-second.toml (last wins)")
		s.Contains(out.String(), "config.read_only=true", "read_only from 10-first.toml")
		s.Contains(out.String(), "config.disable_destructive=true", "disable_destructive from 20-second.toml")
	})
}

func TestCmd(t *testing.T) {
	suite.Run(t, new(CmdSuite))
}

func TestHelp(t *testing.T) {
	ioStreams, _ := testStream()
	rootCmd := NewMCPServer(ioStreams)
	rootCmd.SetArgs([]string{"--help"})
	o, err := captureOutput(rootCmd.Execute)
	require.NoError(t, err)
	assert.Contains(t, o, "kubernetes-mcp-server [flags]")
	assert.NotContains(t, o, "[command]")
	assert.NotContains(t, o, "[options]")
	assert.Contains(t, o, "--config")
	assert.Contains(t, o, "--config-dir")
	assert.Contains(t, o, "--version")
	assert.NotContains(t, o, "--toolsets")
	assert.NotContains(t, o, "--list-output")
	assert.NotContains(t, o, "--port")
	assert.NotContains(t, o, "--kubeconfig")
}

func TestToolsets(t *testing.T) {
	t.Run("matches default config", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML)
		require.NoError(t, err)
		expected := `config.toolsets=["` + strings.Join(config.New().Toolsets.Get(), `","`) + `"]`
		if !strings.Contains(out, expected) {
			t.Fatalf("Expected toolsets '%s', got %s %v", expected, out, err)
		}
	})
	t.Run("set from TOML", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML+`toolsets = ["helm", "config"]`+"\n")
		require.NoError(t, err)
		expected := `config\.toolsets=\["helm","config"\]`
		if m, matchErr := regexp.MatchString(expected, out); !m || matchErr != nil {
			t.Fatalf("Expected toolset to be %s, got %s %v", expected, out, err)
		}
	})
}

func TestListOutput(t *testing.T) {
	t.Run("matches default config", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML)
		require.NoError(t, err)
		defaults := config.New()
		expected := fmt.Sprintf(`config.list_output="%s"`, defaults.ListOutput.Get())
		if !strings.Contains(out, expected) {
			t.Fatalf("Expected list-output '%s', got %s %v", defaults.ListOutput.Get(), out, err)
		}
	})
	t.Run("set from TOML", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML+`list_output = "yaml"`+"\n")
		require.NoError(t, err)
		expected := `config\.list_output="yaml"`
		if m, matchErr := regexp.MatchString(expected, out); !m || matchErr != nil {
			t.Fatalf("Expected list-output to be %s, got %s %v", expected, out, err)
		}
	})
}

func TestReadOnly(t *testing.T) {
	t.Run("matches default config", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML)
		require.NoError(t, err)
		expected := fmt.Sprintf("config.read_only=%v", config.New().ReadOnly.Get())
		if !strings.Contains(out, expected) {
			t.Fatalf("Expected read-only mode %v, got %s %v", config.New().ReadOnly.Get(), out, err)
		}
	})
	t.Run("set from TOML", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML+"read_only = true\n")
		require.NoError(t, err)
		expected := `config\.read_only=true`
		if m, matchErr := regexp.MatchString(expected, out); !m || matchErr != nil {
			t.Fatalf("Expected read-only mode to be %s, got %s %v", expected, out, err)
		}
	})
}

func TestDisableDestructive(t *testing.T) {
	t.Run("matches default config", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML)
		require.NoError(t, err)
		defaults := config.New()
		expected := fmt.Sprintf("config.disable_destructive=%t", defaults.DisableDestructive.Get())
		if !strings.Contains(out, expected) {
			t.Fatalf("Expected disable destructive %t, got %s %v", defaults.DisableDestructive.Get(), out, err)
		}
	})
	t.Run("set from TOML", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML+"disable_destructive = true\n")
		require.NoError(t, err)
		expected := `config\.disable_destructive=true`
		if m, matchErr := regexp.MatchString(expected, out); !m || matchErr != nil {
			t.Fatalf("Expected disable-destructive mode to be %s, got %s %v", expected, out, err)
		}
	})
}

func TestAuthorizationURL(t *testing.T) {
	t.Run("invalid authorization_url without protocol", func(t *testing.T) {
		_, err := executeVersion(t, `
			port = "8080"
			require_oauth = true
			authorization_url = "example.com/auth"
			server_url = "https://example.com:8080"
		`)
		if err == nil {
			t.Fatal("Expected error for invalid authorization-url without protocol, got nil")
		}
		expected := "authorization_url must be a valid URL"
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("Expected error to contain %s, got %s", expected, err.Error())
		}
	})
	t.Run("valid authorization_url with https", func(t *testing.T) {
		_, err := executeVersion(t, `
			port = "8080"
			require_oauth = true
			authorization_url = "https://example.com/auth"
			server_url = "https://example.com:8080"
		`)
		if err != nil {
			t.Fatalf("Expected no error for valid https authorization-url, got %s", err.Error())
		}
	})
}

func TestStdioLogging(t *testing.T) {
	t.Run("stdio disables klog", func(t *testing.T) {
		out, err := executeVersion(t, "log_level = 1\n")
		require.NoErrorf(t, err, "Expected no error executing command, got %v", err)
		assert.Equalf(t, "0.0.0\n", out, "Expected only version output, got %s", out)
	})
	t.Run("http mode enables klog", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML)
		require.NoErrorf(t, err, "Expected no error executing command, got %v", err)
		assert.Containsf(t, out, "Starting kubernetes-mcp-server", "Expected klog output, got %s", out)
	})
}

func TestDisableMultiCluster(t *testing.T) {
	t.Run("defaults to auto-detect", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML)
		require.NoError(t, err)
		if !strings.Contains(out, `config.cluster_provider_strategy="auto-detect (it is recommended to set this explicitly in your Config)"`) {
			t.Fatalf("Expected ClusterProviderStrategy auto-detect, got %s %v", out, err)
		}
	})
	t.Run("kubeconfig path does not recommend setting strategy", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML+`kubeconfig = "/tmp/kubeconfig"`+"\n")
		require.NoError(t, err)
		if !strings.Contains(out, `config.cluster_provider_strategy="auto-detect"`) {
			t.Fatalf("Expected ClusterProviderStrategy auto-detect, got %s %v", out, err)
		}
		if strings.Contains(out, "it is recommended to set this explicitly") {
			t.Fatalf("Did not expect strategy recommendation when kubeconfig is set, got %s", out)
		}
	})
	t.Run("set cluster_provider_strategy=disabled in TOML", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML+`cluster_provider_strategy = "disabled"`+"\n")
		require.NoError(t, err)
		expected := `config\.cluster_provider_strategy="disabled"`
		if m, matchErr := regexp.MatchString(expected, out); !m || matchErr != nil {
			t.Fatalf("Expected ClusterProviderStrategy %s, got %s %v", expected, out, err)
		}
	})
}

func TestClusterProviderValidation(t *testing.T) {
	for _, strategy := range []string{"kubeconfig", "in-cluster", "disabled", "kcp"} {
		t.Run("valid cluster provider "+strategy, func(t *testing.T) {
			_, err := executeVersion(t, dumpTOML+fmt.Sprintf("cluster_provider_strategy = %q\n", strategy))
			require.NoError(t, err)
		})
	}
	t.Run("invalid cluster provider returns error", func(t *testing.T) {
		_, err := executeVersion(t, dumpTOML+`cluster_provider_strategy = "invalid-provider"`+"\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cluster_provider_strategy: invalid-provider")
		assert.Contains(t, err.Error(), "valid values are:")
	})
}

func TestStateless(t *testing.T) {
	t.Run("matches default config", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML)
		require.NoError(t, err)
		defaults := config.New()
		expected := fmt.Sprintf("config.stateless=%t", defaults.Stateless.Get())
		if !strings.Contains(out, expected) {
			t.Fatalf("Expected stateless mode %t, got %s %v", defaults.Stateless.Get(), out, err)
		}
	})
	t.Run("set from TOML", func(t *testing.T) {
		out, err := executeVersion(t, dumpTOML+"stateless = true\n")
		require.NoError(t, err)
		expected := `config\.stateless=true`
		if m, matchErr := regexp.MatchString(expected, out); !m || matchErr != nil {
			t.Fatalf("Expected stateless mode to be %s, got %s %v", expected, out, err)
		}
	})
}

func TestRequireTLSValidation(t *testing.T) {
	t.Run("require_tls without TLS certs in HTTP mode returns error", func(t *testing.T) {
		_, err := executeVersion(t, `
			port = "8080"
			require_tls = true
		`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "require_tls is enabled but TLS certificates are not configured")
	})

	t.Run("require_tls with TLS certs in HTTP mode succeeds", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath := filepath.Join(tempDir, "cert.pem")
		keyPath := filepath.Join(tempDir, "key.pem")
		require.NoError(t, os.WriteFile(certPath, []byte("cert content"), 0o644))
		require.NoError(t, os.WriteFile(keyPath, []byte("key content"), 0o644))

		_, err := executeVersion(t, fmt.Sprintf(`
			port = "8080"
			require_tls = true
			tls_cert = %q
			tls_key = %q
		`, certPath, keyPath))
		require.NoError(t, err)
	})

	t.Run("require_tls in STDIO mode does not require TLS certs", func(t *testing.T) {
		_, err := executeVersion(t, "require_tls = true\n")
		require.NoError(t, err)
	})

	t.Run("require_tls rejects HTTP authorization_url", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath := filepath.Join(tempDir, "cert.pem")
		keyPath := filepath.Join(tempDir, "key.pem")
		require.NoError(t, os.WriteFile(certPath, []byte("cert content"), 0o644))
		require.NoError(t, os.WriteFile(keyPath, []byte("key content"), 0o644))

		_, err := executeVersion(t, fmt.Sprintf(`
			port = "8080"
			require_tls = true
			tls_cert = %q
			tls_key = %q
			require_oauth = true
			authorization_url = "http://example.com/auth"
			server_url = "https://example.com:8080"
		`, certPath, keyPath))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authorization_url")
		assert.Contains(t, err.Error(), "secure scheme required")
	})

	t.Run("require_tls rejects HTTP server_url", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath := filepath.Join(tempDir, "cert.pem")
		keyPath := filepath.Join(tempDir, "key.pem")
		require.NoError(t, os.WriteFile(certPath, []byte("cert content"), 0o644))
		require.NoError(t, os.WriteFile(keyPath, []byte("key content"), 0o644))

		_, err := executeVersion(t, fmt.Sprintf(`
			port = "8080"
			require_tls = true
			tls_cert = %q
			tls_key = %q
			require_oauth = true
			authorization_url = "https://example.com/auth"
			server_url = "http://example.com:8080"
		`, certPath, keyPath))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "server_url")
		assert.Contains(t, err.Error(), "secure scheme required")
	})

	t.Run("require_tls accepts all HTTPS URLs", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath := filepath.Join(tempDir, "cert.pem")
		keyPath := filepath.Join(tempDir, "key.pem")
		require.NoError(t, os.WriteFile(certPath, []byte("cert content"), 0o644))
		require.NoError(t, os.WriteFile(keyPath, []byte("key content"), 0o644))

		_, err := executeVersion(t, fmt.Sprintf(`
			port = "8080"
			require_tls = true
			tls_cert = %q
			tls_key = %q
			require_oauth = true
			authorization_url = "https://example.com/auth"
			server_url = "https://example.com:8080"
		`, certPath, keyPath))
		require.NoError(t, err)
	})
}

func (s *CmdSuite) TestLogFile() {
	s.Run("http mode writes logs to file instead of stdout", func() {
		logPath := filepath.Join(s.T().TempDir(), "server.log")

		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config", writeTOML(s.T(), dumpTOML+fmt.Sprintf("log_file = %q\n", logPath))})
		s.Require().NoError(rootCmd.Execute())

		s.Equal("0.0.0\n", out.String(), "stdout should contain only version output, not logs")
		logContent, err := os.ReadFile(logPath)
		s.Require().NoError(err)
		s.Contains(string(logContent), "Starting kubernetes-mcp-server")
	})

	s.Run("stdio mode writes logs to file", func() {
		logPath := filepath.Join(s.T().TempDir(), "server.log")

		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config", writeTOML(s.T(), fmt.Sprintf("log_level = 1\nlog_file = %q\n", logPath))})
		s.Require().NoError(rootCmd.Execute())

		s.Equal("0.0.0\n", out.String(), "stdout should contain only version output in stdio mode")
		logContent, err := os.ReadFile(logPath)
		s.Require().NoError(err)
		s.Contains(string(logContent), "Starting kubernetes-mcp-server")
	})

	s.Run("log file is created if it does not exist", func() {
		logPath := filepath.Join(s.T().TempDir(), "new-server.log")

		_, err := os.Stat(logPath)
		s.Require().True(os.IsNotExist(err), "log file should not exist before the test")

		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config", writeTOML(s.T(), fmt.Sprintf("log_level = 1\nlog_file = %q\n", logPath))})
		s.Require().NoError(rootCmd.Execute())

		_, err = os.Stat(logPath)
		s.Require().NoError(err, "log file should have been created")
	})

	s.Run("log file is appended to if it already exists", func() {
		logPath := filepath.Join(s.T().TempDir(), "server.log")
		existingContent := "existing log line\n"
		s.Require().NoError(os.WriteFile(logPath, []byte(existingContent), 0o600))

		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config", writeTOML(s.T(), fmt.Sprintf("log_level = 1\nlog_file = %q\n", logPath))})
		s.Require().NoError(rootCmd.Execute())

		logContent, err := os.ReadFile(logPath)
		s.Require().NoError(err)
		s.True(strings.HasPrefix(string(logContent), existingContent), "existing content should be preserved at the start")
		s.Greater(len(logContent), len(existingContent), "new content should be appended after existing content")
	})

	s.Run("nonexistent parent directory returns error", func() {
		logPath := filepath.Join(s.T().TempDir(), "missing", "server.log")
		ioStreams, _ := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config", writeTOML(s.T(), fmt.Sprintf("log_file = %q\n", logPath))})
		err := rootCmd.Execute()
		s.Require().Error(err)
		s.Contains(err.Error(), "failed to open log file")
		s.Contains(err.Error(), logPath)
	})

	s.Run("log_file from TOML config is used", func() {
		logPath := filepath.Join(s.T().TempDir(), "server.log")
		configPath := filepath.Join(s.T().TempDir(), "config.toml")
		s.Require().NoError(os.WriteFile(configPath, []byte(fmt.Sprintf("port = \"1337\"\nlog_level = 1\nlog_file = %q\n", logPath)), 0o600))

		ioStreams, out := testStream()
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config", configPath})
		s.Require().NoError(rootCmd.Execute())

		s.Equal("0.0.0\n", out.String(), "stdout should not contain log output when log_file is set")
		logContent, err := os.ReadFile(logPath)
		s.Require().NoError(err)
		s.Contains(string(logContent), "Starting kubernetes-mcp-server")
	})

	s.Run("stderr routes logs to ErrOut without opening a file", func() {
		errOut := &bytes.Buffer{}
		ioStreams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &bytes.Buffer{},
			ErrOut: errOut,
		}
		rootCmd := NewMCPServer(ioStreams)
		rootCmd.SetArgs([]string{"--version", "--config", writeTOML(s.T(), "log_level = 1\nlog_file = \"stderr\"\n")})
		s.Require().NoError(rootCmd.Execute())

		s.Contains(errOut.String(), "Starting kubernetes-mcp-server", "logs should go to ErrOut")
	})
}

func TestTLSValidation(t *testing.T) {
	t.Run("tls_cert without tls_key returns error", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath := filepath.Join(tempDir, "cert.pem")
		require.NoError(t, os.WriteFile(certPath, []byte("cert content"), 0o644))

		_, err := executeVersion(t, fmt.Sprintf("port = \"8080\"\ntls_cert = %q\n", certPath))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both tls_cert and tls_key must be provided together")
	})

	t.Run("tls_key without tls_cert returns error", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := filepath.Join(tempDir, "key.pem")
		require.NoError(t, os.WriteFile(keyPath, []byte("key content"), 0o644))

		_, err := executeVersion(t, fmt.Sprintf("port = \"8080\"\ntls_key = %q\n", keyPath))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both tls_cert and tls_key must be provided together")
	})

	t.Run("invalid tls_cert path returns error", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := filepath.Join(tempDir, "key.pem")
		require.NoError(t, os.WriteFile(keyPath, []byte("key content"), 0o644))

		_, err := executeVersion(t, fmt.Sprintf("port = \"8080\"\ntls_cert = \"/nonexistent/cert.pem\"\ntls_key = %q\n", keyPath))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tls_cert must be a valid file path")
	})

	t.Run("invalid tls_key path returns error", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath := filepath.Join(tempDir, "cert.pem")
		require.NoError(t, os.WriteFile(certPath, []byte("cert content"), 0o644))

		_, err := executeVersion(t, fmt.Sprintf("port = \"8080\"\ntls_cert = %q\ntls_key = \"/nonexistent/key.pem\"\n", certPath))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tls_key must be a valid file path")
	})

	t.Run("valid tls_cert and tls_key paths succeed", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath := filepath.Join(tempDir, "cert.pem")
		keyPath := filepath.Join(tempDir, "key.pem")
		require.NoError(t, os.WriteFile(certPath, []byte("cert content"), 0o644))
		require.NoError(t, os.WriteFile(keyPath, []byte("key content"), 0o644))

		_, err := executeVersion(t, fmt.Sprintf("port = \"8080\"\ntls_cert = %q\ntls_key = %q\n", certPath, keyPath))
		require.NoError(t, err)
	})

	t.Run("tls_cert without port returns error", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath := filepath.Join(tempDir, "cert.pem")
		keyPath := filepath.Join(tempDir, "key.pem")
		require.NoError(t, os.WriteFile(certPath, []byte("cert content"), 0o644))
		require.NoError(t, os.WriteFile(keyPath, []byte("key content"), 0o644))

		_, err := executeVersion(t, fmt.Sprintf("tls_cert = %q\ntls_key = %q\n", certPath, keyPath))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tls_cert and tls_key require port to be set")
	})
}
