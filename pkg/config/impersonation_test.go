package config_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type ImpersonationConfigSuite struct{ suite.Suite }

const impersonationConfig = `
port = "8080"
cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = ["127.0.0.1/32"]
`

func (s *ImpersonationConfigSuite) TestValidation() {
	for _, tc := range []struct{ name, toml, wantErr string }{
		{"HTTP kubeconfig", impersonationConfig, ""},
		{"stateless HTTP", impersonationConfig + `stateless = true`, ""},
		{"stdio", strings.Replace(impersonationConfig, `port = "8080"`, "", 1), "requires port"},
		{"missing proxies", strings.Replace(impersonationConfig, `impersonation_trusted_proxies = ["127.0.0.1/32"]`, "", 1), "requires impersonation_trusted_proxies"},
		{"invalid CIDR", strings.Replace(impersonationConfig, "127.0.0.1/32", "127.0.0.1", 1), "invalid CIDR"},
		{"other auth mode", strings.Replace(impersonationConfig, `cluster_auth_mode = "impersonation"`, `cluster_auth_mode = "kubeconfig"`, 1), "only valid"},
		{"other provider", impersonationConfig + `cluster_provider_strategy = "in-cluster"`, "only supports kubeconfig"},
		{"unverified JWT", impersonationConfig + `skip_jwt_verification = true`, "skip_jwt_verification"},
		{"token exchange", impersonationConfig + `
require_oauth = true
authorization_url = "https://idp.example.com"
[token_exchange]
strategy = "rfc8693"
`, "token_exchange is incompatible"},
	} {
		s.Run(tc.name, func() {
			cfg, err := config.ReadToml(s.T().Context(), []byte(tc.toml), config.WithBaseDefault())
			s.Require().NoError(err)
			err = cfg.Validate(s.T().Context())
			if tc.wantErr == "" {
				s.NoError(err)
			} else {
				s.ErrorContains(err, tc.wantErr)
			}
		})
	}
}

func (s *ImpersonationConfigSuite) TestReloadRequiresRestart() {
	const kubeconfigMode = `port = "8080"
cluster_auth_mode = "kubeconfig"`
	for _, tc := range []struct{ name, previous, next, wantErr string }{
		{"unchanged", impersonationConfig, impersonationConfig, ""},
		{"enable", kubeconfigMode, impersonationConfig, "restart"},
		{"disable", impersonationConfig, kubeconfigMode, "restart"},
		{"change trusted proxies", impersonationConfig, strings.Replace(impersonationConfig, "127.0.0.1/32", "10.20.30.40/32", 1), "restart"},
	} {
		s.Run(tc.name, func() {
			previous, err := config.ReadToml(s.T().Context(), []byte(tc.previous), config.WithBaseDefault())
			s.Require().NoError(err)
			_, err = config.ReadToml(s.T().Context(), []byte(tc.next), config.WithBaseDefault(), config.WithPrevious(previous))
			if tc.wantErr == "" {
				s.NoError(err)
			} else {
				s.ErrorContains(err, tc.wantErr)
			}
		})
	}
}

func TestImpersonationConfig(t *testing.T) { suite.Run(t, new(ImpersonationConfigSuite)) }
