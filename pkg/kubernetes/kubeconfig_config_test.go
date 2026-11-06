package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type KubeConfigProviderConfigSuite struct {
	suite.Suite
}

func (s *KubeConfigProviderConfigSuite) TestValidate() {
	s.Run("accepts target with only an audience", func() {
		cfg := &KubeConfigProviderConfig{Targets: map[string]KubeConfigTargetConfig{
			"spoke": {Audience: "spoke-cluster"},
		}}
		s.NoError(cfg.Validate())
	})

	s.Run("accepts empty config", func() {
		s.NoError((&KubeConfigProviderConfig{}).Validate())
	})

	s.Run("rejects nil config", func() {
		var cfg *KubeConfigProviderConfig
		s.Error(cfg.Validate())
	})

	s.Run("rejects unknown strategy", func() {
		cfg := &KubeConfigProviderConfig{Targets: map[string]KubeConfigTargetConfig{
			"spoke": {Strategy: "nonexistent"},
		}}
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), `target "spoke"`)
		s.Contains(err.Error(), `invalid strategy "nonexistent"`)
	})

	s.Run("rejects token_url without http scheme or host", func() {
		for _, tokenURL := range []string{"ftp://idp.example.com/token", "https://", "not a url"} {
			cfg := &KubeConfigProviderConfig{Targets: map[string]KubeConfigTargetConfig{
				"spoke": {TokenURL: tokenURL},
			}}
			s.Error(cfg.Validate(), tokenURL)
		}
	})

	s.Run("accepts http and https token_url", func() {
		for _, tokenURL := range []string{"https://idp.example.com/token", "http://localhost:8080/token"} {
			cfg := &KubeConfigProviderConfig{Targets: map[string]KubeConfigTargetConfig{
				"spoke": {TokenURL: tokenURL},
			}}
			s.NoError(cfg.Validate(), tokenURL)
		}
	})

	s.Run("reports every invalid target by name", func() {
		cfg := &KubeConfigProviderConfig{Targets: map[string]KubeConfigTargetConfig{
			"good":  {Audience: "a"},
			"bad-1": {Strategy: "nope"},
			"bad-2": {TokenURL: "https://"},
		}}
		err := cfg.Validate()
		s.Require().Error(err)
		s.Contains(err.Error(), `target "bad-1"`)
		s.Contains(err.Error(), `target "bad-2"`)
		s.NotContains(err.Error(), `target "good"`)
	})
}

func (s *KubeConfigProviderConfigSuite) TestValidateClientAuth() {
	validate := func(auth KubeConfigTargetClientAuth) error {
		return (&KubeConfigProviderConfig{Targets: map[string]KubeConfigTargetConfig{
			"spoke": {ClientAuth: auth},
		}}).Validate()
	}

	s.Run("accepts bare client_id as a public client", func() {
		s.NoError(validate(KubeConfigTargetClientAuth{ClientID: "public"}))
	})

	s.Run("requires method when credentials are configured", func() {
		err := validate(KubeConfigTargetClientAuth{ClientID: "c", ClientSecret: "s"})
		s.Require().Error(err)
		s.Contains(err.Error(), "client_auth.method is required")
	})

	s.Run("requires client_id with a method", func() {
		err := validate(KubeConfigTargetClientAuth{Method: "client_secret_basic", ClientSecret: "s"})
		s.Require().Error(err)
		s.Contains(err.Error(), "client_auth.client_id is required")
	})

	s.Run("secret methods require client_secret", func() {
		for _, method := range []string{"client_secret_basic", "client_secret_post"} {
			err := validate(KubeConfigTargetClientAuth{Method: method, ClientID: "c"})
			s.Require().Error(err, method)
			s.Contains(err.Error(), "client_auth.client_secret is required")
			s.NoError(validate(KubeConfigTargetClientAuth{Method: method, ClientID: "c", ClientSecret: "s"}), method)
		}
	})

	s.Run("private_key_jwt requires existing certificate and key files", func() {
		err := validate(KubeConfigTargetClientAuth{Method: "private_key_jwt", ClientID: "c"})
		s.Require().Error(err)
		s.Contains(err.Error(), "client_auth.certificate_file is required")
		s.Contains(err.Error(), "client_auth.private_key_file is required")

		missing := filepath.Join(s.T().TempDir(), "missing.pem")
		err = validate(KubeConfigTargetClientAuth{Method: "private_key_jwt", ClientID: "c", CertificateFile: missing, PrivateKeyFile: missing})
		s.Require().Error(err)
		s.Contains(err.Error(), "missing.pem")

		cert, key := writeTempFile(s.T(), "cert.pem"), writeTempFile(s.T(), "key.pem")
		s.NoError(validate(KubeConfigTargetClientAuth{Method: "private_key_jwt", ClientID: "c", CertificateFile: cert, PrivateKeyFile: key}))
	})

	s.Run("jwt_file requires an existing token file", func() {
		err := validate(KubeConfigTargetClientAuth{Method: "jwt_file", ClientID: "c"})
		s.Require().Error(err)
		s.Contains(err.Error(), "client_auth.token_file is required")
		s.NoError(validate(KubeConfigTargetClientAuth{Method: "jwt_file", ClientID: "c", TokenFile: writeTempFile(s.T(), "token.jwt")}))
	})

	s.Run("rejects unknown method", func() {
		err := validate(KubeConfigTargetClientAuth{Method: "magic", ClientID: "c"})
		s.Require().Error(err)
		s.Contains(err.Error(), `invalid client_auth.method "magic"`)
	})
}

