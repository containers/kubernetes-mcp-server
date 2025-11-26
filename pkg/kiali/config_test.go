package kiali

import (
	"os"
	"path/filepath"
	"testing"

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
	caContent := `-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAKL7YQ+O2UE3MA0GCSqGSIb3DQEBCQUAMEUxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQwHhcNMTIwOTEyMjE1MjAyWhcNMTUwOTEyMjE1MjAyWjBF
MQswCQYDVQQGEwJBVTETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50
ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
CgKCAQEAuMPNS1Ufof9EW/M98FNwUAKrwflsqVxaxQjBQnHQmiW7XVtyZPSz6vYh
uMiMvAoI8f+2W7bzbU9pLt0SjhBZtq3SMlm6n5Q6DgF3u1ZgE3X/dmBhWanod1x
p7Dv+0T1A2vz64qF7w7f4LdA8OZ6e+pl6UPlyu3+mCPOJqlt8bLE5Dk4StaXVPFZ
Sok2NOyymevQVnyB9v0UCFl5E4KfRmTdtgA3sFniF1P0jXZx5lsp3nMf4YYmv/0
rIXnyblYbiHRwnXCS2YONPQkcQnjkXKPSXIBKnMa5ZBi5TcxUcOLE0Vhz4rNU1hg
NSfN29kbNMks2f1OP+VDHnCIzW8BavL7iPmv5C6M97U6UfYzLrQ0O8Gcydk4wsy
9eORv6mdNSPuSb8qTkIDG5T5SHSgPl8F3y7fY2BqXdcXdeJf3QIDAQABo1AwTjAd
BgNVHQ4EFgQUo5A2tIuSbcA1Aec41vELGW/ag54wHwYDVR0jBBgwFoAUo5A2tIuS
bcA1Aec41vELGW/ag54wDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQUFAAOCAQEA
k4LleEW5M4H4GmRlQTs8E3NYOdvJVE3EbsDNfhRghoWThg2y2pzSn3pGPqYzTBH/
c320GOMHQ4jf4nbT5eE5yt5oUq9sJ9A132K0YI7HLSITedXOW3U4p2qY5S29JMW
bJ1x75c5S3zNTf0CphQRY5yqgy4S7y2N1M6o3TlU1pe3y7hqOXE9z5SsImaj8xZ
fXHqX5u1y5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q
5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q
5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q5q
-----END CERTIFICATE-----`
	s.caFile = filepath.Join(s.tempDir, "ca.crt")
	err = os.WriteFile(s.caFile, []byte(caContent), 0644)
	s.Require().NoError(err, "Failed to write CA file")
}

func (s *ConfigSuite) TestConfigParser_ResolvesRelativePath() {
	// Create a config file in the temp directory
	configFile := filepath.Join(s.tempDir, "config.toml")
	configContent := `
[toolset_configs.kiali]
url = "https://kiali.example/"
certificate_authority = "ca.crt"
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	s.Require().NoError(err, "Failed to write config file")

	// Read config - Read() automatically sets the config directory path
	cfg, err := config.Read(configFile)
	s.Require().NoError(err, "Failed to read config")

	// Get Kiali config
	kialiCfg, ok := cfg.GetToolsetConfig("kiali")
	s.Require().True(ok, "Kiali config should be present")
	kcfg, ok := kialiCfg.(*Config)
	s.Require().True(ok, "Kiali config should be of type *Config")

	// Verify the path was resolved to absolute
	expectedPath := s.caFile
	s.Equal(expectedPath, kcfg.CertificateAuthority, "Relative path should be resolved to absolute path")
}

func (s *ConfigSuite) TestConfigParser_PreservesAbsolutePath() {
	// Create a config file with absolute path
	configFile := filepath.Join(s.tempDir, "config.toml")
	// Convert backslashes to forward slashes for TOML compatibility on Windows
	caFileForTOML := filepath.ToSlash(s.caFile)
	configContent := `
[toolset_configs.kiali]
url = "https://kiali.example/"
certificate_authority = "` + caFileForTOML + `"
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	s.Require().NoError(err, "Failed to write config file")

	// Read config - Read() automatically sets the config directory path
	cfg, err := config.Read(configFile)
	s.Require().NoError(err, "Failed to read config")

	kialiCfg, ok := cfg.GetToolsetConfig("kiali")
	s.Require().True(ok, "Kiali config should be present")
	kcfg, ok := kialiCfg.(*Config)
	s.Require().True(ok, "Kiali config should be of type *Config")

	// Absolute path should be preserved
	actualPath := filepath.Clean(filepath.FromSlash(kcfg.CertificateAuthority))
	expectedPath := filepath.Clean(s.caFile)
	s.Equal(expectedPath, actualPath, "Absolute path should be preserved")
}

func (s *ConfigSuite) TestConfigParser_RejectsInlinePEM() {
	inlinePEM := `-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAKL7YQ+O2UE3MA0GCSqGSIb3DQEBCQUAMEUxCzAJBgNV
-----END CERTIFICATE-----`
	// Create a config file with inline PEM
	configFile := filepath.Join(s.tempDir, "config.toml")
	configContent := `
[toolset_configs.kiali]
url = "https://kiali.example/"
certificate_authority = """` + inlinePEM + `"""
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	s.Require().NoError(err, "Failed to write config file")

	cfg, err := config.Read(configFile)

	// Parser should reject inline PEM content during parsing
	s.Require().Error(err, "Parser should reject inline PEM content")
	s.Contains(err.Error(), "certificate_authority must be a file path, not inline PEM content", "Error message should indicate inline PEM is not allowed")
	s.Nil(cfg, "Config should be nil when parsing fails")
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
