package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"

	// Blank imports to register toolsets and providers in their respective registries.
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
)

type ValidateSuite struct {
	suite.Suite
}

func (s *ValidateSuite) validConfig() *config.StaticConfig {
	cfg := config.BaseDefault()
	return cfg
}

func (s *ValidateSuite) TestValidDefaultConfig() {
	s.Run("default config passes validation", func() {
		cfg := s.validConfig()
		s.NoError(cfg.Validate())
	})
}

func (s *ValidateSuite) TestListOutput() {
	s.Run("invalid list_output is rejected", func() {
		cfg := s.validConfig()
		cfg.ListOutput = "invalid-format"
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid output name")
		s.Contains(err.Error(), "invalid-format")
	})

	s.Run("empty list_output is rejected", func() {
		cfg := s.validConfig()
		cfg.ListOutput = ""
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid output name")
	})

	s.Run("yaml list_output is accepted", func() {
		cfg := s.validConfig()
		cfg.ListOutput = "yaml"
		s.NoError(cfg.Validate())
	})

	s.Run("table list_output is accepted", func() {
		cfg := s.validConfig()
		cfg.ListOutput = "table"
		s.NoError(cfg.Validate())
	})
}

func (s *ValidateSuite) TestToolsets() {
	s.Run("invalid toolset name is rejected", func() {
		cfg := s.validConfig()
		cfg.Toolsets = []string{"nonexistent-toolset"}
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid toolset name")
		s.Contains(err.Error(), "nonexistent-toolset")
	})

	s.Run("valid toolset names are accepted", func() {
		cfg := s.validConfig()
		cfg.Toolsets = []string{"core", "config"}
		s.NoError(cfg.Validate())
	})
}

func (s *ValidateSuite) TestClusterProviderStrategy() {
	s.Run("unknown strategy is skipped without WithProviderStrategies", func() {
		cfg := s.validConfig()
		cfg.ClusterProviderStrategy = "nonexistent-strategy"
		s.NoError(cfg.Validate())
	})

	s.Run("unknown strategy is rejected with WithProviderStrategies", func() {
		cfg := s.validConfig()
		cfg.ClusterProviderStrategy = "nonexistent-strategy"
		err := cfg.WithProviderStrategies([]string{"kubeconfig", "in-cluster"}).Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid cluster-provider")
		s.Contains(err.Error(), "nonexistent-strategy")
	})

	s.Run("valid strategy is accepted with WithProviderStrategies", func() {
		cfg := s.validConfig()
		cfg.ClusterProviderStrategy = "kubeconfig"
		s.NoError(cfg.WithProviderStrategies([]string{"kubeconfig", "in-cluster"}).Validate())
	})
}

func (s *ValidateSuite) TestAuthorizationURL() {
	s.Run("invalid scheme is rejected", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth = true
		cfg.AuthorizationURL = "ftp://example.com/auth"
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "--authorization-url must be a valid URL")
	})

	s.Run("https scheme is accepted", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth = true
		cfg.AuthorizationURL = "https://example.com/auth"
		s.NoError(cfg.Validate())
	})

	s.Run("http scheme is accepted with warning", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth = true
		cfg.AuthorizationURL = "http://example.com/auth"
		s.NoError(cfg.Validate())
	})

	s.Run("authorization_url without require_oauth is rejected", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth = false
		cfg.AuthorizationURL = "https://example.com/auth"
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "require-oauth is enabled")
	})
}