func (s *KubeConfigProviderConfigSuite) TestLoadFromTOML() {
	s.Run("decodes a fully specified target", func() {
		cfg := readTestConfig(s.T(), `
			cluster_provider_strategy = "kubeconfig"

			[cluster_provider_configs.kubeconfig.targets."spoke.example"]
			strategy = "rfc8693"
			token_url = "https://idp.example.com/token"
			audience = "spoke"
			scopes = ["mcp:spoke"]
			subject_token_type = "urn:ietf:params:oauth:token-type:id_token"
			requested_token_type = "urn:ietf:params:oauth:token-type:access_token"
			subject_issuer = "external"
			certificate_authority = "/etc/spoke-ca.crt"

			[cluster_provider_configs.kubeconfig.targets."spoke.example".client_auth]
			method = "client_secret_post"
			client_id = "spoke-client"
			client_secret = "spoke-secret"
		`)
		ext, ok := cfg.GetProviderConfig(config.ClusterProviderKubeConfig)
		s.Require().True(ok)
		pc := ext.(*KubeConfigProviderConfig)
		target, ok := pc.Targets["spoke.example"]
		s.Require().True(ok, "quoted dotted context name must map to one target")
		s.Equal(KubeConfigTargetConfig{
			Strategy:             "rfc8693",
			TokenURL:             "https://idp.example.com/token",
			Audience:             "spoke",
			Scopes:               []string{"mcp:spoke"},
			SubjectTokenType:     "urn:ietf:params:oauth:token-type:id_token",
			RequestedTokenType:   "urn:ietf:params:oauth:token-type:access_token",
			SubjectIssuer:        "external",
			CertificateAuthority: "/etc/spoke-ca.crt",
			ClientAuth: KubeConfigTargetClientAuth{
				Method:       "client_secret_post",
				ClientID:     "spoke-client",
				ClientSecret: "spoke-secret",
			},
		}, target)
	})

	s.Run("rejects unknown keys inside a target", func() {
		_, err := config.Read(context.Background(), writeTestConfig(s.T(), `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			audience = "spoke"
			audiences = ["typo"]
		`), "")
		s.Require().Error(err)
		s.Contains(err.Error(), "audiences")
	})

	s.Run("rejects unknown keys inside client_auth", func() {
		_, err := config.Read(context.Background(), writeTestConfig(s.T(), `
			[cluster_provider_configs.kubeconfig.targets.spoke.client_auth]
			method = "client_secret_basic"
			client_id = "c"
			secret = "typo"
		`), "")
		s.Require().Error(err)
		s.Contains(err.Error(), "secret")
	})

	s.Run("rejects invalid target at load time", func() {
		_, err := config.Read(context.Background(), writeTestConfig(s.T(), `
			[cluster_provider_configs.kubeconfig.targets.spoke]
			strategy = "nonexistent"
		`), "")
		s.Require().Error(err)
		s.Contains(err.Error(), `invalid strategy "nonexistent"`)
	})
}

func TestKubeConfigProviderConfig(t *testing.T) {
	suite.Run(t, new(KubeConfigProviderConfigSuite))
}

// writeTestConfig writes a TOML config to a temp file and returns its path.
func writeTestConfig(t testing.TB, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}

// readTestConfig loads a TOML config from a temp file.
func readTestConfig(t testing.TB, content string) *config.Config {
	t.Helper()
	cfg, err := config.Read(context.Background(), writeTestConfig(t, content), "")
	if err != nil {
		t.Fatalf("read test config: %v", err)
	}
	return cfg
}

// writeTempFile creates a placeholder file and returns a TOML-safe path.
func writeTempFile(t testing.TB, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("placeholder"), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return filepath.ToSlash(path)
}
