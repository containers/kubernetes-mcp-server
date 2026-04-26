package kiali

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
	tempDir string
	caFile  string
}

func (s *ConfigSuite) SetupTest() {
	// Create a test CA certificate file
	s.tempDir = s.T().TempDir()
	s.caFile = filepath.Join(s.tempDir, "ca.crt")
	err := os.WriteFile(s.caFile, []byte("test ca content"), 0644)
	s.Require().NoError(err, "Failed to write CA file")
}

func (s *ConfigSuite) TestConfigParser_ResolvesRelativePath() {
	// Read config with configDirPath set to tempDir to resolve relative paths
	cfg := test.Must(config.ReadToml([]byte(`
		[toolset_configs.kiali]
		url = "https://kiali.example/"
		certificate_authority = "ca.crt"
	`), config.WithDirPath(s.tempDir)))

	// Get Kiali config
	kialiCfg, ok := cfg.GetToolsetConfig("kiali")
	s.Require().True(ok, "Kiali config should be present")
	kcfg, ok := kialiCfg.(*Config)
	s.Require().True(ok, "Kiali config should be of type *Config")

	// Verify the path was resolved to absolute
	s.Equal(s.caFile, kcfg.CertificateAuthority, "Relative path should be resolved to absolute path")
}

func (s *ConfigSuite) TestConfigParser_PreservesAbsolutePath() {
	// Convert backslashes to forward slashes for TOML compatibility on Windows
	caFileForTOML := filepath.ToSlash(s.caFile)

	cfg := test.Must(config.ReadToml([]byte(`
		[toolset_configs.kiali]
		url = "https://kiali.example/"
		certificate_authority = "` + caFileForTOML + `"
	`)))

	kialiCfg, ok := cfg.GetToolsetConfig("kiali")
	s.Require().True(ok, "Kiali config should be present")
	kcfg, ok := kialiCfg.(*Config)
	s.Require().True(ok, "Kiali config should be of type *Config")

	// Absolute path should be preserved
	actualPath := filepath.Clean(filepath.FromSlash(kcfg.CertificateAuthority))
	expectedPath := filepath.Clean(s.caFile)
	s.Equal(expectedPath, actualPath, "Absolute path should be preserved")
}

func (s *ConfigSuite) TestConfigParser_RejectsInvalidFile() {
	// Use a non-existent file path
	nonExistentFile := filepath.Join(s.tempDir, "non-existent.crt")
	// Convert backslashes to forward slashes for TOML compatibility on Windows
	nonExistentFileForTOML := filepath.ToSlash(nonExistentFile)

	cfg, err := config.ReadToml([]byte(`
		[toolset_configs.kiali]
		url = "https://kiali.example/"
		certificate_authority = "` + nonExistentFileForTOML + `"
	`))

	// Validate should reject invalid file path
	s.Require().Error(err, "Validate should reject invalid file path")
	s.Contains(err.Error(), "certificate_authority must be a valid file path", "Error message should indicate file path is invalid")
	s.Nil(cfg, "Config should be nil when validation fails")
}

func (s *ConfigSuite) TestConfigParser_RejectsInsecureWithRequireTLS() {
	caFileForTOML := filepath.ToSlash(s.caFile)

	_, err := config.ReadToml([]byte(`
		require_tls = true
		[toolset_configs.kiali]
		url = "https://kiali.example/"
		insecure = true
		certificate_authority = "` + caFileForTOML + `"
	`))

	s.Require().Error(err)
	s.Contains(err.Error(), "insecure=true disables certificate verification")
}

func (s *ConfigSuite) TestConfigParser_AllowsSecureWithRequireTLS() {
	caFileForTOML := filepath.ToSlash(s.caFile)

	cfg, err := config.ReadToml([]byte(`
		require_tls = true
		[toolset_configs.kiali]
		url = "https://kiali.example/"
		insecure = false
		certificate_authority = "` + caFileForTOML + `"
	`))

	s.Require().NoError(err)
	kialiCfg, ok := cfg.GetToolsetConfig("kiali")
	s.Require().True(ok)
	kcfg, ok := kialiCfg.(*Config)
	s.Require().True(ok)
	s.False(kcfg.Insecure)
}

func (s *ConfigSuite) TestValidate() {
	s.Run("nil config returns error", func() {
		var cfg *Config
		err := cfg.Validate()
		s.Error(err, "Expected error for nil config")
		s.ErrorContains(err, "kiali config is nil")
	})
	s.Run("empty URL returns error", func() {
		cfg := &Config{}
		err := cfg.Validate()
		s.Error(err, "Expected error for empty URL")
		s.ErrorContains(err, "url is required")
	})
	s.Run("invalid URL returns error", func() {
		cfg := &Config{Url: "://bad-url"}
		err := cfg.Validate()
		s.Error(err, "Expected error for invalid URL")
		s.ErrorContains(err, "url must be a valid URL")
	})
	s.Run("URL without scheme returns error", func() {
		cfg := &Config{Url: "just-a-hostname"}
		err := cfg.Validate()
		s.Error(err, "Expected error for URL without scheme")
		s.ErrorContains(err, "url must be a valid URL")
	})
	s.Run("HTTP URL does not require certificate_authority", func() {
		cfg := &Config{Url: "http://kiali.example/"}
		err := cfg.Validate()
		s.NoError(err, "HTTP URL should not require certificate_authority")
	})
	s.Run("HTTPS with insecure=true does not require certificate_authority", func() {
		cfg := &Config{Url: "https://kiali.example/", Insecure: true}
		err := cfg.Validate()
		s.NoError(err, "HTTPS with insecure=true should not require certificate_authority")
	})
	s.Run("HTTPS with insecure=false requires certificate_authority", func() {
		cfg := &Config{Url: "https://kiali.example/", Insecure: false}
		err := cfg.Validate()
		s.Error(err, "Expected error for HTTPS without cert when not insecure")
		s.ErrorContains(err, "certificate_authority is required for https when insecure is false")
	})
	s.Run("HTTPS with insecure=false and valid certificate_authority passes", func() {
		cfg := &Config{
			Url:                  "https://kiali.example/",
			CertificateAuthority: s.caFile,
		}
		err := cfg.Validate()
		s.NoError(err, "HTTPS with valid certificate_authority should pass validation")
	})
}

func (s *ConfigSuite) TestConfigParser_HTTPUrl_NoCertRequired() {
	cfg, err := config.ReadToml([]byte(`
		[toolset_configs.kiali]
		url = "http://kiali.example/"
	`))
	s.NoError(err, "HTTP URL should not require certificate_authority")
	s.NotNil(cfg, "Config should not be nil for valid HTTP URL")
}

func (s *ConfigSuite) TestConfigParser_NoCertificateAuthority() {
	cfg := test.Must(config.ReadToml([]byte(`
		[toolset_configs.kiali]
		url = "http://kiali.example/"
	`)))

	kialiCfg, ok := cfg.GetToolsetConfig("kiali")
	s.Require().True(ok, "Kiali config should be present")
	kcfg, ok := kialiCfg.(*Config)
	s.Require().True(ok, "Kiali config should be of type *Config")
	s.Empty(kcfg.CertificateAuthority, "certificate_authority should be empty when not provided")
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
