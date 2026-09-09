package mcp

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/stretchr/testify/suite"
)

// brokenToolset is a fake api.Toolset whose single tool has a non-object
// input schema. ServerToolToGoSdkTool rejects this in the convert phase
// ("input schema must have type \"object\""), which is the only failure
// mode that exercises applyToolsets's transactional swap path —
// config-level errors fail earlier, in Validate. We can't simply use
// InputSchema: nil because the WithTargetParameter mutator initializes a
// nil schema to type=object for cluster-aware tools.
type brokenToolset struct{}

func (brokenToolset) GetName() string        { return "broken-test-toolset" }
func (brokenToolset) GetDescription() string { return "test-only toolset that fails convert phase" }
func (brokenToolset) GetTools(api.FilteringProvider) []api.ServerTool {
	return []api.ServerTool{{Tool: api.Tool{
		Name:        "broken-tool",
		InputSchema: &jsonschema.Schema{Type: "string"},
	}}}
}
func (brokenToolset) GetPrompts() []api.ServerPrompt                     { return nil }
func (brokenToolset) GetResources() []api.ServerResource                 { return nil }
func (brokenToolset) GetResourceTemplates() []api.ServerResourceTemplate { return nil }

type ConfigReloadSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
	server     *Server
}

func (s *ConfigReloadSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig.SetForTest(s.mockServer.KubeconfigFile(s.T()))
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
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(s.T().Context(), Configuration{
		Config: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.Require().NotNil(server)
	s.server = server

	s.Run("initial configuration loaded correctly", func() {
		s.Equal(s.Cfg.LogLevel.Get(), server.configuration.Load().LogLevel.Get())
		s.Equal(s.Cfg.ListOutput.Get(), server.configuration.Load().Config.ListOutput.Get())
		s.Equal(s.Cfg.Toolsets.Get(), server.configuration.Load().Config.Toolsets.Get())
	})

	s.Run("reload with new log level", func() {
		newConfig := config.New()
		newConfig.LogLevel.SetForTest(5)
		newConfig.ListOutput.SetForTest("yaml")
		newConfig.Toolsets.SetForTest([]string{"core", "config"})
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())

		err = server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().NoError(err)

		s.Equal(5, server.configuration.Load().LogLevel.Get())
		s.Equal("yaml", server.configuration.Load().Config.ListOutput.Get())
		s.Equal([]string{"core", "config"}, server.configuration.Load().Config.Toolsets.Get())
	})

	s.Run("reload with additional toolsets", func() {
		newConfig := config.New()
		newConfig.LogLevel.SetForTest(5)
		newConfig.ListOutput.SetForTest("yaml")
		newConfig.Toolsets.SetForTest([]string{"core", "config", "helm"})
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())

		err = server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().NoError(err)

		s.Equal(5, server.configuration.Load().LogLevel.Get())
		s.Equal("yaml", server.configuration.Load().Config.ListOutput.Get())
		s.Equal([]string{"core", "config", "helm"}, server.configuration.Load().Config.Toolsets.Get())
	})

	s.Run("reload with partial changes", func() {
		newConfig := config.New()
		newConfig.LogLevel.SetForTest(7)
		newConfig.ListOutput.SetForTest("yaml")
		newConfig.Toolsets.SetForTest([]string{"core", "config", "helm"})
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())

		err = server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().NoError(err)

		s.Equal(7, server.configuration.Load().LogLevel.Get())
		s.Equal("yaml", server.configuration.Load().Config.ListOutput.Get())
		s.Equal([]string{"core", "config", "helm"}, server.configuration.Load().Config.Toolsets.Get())
	})

	s.Run("reload back to defaults", func() {
		newConfig := config.New()
		newConfig.LogLevel.SetForTest(0)
		newConfig.ListOutput.SetForTest("table")
		newConfig.Toolsets.SetForTest([]string{"core", "config"})
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())

		err = server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().NoError(err)

		s.Equal(0, server.configuration.Load().LogLevel.Get())
		s.Equal("table", server.configuration.Load().Config.ListOutput.Get())
		s.Equal([]string{"core", "config"}, server.configuration.Load().Config.Toolsets.Get())
	})
}

