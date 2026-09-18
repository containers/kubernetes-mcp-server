package guestobservability

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
}

func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}

func (s *ConfigSuite) TestConfigValidate() {
	s.Run("valid configurations", func() {
		s.Run("accepts valid HTTP Loki URL", func() {
			cfg := &Config{
				Loki: LokiConfig{
					URL: "http://localhost:3100",
				},
			}

			s.NoError(
				cfg.Validate(),
				"expected valid HTTP Loki configuration",
			)
		})

		s.Run("HTTPS accepts system trust without custom CA", func() {
			cfg := &Config{
				Loki: LokiConfig{
					URL: "https://loki.example.com",
				},
			}

			s.NoError(
				cfg.Validate(),
				"expected HTTPS configuration using system trust to be valid",
			)
		})

		s.Run("HTTPS accepts insecure mode without CA", func() {
			cfg := &Config{
				Loki: LokiConfig{
					URL:      "https://loki.example.com",
					Insecure: true,
				},
			}

			s.NoError(
				cfg.Validate(),
				"expected insecure HTTPS configuration to be valid",
			)
		})

		s.Run("HTTPS accepts valid CA file", func() {
			dir := s.T().TempDir()
			caFile := filepath.Join(dir, "ca.crt")
			certPEM := createTestCACertificatePEM(s.T())

			s.Require().NoError(
				os.WriteFile(caFile, certPEM, 0600),
				"failed to create CA file",
			)

			cfg := &Config{
				Loki: LokiConfig{
					URL:                  "https://loki.example.com",
					CertificateAuthority: caFile,
				},
			}

			s.NoError(
				cfg.Validate(),
				"expected HTTPS configuration with custom CA to be valid",
			)
		})
	})

	s.Run("invalid configurations", func() {
		s.Run("rejects missing Loki URL", func() {
			cfg := &Config{}

			err := cfg.Validate()

			s.Require().Error(err, "expected validation error")
			s.Equal(
				"loki url is required",
				err.Error(),
				"unexpected validation error",
			)
		})

		s.Run("rejects invalid Loki URL", func() {
			cfg := &Config{
				Loki: LokiConfig{
					URL: "not-a-url",
				},
			}

			s.Error(
				cfg.Validate(),
				"expected invalid Loki URL validation error",
			)
		})

		s.Run("rejects invalid CA PEM", func() {
			dir := s.T().TempDir()
			caFile := filepath.Join(dir, "ca.crt")

			s.Require().NoError(
				os.WriteFile(
					caFile,
					[]byte("not-a-valid-pem-certificate"),
					0600,
				),
				"failed to create invalid CA file",
			)

			cfg := &Config{
				Loki: LokiConfig{
					URL:                  "https://loki.example.com",
					CertificateAuthority: caFile,
				},
			}

			err := cfg.Validate()

			s.Require().Error(
				err,
				"expected invalid CA PEM validation error",
			)
			s.Contains(
				err.Error(),
				"contains no valid PEM certificates",
				"unexpected validation error",
			)
		})

		s.Run("rejects missing CA file", func() {
			cfg := &Config{
				Loki: LokiConfig{
					URL:                  "https://loki.example.com",
					CertificateAuthority: "/does/not/exist/ca.crt",
				},
			}

			s.Error(
				cfg.Validate(),
				"expected missing CA file validation error",
			)
		})
	})
}

func (s *ConfigSuite) TestToolsetConfigParsing() {
	s.Run("valid configuration", func() {
		s.Run("parses Loki configuration", func() {
			cfg, err := config.ReadToml([]byte(`
[toolset_configs.guest-observability.loki]
url = "http://127.0.0.1:3101"
tenant = "application"
`))
			s.Require().NoError(
				err,
				"failed to parse guest-observability configuration",
			)

			extended, ok := cfg.GetToolsetConfig(ToolsetName)
			s.Require().True(
				ok,
				"guest-observability toolset config not found",
			)

			guestCfg, ok := extended.(*Config)
			s.Require().True(
				ok,
				"expected *guestobservability.Config, got %T",
				extended,
			)

			s.Equal(
				"http://127.0.0.1:3101",
				guestCfg.Loki.URL,
				"unexpected Loki URL",
			)
			s.Equal(
				"application",
				guestCfg.Loki.Tenant,
				"unexpected Loki tenant",
			)
		})
	})

	s.Run("TLS validation", func() {
		s.Run("require_tls rejects HTTP Loki endpoint", func() {
			_, err := config.ReadToml([]byte(`
require_tls = true

[toolset_configs.guest-observability.loki]
url = "http://127.0.0.1:3101"
`))

			s.Error(
				err,
				"expected require_tls validation error",
			)
		})

		s.Run("require_tls rejects insecure Loki configuration", func() {
			_, err := config.ReadToml([]byte(`
require_tls = true

[toolset_configs.guest-observability.loki]
url = "https://loki.example.com"
insecure = true
`))

			s.Error(
				err,
				"expected insecure TLS validation error",
			)
		})
	})
}

func createTestCACertificatePEM(t *testing.T) []byte {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(
		t,
		err,
		"failed to generate test CA key",
	)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "guest-observability-test-ca",
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&privateKey.PublicKey,
		privateKey,
	)
	require.NoError(
		t,
		err,
		"failed to create test CA certificate",
	)

	return pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: derBytes,
		},
	)
}
