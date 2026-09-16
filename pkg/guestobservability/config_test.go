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
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func TestConfigValidate(t *testing.T) {
	t.Run("accepts valid HTTP Loki URL", func(t *testing.T) {
		cfg := &Config{
			Loki: LokiConfig{
				URL: "http://localhost:3100",
			},
		}

		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected valid config, got error: %v", err)
		}
	})

	t.Run("rejects missing Loki URL", func(t *testing.T) {
		cfg := &Config{}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected validation error")
		}

		if got := err.Error(); got != "loki url is required" {
			t.Fatalf("unexpected error: %s", got)
		}
	})

	t.Run("rejects invalid Loki URL", func(t *testing.T) {
		cfg := &Config{
			Loki: LokiConfig{
				URL: "not-a-url",
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("HTTPS accepts system trust without custom CA", func(t *testing.T) {
		cfg := &Config{
			Loki: LokiConfig{
				URL: "https://loki.example.com",
			},
		}

		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected valid config, got error: %v", err)
		}
	})

	t.Run("HTTPS accepts insecure mode without CA", func(t *testing.T) {
		cfg := &Config{
			Loki: LokiConfig{
				URL:      "https://loki.example.com",
				Insecure: true,
			},
		}

		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected valid config, got error: %v", err)
		}
	})

	t.Run("HTTPS accepts valid CA file", func(t *testing.T) {
		dir := t.TempDir()
		caFile := filepath.Join(dir, "ca.crt")

		certPEM := createTestCACertificatePEM(t)

		if err := os.WriteFile(caFile, certPEM, 0600); err != nil {
			t.Fatalf("failed to create CA file: %v", err)
		}

		cfg := &Config{
			Loki: LokiConfig{
				URL:                  "https://loki.example.com",
				CertificateAuthority: caFile,
			},
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected valid config, got error: %v", err)
		}
	})

	t.Run("rejects invalid CA PEM", func(t *testing.T) {
		dir := t.TempDir()
		caFile := filepath.Join(dir, "ca.crt")

		if err := os.WriteFile(
			caFile,
			[]byte("not-a-valid-pem-certificate"),
			0600,
		); err != nil {
			t.Fatalf("failed to create invalid CA file: %v", err)
		}

		cfg := &Config{
			Loki: LokiConfig{
				URL:                  "https://loki.example.com",
				CertificateAuthority: caFile,
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected invalid CA PEM validation error")
		}

		if !strings.Contains(
			err.Error(),
			"contains no valid PEM certificates",
		) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects missing CA file", func(t *testing.T) {
		cfg := &Config{
			Loki: LokiConfig{
				URL:                  "https://loki.example.com",
				CertificateAuthority: "/does/not/exist/ca.crt",
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected validation error")
		}
	})
}

func TestToolsetConfigParsing(t *testing.T) {
	t.Run("parses Loki configuration", func(t *testing.T) {
		cfg, err := config.ReadToml([]byte(`
[toolset_configs.guest-observability.loki]
url = "http://127.0.0.1:3101"
tenant = "application"
`))
		if err != nil {
			t.Fatalf("failed to parse config: %v", err)
		}

		extended, ok := cfg.GetToolsetConfig(ToolsetName)
		if !ok {
			t.Fatal("guest-observability toolset config not found")
		}

		guestCfg, ok := extended.(*Config)
		if !ok {
			t.Fatalf(
				"expected *guestobservability.Config, got %T",
				extended,
			)
		}

		if guestCfg.Loki.URL != "http://127.0.0.1:3101" {
			t.Fatalf(
				"unexpected Loki URL: %q",
				guestCfg.Loki.URL,
			)
		}

		if guestCfg.Loki.Tenant != "application" {
			t.Fatalf(
				"unexpected Loki tenant: %q",
				guestCfg.Loki.Tenant,
			)
		}
	})

	t.Run("require_tls rejects HTTP Loki endpoint", func(t *testing.T) {
		_, err := config.ReadToml([]byte(`
require_tls = true

[toolset_configs.guest-observability.loki]
url = "http://127.0.0.1:3101"
`))

		if err == nil {
			t.Fatal("expected require_tls validation error")
		}
	})

	t.Run("require_tls rejects insecure Loki configuration", func(t *testing.T) {
		_, err := config.ReadToml([]byte(`
require_tls = true

[toolset_configs.guest-observability.loki]
url = "https://loki.example.com"
insecure = true
`))

		if err == nil {
			t.Fatal("expected insecure TLS validation error")
		}
	})
}

func createTestCACertificatePEM(t *testing.T) []byte {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test CA key: %v", err)
	}

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
	if err != nil {
		t.Fatalf("failed to create test CA certificate: %v", err)
	}

	return pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: derBytes,
		},
	)
}