func (s *ConfigReloadSuite) TestConfigurationValues() {
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(s.T().Context(), Configuration{
		Config: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.server = server

	s.Run("reload updates configuration values", func() {
		// Verify initial values
		initialLogLevel := server.configuration.Load().LogLevel.Get()

		newConfig := config.New()
		newConfig.LogLevel.SetForTest(9)
		newConfig.ListOutput.SetForTest("yaml")
		newConfig.Toolsets.SetForTest([]string{"core", "config", "helm"})
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())

		err = server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().NoError(err)

		// Verify configuration was updated
		s.NotEqual(initialLogLevel, server.configuration.Load().LogLevel.Get())
		s.Equal(9, server.configuration.Load().LogLevel.Get())
		s.Equal([]string{"core", "config", "helm"}, server.configuration.Load().Config.Toolsets.Get())
		s.Equal("yaml", server.configuration.Load().Config.ListOutput.Get())
	})
}

func (s *ConfigReloadSuite) TestMultipleReloads() {
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(s.T().Context(), Configuration{
		Config: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.server = server

	s.Run("multiple reloads in succession", func() {
		// First reload
		cfg1 := config.New()
		cfg1.LogLevel.SetForTest(3)
		cfg1.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		cfg1.Toolsets.SetForTest([]string{"core"})
		err = server.ReloadConfiguration(s.T().Context(), cfg1)
		s.Require().NoError(err)
		s.Equal(3, server.configuration.Load().LogLevel.Get())

		// Second reload
		cfg2 := config.New()
		cfg2.LogLevel.SetForTest(6)
		cfg2.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		cfg2.Toolsets.SetForTest([]string{"core", "config"})
		err = server.ReloadConfiguration(s.T().Context(), cfg2)
		s.Require().NoError(err)
		s.Equal(6, server.configuration.Load().LogLevel.Get())

		// Third reload
		cfg3 := config.New()
		cfg3.LogLevel.SetForTest(9)
		cfg3.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		cfg3.Toolsets.SetForTest([]string{"core", "config", "helm"})
		err = server.ReloadConfiguration(s.T().Context(), cfg3)
		s.Require().NoError(err)
		s.Equal(9, server.configuration.Load().LogLevel.Get())
	})
}

func (s *ConfigReloadSuite) TestReloadUpdatesToolsets() {
	// Get initial tools
	s.InitMcpClient()
	initialTools, err := s.ListTools()
	s.Require().NoError(err)
	s.Require().Greater(len(initialTools.Tools), 0)

	// Add helm toolset via reload
	newConfig := config.New()
	newConfig.Toolsets.SetForTest([]string{"core", "config", "helm"})
	newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get(

	// Reload configuration on the server the MCP client is connected to
	))

	err = s.mcpServer.ReloadConfiguration(s.T().Context(), newConfig)
	s.Require().NoError(err)

	// Verify helm tools are available
	reloadedTools, err := s.ListTools()
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

func (s *ConfigReloadSuite) TestReloadRejectsHTTPURLsWhenRequireTLS() {
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(s.T().Context(), Configuration{
		Config: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.server = server

	s.Run("reload with require_tls and HTTP authorization_url is rejected", func() {
		newConfig := config.New()
		newConfig.Port.SetForTest("8080")
		newConfig.RequireOAuth.SetForTest(true)
		newConfig.RequireTLS.SetForTest(true)
		newConfig.AuthorizationURL.SetForTest("http://example.com/auth")
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().Error(err)
		s.Contains(err.Error(), "authorization_url")
		s.Contains(err.Error(), "secure scheme required")
	})

	s.Run("reload with require_tls and HTTP server_url is rejected", func() {
		newConfig := config.New()
		newConfig.Port.SetForTest("8080")
		newConfig.RequireOAuth.SetForTest(true)
		newConfig.RequireTLS.SetForTest(true)
		newConfig.AuthorizationURL.SetForTest("https://example.com/auth")
		newConfig.ServerURL.SetForTest("http://example.com:8080")
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().Error(err)
		s.Contains(err.Error(), "server_url")
		s.Contains(err.Error(), "secure scheme required")
	})

	s.Run("reload with require_tls and HTTPS URLs succeeds", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0o644))
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0o644))

		newConfig := config.New()
		newConfig.Port.SetForTest("8080")
		newConfig.RequireOAuth.SetForTest(true)
		newConfig.RequireTLS.SetForTest(true)
		newConfig.TLSCert.SetForTest(certPath)
		newConfig.TLSKey.SetForTest(keyPath)
		newConfig.AuthorizationURL.SetForTest("https://example.com/auth")
		newConfig.ServerURL.SetForTest("https://example.com:8080")
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.NoError(err)
	})

	s.Run("reload without require_tls allows HTTP URLs", func() {
		newConfig := config.New()
		newConfig.Port.SetForTest("8080")
		newConfig.RequireOAuth.SetForTest(true)
		newConfig.RequireTLS.SetForTest(false)
		newConfig.AuthorizationURL.SetForTest("http://example.com/auth")
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.NoError(err)
	})
}

