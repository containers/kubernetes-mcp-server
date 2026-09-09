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

func (s *ValidateSuite) validConfig() *config.Config {
	cfg := config.New()
	return cfg
}

func (s *ValidateSuite) TestValidDefaultConfig() {
	s.Run("default config passes validation", func() {
		cfg := s.validConfig()
		s.NoError(cfg.Validate(s.T().Context()))
	})
}

func (s *ValidateSuite) TestRequireOAuthStdio() {
	s.Run("require_oauth with empty port is rejected", func() {
		cfg := s.validConfig()
		cfg.Port.SetForTest("")
		cfg.RequireOAuth.SetForTest(true)
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "require_oauth is not supported in stdio mode")
	})

	s.Run("require_oauth with port is accepted when skip_jwt_verification is set", func() {
		cfg := s.validConfig()
		cfg.Port.SetForTest("8080")
		cfg.RequireOAuth.SetForTest(true)
		cfg.SkipJWTVerification.SetForTest(true)
		s.NoError(cfg.Validate(s.T().Context()))
	})
}

func (s *ValidateSuite) TestListOutput() {
	cases := []struct {
		name    string
		value   string
		wantErr string
	}{
		{"invalid list_output is rejected", "invalid-format", "invalid output name"},
		{"empty list_output is rejected", "", "invalid output name"},
		{"yaml list_output is accepted", "yaml", ""},
		{"table list_output is accepted", "table", ""},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			cfg := s.validConfig()
			cfg.ListOutput.SetForTest(tc.value)
			err := cfg.Validate(s.T().Context())
			if tc.wantErr == "" {
				s.NoError(err)
				return
			}
			s.Require().Error(err)
			s.Contains(err.Error(), tc.wantErr)
			if tc.value != "" {
				s.Contains(err.Error(), tc.value)
			}
		})
	}
}

func (s *ValidateSuite) TestToolsets() {
	cases := []struct {
		name    string
		value   []string
		wantErr string
	}{
		{"invalid toolset name is rejected", []string{"nonexistent-toolset"}, "invalid toolset name"},
		{"valid toolset names are accepted", []string{"core", "config"}, ""},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			cfg := s.validConfig()
			cfg.Toolsets.SetForTest(tc.value)
			err := cfg.Validate(s.T().Context())
			if tc.wantErr == "" {
				s.NoError(err)
				return
			}
			s.Require().Error(err)
			s.Contains(err.Error(), tc.wantErr)
			s.Contains(err.Error(), tc.value[0])
		})
	}
}

func (s *ValidateSuite) TestClusterProviderStrategy() {
	cases := []struct {
		name       string
		strategy   string
		registered []string
		wantErr    string
	}{
		{"unknown strategy is skipped without WithProviderStrategies", "nonexistent-strategy", nil, ""},
		{"unknown strategy is rejected with WithProviderStrategies", "nonexistent-strategy", []string{"kubeconfig", "in-cluster"}, "invalid cluster_provider_strategy"},
		{"valid strategy is accepted with WithProviderStrategies", "kubeconfig", []string{"kubeconfig", "in-cluster"}, ""},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			cfg := s.validConfig()
			cfg.ClusterProviderStrategy.SetForTest(tc.strategy)
			if tc.registered != nil {
				cfg = cfg.WithProviderStrategies(tc.registered)
			}
			err := cfg.Validate(s.T().Context())
			if tc.wantErr == "" {
				s.NoError(err)
				return
			}
			s.Require().Error(err)
			s.Contains(err.Error(), tc.wantErr)
			s.Contains(err.Error(), tc.strategy)
		})
	}
}

func (s *ValidateSuite) TestAuthorizationURL() {
	cases := []struct {
		name        string
		requireAuth bool
		url         string
		wantErr     string
	}{
		{"invalid scheme is rejected", true, "ftp://example.com/auth", "authorization_url must be a valid URL"},
		{"https scheme is accepted", true, "https://example.com/auth", ""},
		{"http scheme is accepted with warning", true, "http://example.com/auth", ""},
		{"authorization_url without require_oauth is rejected", false, "https://example.com/auth", "only valid if require_oauth is enabled"},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			cfg := s.validConfig()
			if tc.requireAuth {
				cfg.Port.SetForTest("8080")
			}
			cfg.RequireOAuth.SetForTest(tc.requireAuth)
			cfg.AuthorizationURL.SetForTest(tc.url)
			err := cfg.Validate(s.T().Context())
			if tc.wantErr == "" {
				s.NoError(err)
				return
			}
			s.Require().Error(err)
			s.Contains(err.Error(), tc.wantErr)
		})
	}
}

