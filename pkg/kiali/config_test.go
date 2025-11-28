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
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "kiali-config-test-*")
	s.Require().NoError(err, "Failed to create temp directory")
	s.tempDir = tempDir

	// Create a test CA certificate file
	s.caFile = filepath.Join(s.tempDir, "ca.crt")
	err = os.WriteFile(s.caFile, []byte("test ca content"), 0644)
	s.Require().NoError(err, "Failed to write CA file")
}

func (s *ConfigSuite) TestConfigParser_ResolvesRelativePath() {
	// Create CA file in temp directory
	caFile := filepath.Join(s.tempDir, "ca.crt")
	err := os.WriteFile(caFile, []byte("test ca content"), 0644)
	s.Require().NoError(err, "Failed to write CA file")

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
	expectedPath := caFile
	s.Equal(expectedPath, kcfg.CertificateAuthority, "Relative path should be resolved to absolute path")
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

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