func (s *ValidateSuite) TestCertificateAuthority() {
	s.Run("non-existent file is rejected", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth = true
		cfg.CertificateAuthority = "/nonexistent/path/ca.crt"
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "certificate-authority must be a valid file path")
	})

	s.Run("existing file is accepted", func() {
		tmpDir := s.T().TempDir()
		caPath := filepath.Join(tmpDir, "ca.crt")
		s.Require().NoError(os.WriteFile(caPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.RequireOAuth = true
		cfg.CertificateAuthority = caPath
		s.NoError(cfg.Validate())
	})

	s.Run("whitespace-only is treated as empty", func() {
		cfg := s.validConfig()
		cfg.CertificateAuthority = "   "
		s.NoError(cfg.Validate())
		s.Equal("", cfg.CertificateAuthority, "whitespace should be trimmed from certificate-authority")
	})
}

func (s *ValidateSuite) TestTLSCertKey() {
	s.Run("tls_cert without tls_key is rejected", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.TLSCert = certPath
		cfg.TLSKey = ""
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "both --tls-cert and --tls-key must be provided together")
	})

	s.Run("tls_key without tls_cert is rejected", func() {
		tmpDir := s.T().TempDir()
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.TLSCert = ""
		cfg.TLSKey = keyPath
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "both --tls-cert and --tls-key must be provided together")
	})

	s.Run("non-existent tls_cert file is rejected", func() {
		tmpDir := s.T().TempDir()
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.TLSCert = "/nonexistent/cert.pem"
		cfg.TLSKey = keyPath
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "tls-cert must be a valid file path")
	})

	s.Run("non-existent tls_key file is rejected", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.TLSCert = certPath
		cfg.TLSKey = "/nonexistent/key.pem"
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "tls-key must be a valid file path")
	})

	s.Run("both tls_cert and tls_key with valid files are accepted", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.TLSCert = certPath
		cfg.TLSKey = keyPath
		s.NoError(cfg.Validate())
	})

	s.Run("whitespace-only tls_cert and tls_key are treated as empty", func() {
		cfg := s.validConfig()
		cfg.TLSCert = "   "
		cfg.TLSKey = "   "
		s.NoError(cfg.Validate())
		s.Equal("", cfg.TLSCert, "whitespace should be trimmed from tls-cert")
		s.Equal("", cfg.TLSKey, "whitespace should be trimmed from tls-key")
	})
}

func (s *ValidateSuite) TestTokenExchangeStrategy() {
	s.Run("unknown strategy is skipped without WithTokenExchangeStrategies", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth = true
		cfg.TokenExchangeStrategy = "nonexistent-strategy"
		s.NoError(cfg.Validate())
	})

	s.Run("unknown strategy is rejected with WithTokenExchangeStrategies", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth = true
		cfg.TokenExchangeStrategy = "nonexistent-strategy"
		err := cfg.WithTokenExchangeStrategies([]string{"rfc8693", "keycloak-v1", "entra-obo"}).Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid token_exchange_strategy")
		s.Contains(err.Error(), "nonexistent-strategy")
	})

	s.Run("valid strategy is accepted with WithTokenExchangeStrategies", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth = true
		cfg.TokenExchangeStrategy = "rfc8693"
		s.NoError(cfg.WithTokenExchangeStrategies([]string{"rfc8693", "keycloak-v1", "entra-obo"}).Validate())
	})
}

func (s *ValidateSuite) TestStsAuthStyle() {
	s.Run("invalid sts_auth_style is rejected", func() {
		cfg := s.validConfig()
		cfg.StsAuthStyle = "invalid-style"
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid sts_auth_style")
		s.Contains(err.Error(), "invalid-style")
	})

	s.Run("empty sts_auth_style is accepted", func() {
		cfg := s.validConfig()
		cfg.StsAuthStyle = ""
		s.NoError(cfg.Validate())
	})

	s.Run("params sts_auth_style is accepted", func() {
		cfg := s.validConfig()
		cfg.StsAuthStyle = "params"
		s.NoError(cfg.Validate())
	})

	s.Run("header sts_auth_style is accepted", func() {
		cfg := s.validConfig()
		cfg.StsAuthStyle = "header"
		s.NoError(cfg.Validate())
	})

	s.Run("whitespace-only sts_auth_style is treated as empty", func() {
		cfg := s.validConfig()
		cfg.StsAuthStyle = "   "
		s.NoError(cfg.Validate())
		s.Equal("", cfg.StsAuthStyle, "whitespace should be trimmed from sts_auth_style")
	})
}