func (s *ValidateSuite) TestCertificateAuthority() {
	s.Run("non-existent file is rejected", func() {
		cfg := s.validConfig()
		cfg.Port.SetForTest("8080")
		cfg.RequireOAuth.SetForTest(true)
		cfg.AuthorizationURL.SetForTest("https://example.com/auth")
		cfg.CertificateAuthority.SetForTest("/nonexistent/path/ca.crt")
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "certificate_authority must be a valid file path")
	})

	s.Run("existing file is accepted", func() {
		tmpDir := s.T().TempDir()
		caPath := filepath.Join(tmpDir, "ca.crt")
		s.Require().NoError(os.WriteFile(caPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.Port.SetForTest("8080")
		cfg.RequireOAuth.SetForTest(true)
		cfg.AuthorizationURL.SetForTest("https://example.com/auth")
		cfg.CertificateAuthority.SetForTest(caPath)
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("whitespace-only is treated as empty", func() {
		cfg := s.validConfig()
		cfg.CertificateAuthority.SetForTest("   ")
		s.NoError(cfg.Validate(s.T().Context()))
		s.Equal("", cfg.CertificateAuthority.Get(), "whitespace should be trimmed from certificate_authority")
	})
}

func (s *ValidateSuite) TestTLSCertKey() {
	s.Run("tls_cert without tls_key is rejected", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.TLSCert.SetForTest(certPath)
		cfg.TLSKey.SetForTest("")
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "both tls_cert and tls_key must be provided together")
	})

	s.Run("tls_key without tls_cert is rejected", func() {
		tmpDir := s.T().TempDir()
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.TLSCert.SetForTest("")
		cfg.TLSKey.SetForTest(keyPath)
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "both tls_cert and tls_key must be provided together")
	})

	s.Run("non-existent tls_cert file is rejected", func() {
		tmpDir := s.T().TempDir()
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.TLSCert.SetForTest("/nonexistent/cert.pem")
		cfg.TLSKey.SetForTest(keyPath)
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "tls_cert must be a valid file path")
	})

	s.Run("non-existent tls_key file is rejected", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.TLSCert.SetForTest(certPath)
		cfg.TLSKey.SetForTest("/nonexistent/key.pem")
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "tls_key must be a valid file path")
	})

	s.Run("both tls_cert and tls_key with valid files are accepted", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0644))

		cfg := s.validConfig()
		cfg.Port.SetForTest("8443")
		cfg.TLSCert.SetForTest(certPath)
		cfg.TLSKey.SetForTest(keyPath)
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("whitespace-only tls_cert and tls_key are treated as empty", func() {
		cfg := s.validConfig()
		cfg.TLSCert.SetForTest("   ")
		cfg.TLSKey.SetForTest("   ")
		s.NoError(cfg.Validate(s.T().Context()))
		s.Equal("", cfg.TLSCert.Get(), "whitespace should be trimmed from tls_cert")
		s.Equal("", cfg.TLSKey.Get(), "whitespace should be trimmed from tls_key")
	})
}