func (s *ConfigReloadSuite) TestReloadRejectsInvalidConfig() {
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(s.T().Context(), Configuration{
		Config: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.server = server

	s.Run("reload with invalid list_output is rejected", func() {
		newConfig := config.New()
		newConfig.ListOutput.SetForTest("invalid-format")
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid output name")
	})

	s.Run("reload with invalid toolset name is rejected", func() {
		newConfig := config.New()
		newConfig.Toolsets.SetForTest([]string{"nonexistent-toolset"})
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid toolset name")
	})

	s.Run("reload with invalid cluster_provider_strategy is rejected", func() {
		newConfig := config.New()
		newConfig.ClusterProviderStrategy.SetForTest("nonexistent-strategy")
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid cluster_provider_strategy")
	})

	s.Run("reload with invalid authorization_url scheme is rejected", func() {
		newConfig := config.New()
		newConfig.Port.SetForTest("8080")
		newConfig.RequireOAuth.SetForTest(true)
		newConfig.AuthorizationURL.SetForTest("ftp://example.com/auth")
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().Error(err)
		s.Contains(err.Error(), "authorization_url must be a valid URL")
	})

	s.Run("reload with non-existent certificate_authority is rejected", func() {
		newConfig := config.New()
		newConfig.Port.SetForTest("8080")
		newConfig.RequireOAuth.SetForTest(true)
		newConfig.AuthorizationURL.SetForTest("https://example.com/auth")
		newConfig.CertificateAuthority.SetForTest("/nonexistent/path/ca.crt")
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().Error(err)
		s.Contains(err.Error(), "certificate_authority must be a valid file path")
	})

	s.Run("reload with mismatched tls_cert and tls_key is rejected", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0o644))
		newConfig := config.New()
		newConfig.TLSCert.SetForTest(certPath)
		newConfig.TLSKey.SetForTest("")
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().Error(err)
		s.Contains(err.Error(), "both tls_cert and tls_key must be provided together")
	})

	s.Run("reload with require_oauth without authorization_url and skip_jwt_verification=false is rejected", func() {
		newConfig := config.New()
		newConfig.Port.SetForTest("8080")
		newConfig.RequireOAuth.SetForTest(true)
		newConfig.AuthorizationURL.SetForTest("")
		newConfig.SkipJWTVerification.SetForTest(false)
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_oauth is enabled but authorization_url is not configured")
	})

	s.Run("reload with require_oauth without authorization_url and skip_jwt_verification=true is accepted", func() {
		newConfig := config.New()
		newConfig.Port.SetForTest("8080")
		newConfig.RequireOAuth.SetForTest(true)
		newConfig.AuthorizationURL.SetForTest("")
		newConfig.SkipJWTVerification.SetForTest(true)
		newConfig.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		err := server.ReloadConfiguration(s.T().Context(), newConfig)
		s.NoError(err)
	})
}