func (s *ValidateSuite) TestStsClientCertKey() {
	s.Run("assertion auth_style without cert file is rejected", func() {
		cfg := s.validConfig()
		cfg.StsAuthStyle = "assertion"
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "sts_client_cert_file is required")
	})

	s.Run("assertion auth_style without key file is rejected", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.StsAuthStyle = "assertion"
		cfg.StsClientCertFile = certPath
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "sts_client_key_file is required")
	})

	s.Run("non-existent sts_client_cert_file is rejected", func() {
		tmpDir := s.T().TempDir()
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.StsAuthStyle = "assertion"
		cfg.StsClientCertFile = "/nonexistent/cert.pem"
		cfg.StsClientKeyFile = keyPath
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "sts_client_cert_file must be a valid file path")
	})

	s.Run("non-existent sts_client_key_file is rejected", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.StsAuthStyle = "assertion"
		cfg.StsClientCertFile = certPath
		cfg.StsClientKeyFile = "/nonexistent/key.pem"
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "sts_client_key_file must be a valid file path")
	})

	s.Run("assertion auth_style with valid cert and key is accepted", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.StsAuthStyle = "assertion"
		cfg.StsClientCertFile = certPath
		cfg.StsClientKeyFile = keyPath
		s.NoError(cfg.Validate())
	})

	s.Run("whitespace-only sts_client_cert_file is treated as empty", func() {
		tmpDir := s.T().TempDir()
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.StsAuthStyle = "assertion"
		cfg.StsClientCertFile = "   "
		cfg.StsClientKeyFile = keyPath
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "sts_client_cert_file is required")
		s.Equal("", cfg.StsClientCertFile, "whitespace should be trimmed from sts_client_cert_file")
	})

	s.Run("whitespace-only sts_client_key_file is treated as empty", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.StsAuthStyle = "assertion"
		cfg.StsClientCertFile = certPath
		cfg.StsClientKeyFile = "   "
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "sts_client_key_file is required")
		s.Equal("", cfg.StsClientKeyFile, "whitespace should be trimmed from sts_client_key_file")
	})
}

func (s *ValidateSuite) TestConfirmationFallback() {
	s.Run("empty fallback is accepted", func() {
		cfg := s.validConfig()
		cfg.ConfirmationFallback = ""
		s.NoError(cfg.Validate())
	})

	s.Run("allow fallback is accepted", func() {
		cfg := s.validConfig()
		cfg.ConfirmationFallback = "allow"
		s.NoError(cfg.Validate())
	})

	s.Run("deny fallback is accepted", func() {
		cfg := s.validConfig()
		cfg.ConfirmationFallback = "deny"
		s.NoError(cfg.Validate())
	})

	s.Run("invalid fallback value is rejected", func() {
		cfg := s.validConfig()
		cfg.ConfirmationFallback = "block"
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid confirmation_fallback")
		s.Contains(err.Error(), "block")
	})
}

func (s *ValidateSuite) TestConfirmationRules() {
	s.Run("empty rules are accepted", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules = nil
		s.NoError(cfg.Validate())
	})

	s.Run("valid tool-level rule is accepted", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules = []api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "Uninstall a release."},
		}
		s.NoError(cfg.Validate())
	})

	s.Run("valid kube-level rule is accepted", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules = []api.ConfirmationRule{
			{Verb: "delete", Kind: "Secret", Message: "Delete a Secret."},
		}
		s.NoError(cfg.Validate())
	})

	s.Run("rule mixing tool and kube fields is rejected", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules = []api.ConfirmationRule{
			{Tool: "helm_uninstall", Verb: "delete", Message: "Mixed rule."},
		}
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid confirmation rules")
	})

	s.Run("rule with no classifying fields is rejected", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules = []api.ConfirmationRule{
			{Message: "No level fields."},
		}
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "must set at least one")
	})

	s.Run("reports all rule errors with indices", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules = []api.ConfirmationRule{
			{Tool: "a", Verb: "delete", Message: "Mixed 1."},
			{Kind: "Pod", Tool: "b", Message: "Mixed 2."},
		}
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), "confirmation_rules[0]")
		s.Contains(err.Error(), "confirmation_rules[1]")
	})
}

func TestValidate(t *testing.T) {
	suite.Run(t, new(ValidateSuite))
}