func (s *ValidateSuite) TestTLSSettings() {
	s.Run("default config passes with empty TLS settings", func() {
		s.Require().NoError(os.Unsetenv(config.EnvTLSMinVersion))
		s.Require().NoError(os.Unsetenv(config.EnvTLSCipherSuites))
		cfg := s.validConfig()
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("valid tls_min_version in config is accepted", func() {
		s.Require().NoError(os.Unsetenv(config.EnvTLSMinVersion))
		s.Require().NoError(os.Unsetenv(config.EnvTLSCipherSuites))
		cfg := s.validConfig()
		cfg.TLSMinVersion.SetForTest("1.3")
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("valid tls_cipher_suites in config is accepted", func() {
		s.Require().NoError(os.Unsetenv(config.EnvTLSMinVersion))
		s.Require().NoError(os.Unsetenv(config.EnvTLSCipherSuites))
		cfg := s.validConfig()
		cfg.TLSCipherSuites.SetForTest([]string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"})
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("TLS_MIN_VERSION env overrides invalid config value", func() {
		s.T().Setenv(config.EnvTLSMinVersion, "1.3")
		cfg, err := config.ReadToml([]byte(`tls_min_version = "invalid"`))
		s.Require().NoError(err)
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("invalid TLS_MIN_VERSION env is rejected", func() {
		s.T().Setenv(config.EnvTLSMinVersion, "bad")
		cfg, err := config.ReadToml(nil)
		s.Require().NoError(err)
		err = cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid TLS version")
	})

	s.Run("TLS_CIPHER_SUITES env overrides invalid config value", func() {
		s.T().Setenv(config.EnvTLSCipherSuites, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256")
		cfg, err := config.ReadToml([]byte(`tls_cipher_suites = ["UNKNOWN_CIPHER"]`))
		s.Require().NoError(err)
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("invalid TLS_CIPHER_SUITES env is rejected", func() {
		s.T().Setenv(config.EnvTLSCipherSuites, "UNKNOWN_CIPHER")
		cfg, err := config.ReadToml(nil)
		s.Require().NoError(err)
		err = cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid cipher suites")
	})
}

func (s *ValidateSuite) TestTokenExchangeStrategy() {
	cases := []struct {
		name     string
		strategy string
		wantErr  string
	}{
		{"unknown strategy is rejected", "nonexistent-strategy", "invalid token_exchange.strategy"},
		{"registered strategy is accepted", "rfc8693", ""},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			cfg := s.validConfig()
			cfg.Port.SetForTest("8080")
			cfg.RequireOAuth.SetForTest(true)
			cfg.AuthorizationURL.SetForTest("https://example.com/auth")
			cfg.TokenExchange.Strategy.SetForTest(tc.strategy)
			err := cfg.Validate(s.T().Context())
			if tc.wantErr == "" {
				s.NoError(err)
				return
			}
			s.Require().Error(err)
			s.Contains(err.Error(), tc.wantErr)
			s.Contains(err.Error(), tc.strategy)
		})
	}
}

func (s *ValidateSuite) TestTokenExchangeClientAuth() {
	type authVals struct {
		Method          api.TokenExchangeClientAuthMethod
		ClientID        string
		ClientSecret    string
		CertificateFile string
		PrivateKeyFile  string
		TokenFile       string
	}
	newConfig := func(auth authVals) *config.Config {
		cfg := s.validConfig()
		cfg.Port.SetForTest("8080")
		cfg.RequireOAuth.SetForTest(true)
		cfg.AuthorizationURL.SetForTest("https://example.com/auth")
		cfg.TokenExchange.Strategy.SetForTest("rfc8693")
		if auth.Method != "" {
			cfg.TokenExchange.ClientAuth.Method.SetForTest(string(auth.Method))
		}
		if auth.ClientID != "" {
			cfg.TokenExchange.ClientAuth.ClientID.SetForTest(auth.ClientID)
		}
		if auth.ClientSecret != "" {
			cfg.TokenExchange.ClientAuth.ClientSecret.SetForTest(auth.ClientSecret)
		}
		if auth.CertificateFile != "" {
			cfg.TokenExchange.ClientAuth.CertificateFile.SetForTest(auth.CertificateFile)
		}
		if auth.PrivateKeyFile != "" {
			cfg.TokenExchange.ClientAuth.PrivateKeyFile.SetForTest(auth.PrivateKeyFile)
		}
		if auth.TokenFile != "" {
			cfg.TokenExchange.ClientAuth.TokenFile.SetForTest(auth.TokenFile)
		}
		return cfg
	}

	s.Run("method is required when client authentication fields are configured", func() {
		cfg := newConfig(authVals{ClientSecret: "secret"})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "token_exchange.client_auth.method is required")
	})

	s.Run("public client with only a client ID is accepted", func() {
		cfg := newConfig(authVals{ClientID: "public-client"})
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("configured method requires a client ID", func() {
		cfg := newConfig(authVals{
			Method:       api.TokenExchangeClientAuthMethodSecretBasic,
			ClientSecret: "secret",
		})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "token_exchange.client_auth.client_id is required")
	})

	s.Run("invalid method is rejected", func() {
		cfg := newConfig(authVals{
			Method:   api.TokenExchangeClientAuthMethod("unknown"),
			ClientID: "client",
		})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid token_exchange.client_auth.method")
	})

	s.Run("client_secret_basic requires a client secret", func() {
		cfg := newConfig(authVals{
			Method:   api.TokenExchangeClientAuthMethodSecretBasic,
			ClientID: "client",
		})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "token_exchange.client_auth.client_secret is required")
	})

	s.Run("client_secret_basic with client credentials is accepted", func() {
		cfg := newConfig(authVals{
			Method:       api.TokenExchangeClientAuthMethodSecretBasic,
			ClientID:     "client",
			ClientSecret: "secret",
		})
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("client_secret_post with client credentials is accepted", func() {
		cfg := newConfig(authVals{
			Method:       api.TokenExchangeClientAuthMethodSecretPost,
			ClientID:     "client",
			ClientSecret: "secret",
		})
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("private_key_jwt requires certificate and private key files", func() {
		cfg := newConfig(authVals{
			Method:   api.TokenExchangeClientAuthMethodPrivateKey,
			ClientID: "client",
		})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "token_exchange.client_auth.certificate_file is required")
	})

	s.Run("private_key_jwt requires a private key file when certificate is configured", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))

		cfg := newConfig(authVals{
			Method:          api.TokenExchangeClientAuthMethodPrivateKey,
			ClientID:        "client",
			CertificateFile: certPath,
		})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "token_exchange.client_auth.private_key_file is required")
	})

	s.Run("private_key_jwt with valid certificate and private key files is accepted", func() {
		tmpDir := s.T().TempDir()
		certPath := filepath.Join(tmpDir, "cert.pem")
		keyPath := filepath.Join(tmpDir, "key.pem")
		s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0644))
		s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0644))

		cfg := newConfig(authVals{
			Method:          api.TokenExchangeClientAuthMethodPrivateKey,
			ClientID:        "client",
			CertificateFile: certPath,
			PrivateKeyFile:  keyPath,
		})
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("jwt_file requires a token file", func() {
		cfg := newConfig(authVals{
			Method:   api.TokenExchangeClientAuthMethodJWTFile,
			ClientID: "client",
		})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "token_exchange.client_auth.token_file is required")
	})

	s.Run("jwt_file with a valid token file is accepted", func() {
		tmpDir := s.T().TempDir()
		tokenPath := filepath.Join(tmpDir, "token")
		s.Require().NoError(os.WriteFile(tokenPath, []byte("jwt-token"), 0600))

		cfg := newConfig(authVals{
			Method:    api.TokenExchangeClientAuthMethodJWTFile,
			ClientID:  "client",
			TokenFile: tokenPath,
		})
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("jwt_file rejects a missing token file", func() {
		cfg := newConfig(authVals{
			Method:    api.TokenExchangeClientAuthMethodJWTFile,
			ClientID:  "client",
			TokenFile: filepath.Join(s.T().TempDir(), "missing-token"),
		})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "token_exchange.client_auth.token_file must be a valid file path")
	})
}

func (s *ValidateSuite) TestTokenExchangeWhitespaceNormalization() {
	cfg := s.validConfig()
	cfg.Port.SetForTest("8080")
	cfg.RequireOAuth.SetForTest(true)
	cfg.AuthorizationURL.SetForTest("https://example.com/auth")
	cfg.TokenExchange.Strategy.SetForTest(" rfc8693 ")
	cfg.TokenExchange.Audience.SetForTest(" audience ")
	cfg.TokenExchange.SubjectTokenType.SetForTest(" subject-token-type ")
	cfg.TokenExchange.RequestedTokenType.SetForTest(" requested-token-type ")
	cfg.TokenExchange.ClientAuth.Method.SetForTest(" client_secret_basic ")
	cfg.TokenExchange.ClientAuth.ClientID.SetForTest(" client ")
	cfg.TokenExchange.ClientAuth.ClientSecret.SetForTest(" secret ")
	cfg.TokenExchange.ClientAuth.CertificateFile.SetForTest(" cert.pem ")
	cfg.TokenExchange.ClientAuth.PrivateKeyFile.SetForTest(" key.pem ")
	cfg.TokenExchange.ClientAuth.TokenFile.SetForTest(" token ")

	s.Require().NoError(cfg.Validate(s.T().Context()))
	s.Equal("rfc8693", cfg.TokenExchange.Strategy.Get())
	s.Equal("audience", cfg.TokenExchange.Audience.Get())
	s.Equal("subject-token-type", cfg.TokenExchange.SubjectTokenType.Get())
	s.Equal("requested-token-type", cfg.TokenExchange.RequestedTokenType.Get())
	s.Equal(string(api.TokenExchangeClientAuthMethodSecretBasic), cfg.TokenExchange.ClientAuth.Method.Get())
	s.Equal("client", cfg.TokenExchange.ClientAuth.ClientID.Get())
	s.Equal("secret", cfg.TokenExchange.ClientAuth.ClientSecret.Get())
	s.Equal("cert.pem", cfg.TokenExchange.ClientAuth.CertificateFile.Get())
	s.Equal("key.pem", cfg.TokenExchange.ClientAuth.PrivateKeyFile.Get())
	s.Equal("token", cfg.TokenExchange.ClientAuth.TokenFile.Get())
}

func (s *ValidateSuite) TestConfirmationFallback() {
	cases := []struct {
		name    string
		value   string
		wantErr string
	}{
		{"empty fallback is accepted", "", ""},
		{"allow fallback is accepted", "allow", ""},
		{"deny fallback is accepted", "deny", ""},
		{"invalid fallback value is rejected", "block", "invalid confirmation_fallback"},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			cfg := s.validConfig()
			cfg.ConfirmationFallback.SetForTest(tc.value)
			err := cfg.Validate(s.T().Context())
			if tc.wantErr == "" {
				s.NoError(err)
				return
			}
			s.Require().Error(err)
			s.Contains(err.Error(), tc.wantErr)
			s.Contains(err.Error(), tc.value)
		})
	}
}

func (s *ValidateSuite) TestConfirmationRules() {
	s.Run("empty rules are accepted", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules.SetForTest(nil)
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("valid tool-level rule is accepted", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules.SetForTest([]api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "Uninstall a release."},
		})
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("valid kube-level rule is accepted", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules.SetForTest([]api.ConfirmationRule{
			{Verb: "delete", Kind: "Secret", Message: "Delete a Secret."},
		})
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("rule mixing tool and kube fields is rejected", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules.SetForTest([]api.ConfirmationRule{
			{Tool: "helm_uninstall", Verb: "delete", Message: "Mixed rule."},
		})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid confirmation rules")
	})

	s.Run("rule with no classifying fields is rejected", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules.SetForTest([]api.ConfirmationRule{
			{Message: "No level fields."},
		})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "must set at least one")
	})

	s.Run("reports all rule errors with indices", func() {
		cfg := s.validConfig()
		cfg.ConfirmationRules.SetForTest([]api.ConfirmationRule{
			{Tool: "a", Verb: "delete", Message: "Mixed 1."},
			{Kind: "Pod", Tool: "b", Message: "Mixed 2."},
		})
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "confirmation_rules[0]")
		s.Contains(err.Error(), "confirmation_rules[1]")
	})
}

func (s *ValidateSuite) TestSkipJWTVerification() {
	cases := []struct {
		name    string
		oauth   bool
		authURL string
		skipJWT bool
		wantErr string
	}{
		{"require_oauth with authorization_url set is accepted", true, "https://example.com/auth", false, ""},
		{"require_oauth without authorization_url and skip_jwt_verification=false is rejected", true, "", false, "require_oauth is enabled but authorization_url is not configured"},
		{"require_oauth without authorization_url and skip_jwt_verification=true is accepted", true, "", true, ""},
		{"require_oauth=false with skip_jwt_verification=true is accepted", false, "", true, ""},
		{"require_oauth=false is accepted", false, "", false, ""},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			cfg := s.validConfig()
			if tc.oauth {
				cfg.Port.SetForTest("8080")
			}
			cfg.RequireOAuth.SetForTest(tc.oauth)
			cfg.AuthorizationURL.SetForTest(tc.authURL)
			cfg.SkipJWTVerification.SetForTest(tc.skipJWT)
			err := cfg.Validate(s.T().Context())
			if tc.wantErr == "" {
				s.NoError(err)
				return
			}
			s.Require().Error(err)
			s.Contains(err.Error(), tc.wantErr)
			s.Contains(err.Error(), "skip_jwt_verification=true")
		})
	}
}