// TestReloadFailureLeavesConfigurationIntact is the regression for issue
// #1128: a reload whose convert phase fails must leave s.configuration, the
// SDK surface, and the enabled-X bookkeeping all at their pre-reload values.
// We trigger the failure via brokenToolset (a tool with a non-object input
// schema is rejected by ServerToolToGoSdkTool) and call applyToolsets
// directly so we exercise the transactional swap rather than the Validate
// fast-path.
func (s *ConfigReloadSuite) TestReloadFailureLeavesConfigurationIntact() {
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(s.T().Context(), Configuration{
		Config: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.server = server

	prevConfig := server.configuration.Load()
	prevEnabledTools := server.GetEnabledTools()
	prevEnabledPrompts := server.GetEnabledPrompts()
	prevEnabledResources := server.GetEnabledResources()
	prevEnabledResourceTemplates := server.GetEnabledResourceTemplates()
	s.Require().NotEmpty(prevEnabledTools, "baseline must have some enabled tools to be a meaningful regression target")

	// Build a candidate Configuration that bypasses Config.Toolsets
	// resolver and goes straight to the broken toolset, so collectApplicable*
	// returns the bad tool and the convert phase fails.
	candidate := config.New()
	candidate.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
	candidate.ReadOnly.SetForTest(s.Cfg.ReadOnly.Get())
	candidateCfg := &Configuration{
		Config:   candidate,
		toolsets: []api.Toolset{brokenToolset{}},
	}

	s.Run("convert-phase failure does not mutate s.configuration", func() {
		err := server.applyToolsets(s.T().Context(), candidateCfg)
		s.Require().Error(err, "reload must fail when a tool has a non-object input schema")

		s.Same(prevConfig, server.configuration.Load(),
			"s.configuration pointer must be unchanged after a rejected reload")
		s.Equal(prevEnabledTools, server.GetEnabledTools(),
			"enabledTools must be unchanged after a rejected reload")
		s.Equal(prevEnabledPrompts, server.GetEnabledPrompts(),
			"enabledPrompts must be unchanged after a rejected reload")
		s.Equal(prevEnabledResources, server.GetEnabledResources(),
			"enabledResources must be unchanged after a rejected reload")
		s.Equal(prevEnabledResourceTemplates, server.GetEnabledResourceTemplates(),
			"enabledResourceTemplates must be unchanged after a rejected reload")
	})

	s.Run("a subsequent successful re-apply still works", func() {
		// Confirms the failed swap didn't leave reloadMu/mu in a bad state
		// or corrupt the existing SDK surface.
		s.Require().NoError(server.reapplyToolsets())
		s.Equal(prevEnabledTools, server.GetEnabledTools())
	})
}

// TestConcurrentReadsDuringReload runs many reader goroutines that exercise
// the same s.configuration access pattern handlers use (Load() + read fields
// off the snapshot), in parallel with a writer goroutine that calls
// ReloadConfiguration repeatedly. With the field stored as a plain pointer
// guarded only by the now-unused-by-handlers s.mu, `go test -race` would
// report a data race on the field. With atomic.Pointer it is race-free.
func (s *ConfigReloadSuite) TestConcurrentReadsDuringReload() {
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(s.T().Context(), Configuration{
		Config: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.server = server

	stop := make(chan struct{})
	var observedReads atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				cfg := server.configuration.Load()
				_ = cfg.HTTP.RateLimitRPS.Get()
				_ = cfg.Stateless.Get()
				_ = cfg.LogLevel.Get()
				observedReads.Add(1)
			}
		}()
	}

	deadline := time.After(500 * time.Millisecond)
	toggle := false
	for {
		select {
		case <-deadline:
			close(stop)
			wg.Wait()
			s.Greater(observedReads.Load(), int64(0), "readers must have run")
			return
		default:
		}
		newCfg := config.New()
		newCfg.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		if toggle {
			newCfg.LogLevel.SetForTest(9)
		} else {
			newCfg.LogLevel.SetForTest(1)
		}
		toggle = !toggle
		s.Require().NoError(server.ReloadConfiguration(s.T().Context(), newCfg))
	}
}

// TestConcurrentListOutputAfterReload exercises the lazy ListOutput cache
// race: after a successful reload, several handlers reading
// cfg.ListOutput() concurrently for the first time would each write the
// cache field unsynchronized. With the cache pre-warmed by warmCaches in
// applyToolsets before publish, the first-read writes are gone and
// `-race` stays clean.
func (s *ConfigReloadSuite) TestConcurrentListOutputAfterReload() {
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(s.T().Context(), Configuration{
		Config: s.Cfg,
	}, provider)
	s.Require().NoError(err)
	s.server = server

	for iter := 0; iter < 5; iter++ {
		newCfg := config.New()
		newCfg.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		if iter%2 == 0 {
			newCfg.ListOutput.SetForTest("yaml")
		} else {
			newCfg.ListOutput.SetForTest("table")
		}
		s.Require().NoError(server.ReloadConfiguration(s.T().Context(), newCfg))

		var wg sync.WaitGroup
		for r := 0; r < 16; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cfg := server.configuration.Load()
				_ = cfg.ListOutput()
				_ = cfg.Toolsets()
			}()
		}
		wg.Wait()
	}
}

func (s *ConfigReloadSuite) TestServerLifecycle() {
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err)
	server, err := NewServer(s.T().Context(), Configuration{
		Config: s.Cfg,
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