func (s *ValidateSuite) TestClusterAuthMode() {
	s.Run("passthrough without require_oauth is accepted", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth.SetForTest(false)
		cfg.ClusterAuthMode.SetForTest(api.ClusterAuthPassthrough)
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("passthrough with require_oauth is accepted", func() {
		cfg := s.validConfig()
		cfg.Port.SetForTest("8080")
		cfg.RequireOAuth.SetForTest(true)
		cfg.SkipJWTVerification.SetForTest(true)
		cfg.ClusterAuthMode.SetForTest(api.ClusterAuthPassthrough)
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("kubeconfig with require_oauth is rejected", func() {
		cfg := s.validConfig()
		cfg.Port.SetForTest("8080")
		cfg.RequireOAuth.SetForTest(true)
		cfg.SkipJWTVerification.SetForTest(true)
		cfg.ClusterAuthMode.SetForTest(api.ClusterAuthKubeconfig)
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "is not compatible with require_oauth=true")
	})

	s.Run("kubeconfig without require_oauth is accepted", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth.SetForTest(false)
		cfg.ClusterAuthMode.SetForTest(api.ClusterAuthKubeconfig)
		s.NoError(cfg.Validate(s.T().Context()))
	})

	s.Run("invalid cluster_auth_mode is rejected", func() {
		cfg := s.validConfig()
		cfg.ClusterAuthMode.SetForTest("bogus")
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid cluster_auth_mode")
	})

	s.Run("token exchange without require_oauth is rejected", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth.SetForTest(false)
		cfg.TokenExchange.Strategy.SetForTest("rfc8693")
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "token exchange requires require_oauth=true")
	})

	s.Run("token exchange without authorization_url is rejected", func() {
		cfg := s.validConfig()
		cfg.Port.SetForTest("8080")
		cfg.RequireOAuth.SetForTest(true)
		cfg.SkipJWTVerification.SetForTest(true)
		cfg.TokenExchange.Strategy.SetForTest("rfc8693")
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "token exchange requires authorization_url")
	})

	s.Run("token exchange with kubeconfig mode is rejected", func() {
		cfg := s.validConfig()
		cfg.RequireOAuth.SetForTest(false)
		cfg.ClusterAuthMode.SetForTest(api.ClusterAuthKubeconfig)
		cfg.TokenExchange.Strategy.SetForTest("rfc8693")
		err := cfg.Validate(s.T().Context())
		s.Require().Error(err)
		s.Contains(err.Error(), "token_exchange is incompatible with cluster_auth_mode")
	})
}

func (s *ValidateSuite) TestMetricsPort() {
	cases := []struct {
		name        string
		port        string
		metricsPort string
		wantErr     string
	}{
		{"metrics_port without port is rejected", "", "9090", "metrics_port requires port"},
		{"metrics_port same as port is rejected", "8080", "8080", "metrics_port must be different from port"},
		{"metrics_port with different port is accepted", "8080", "9090", ""},
		{"metrics_port with non-numeric value is rejected", "8080", "abc", "metrics_port must be a valid port number"},
		{"metrics_port with out-of-range value is rejected", "8080", "99999", "metrics_port must be a valid port number"},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			cfg := s.validConfig()
			cfg.Port.SetForTest(tc.port)
			cfg.MetricsPort.SetForTest(tc.metricsPort)
			err := cfg.Validate(s.T().Context())
			if tc.wantErr == "" {
				s.NoError(err)
				return
			}
			s.Require().Error(err)
			s.Contains(err.Error(), tc.wantErr)
		})
	}
}

func TestValidate(t *testing.T) {
	suite.Run(t, new(ValidateSuite))
}
