package config_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type ImpersonationConfigSuite struct {
	suite.Suite
}

func (s *ImpersonationConfigSuite) TestValidation() {
	const valid = `
port = "8080"
cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = ["127.0.0.1/32", "::1/128"]
`
	for _, tc := range []struct {
		name    string
		toml    string
		wantErr string
	}{
		{"trusted proxy authentication", valid, ""},
		{"stateless mode", valid + `stateless = true`, ""},
		{"explicit kubeconfig provider", valid + `cluster_provider_strategy = "kubeconfig"`, ""},
		{"in-cluster provider is rejected", valid + `cluster_provider_strategy = "in-cluster"`, "only supports kubeconfig"},
		{"KCP provider is rejected", valid + `cluster_provider_strategy = "kcp"`, "only supports kubeconfig"},
		{"disabled provider is rejected", valid + `cluster_provider_strategy = "disabled"`, "only supports kubeconfig"},
		{"custom provider is rejected", valid + `cluster_provider_strategy = "custom"`, "only supports kubeconfig"},
		{"additional OIDC verification", valid + `
require_oauth = true
authorization_url = "https://idp.example.com"
oauth_audience = "mcp"
`, ""},
		{"stdio is rejected", `cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = ["127.0.0.1/32"]`, "requires port"},
		{"missing trusted proxies", `port = "8080"
cluster_auth_mode = "impersonation"`, "requires impersonation_trusted_proxies"},
		{"empty trusted proxies", `port = "8080"
cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = []`, "requires impersonation_trusted_proxies"},
		{"invalid CIDR", `port = "8080"
cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = ["127.0.0.1"]`, "invalid CIDR"},
		{"invalid IPv6 prefix", `port = "8080"
cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = ["::1/129"]`, "invalid CIDR"},
		{"empty CIDR", `port = "8080"
cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = [""]`, "invalid CIDR"},
		{"proxy setting outside mode", `impersonation_trusted_proxies = ["127.0.0.1/32"]`, "only valid"},
		{"explicit empty proxy setting outside mode", `impersonation_trusted_proxies = []`, "only valid"},
		{"OAuth without verifier", valid + `require_oauth = true`, "authorization_url"},
		{"skipping JWT verification", valid + `
require_oauth = true
skip_jwt_verification = true
`, "skip_jwt_verification"},
		{"skipping JWT verification with issuer", valid + `
require_oauth = true
authorization_url = "https://idp.example.com"
skip_jwt_verification = true
`, "skip_jwt_verification"},
		{"skipping JWT verification without OAuth", valid + `skip_jwt_verification = true`, "skip_jwt_verification"},
		{"global token exchange", valid + `
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
			if tc.wantErr != "" {
				s.ErrorContains(err, tc.wantErr)
			} else {
				s.NoError(err)
			}
		})
	}
}

func (s *ImpersonationConfigSuite) TestProxyAllowlistRequiresRestart() {
	const initial = `
port = "8080"
cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = ["127.0.0.1/32"]
`
	previous, err := config.ReadToml(s.T().Context(), []byte(initial), config.WithBaseDefault())
	s.Require().NoError(err)
	const reloaded = `
port = "8080"
cluster_auth_mode = "impersonation"
impersonation_trusted_proxies = ["10.20.30.40/32"]
`
	_, err = config.ReadToml(s.T().Context(), []byte(reloaded), config.WithBaseDefault(), config.WithPrevious(previous))
	s.ErrorContains(err, "non-reloadable option impersonation_trusted_proxies")
	s.ErrorContains(err, "restart")
	s.Equal([]string{"127.0.0.1/32"}, previous.ImpersonationTrustedProxies.Get())
	_, err = config.ReadToml(s.T().Context(), []byte(initial), config.WithBaseDefault(), config.WithPrevious(previous))
	s.NoError(err, "an unchanged allowlist must not block other configuration reloads")
}

func (s *ImpersonationConfigSuite) TestDefaultsAndMetadata() {
	cfg := config.BaseDefault()
	s.Equal(config.ClusterAuthPassthrough, cfg.ResolveClusterAuthMode())
	s.Empty(cfg.ImpersonationTrustedProxies.Get())
	s.Equal("impersonation_trusted_proxies", cfg.ImpersonationTrustedProxies.TOMLKey)
}

func (s *ImpersonationConfigSuite) TestAuthModeTransitionsRequireRestart() {
	for _, modes := range [][2]string{
		{"", config.ClusterAuthImpersonation},
		{config.ClusterAuthPassthrough, config.ClusterAuthImpersonation},
		{config.ClusterAuthKubeconfig, config.ClusterAuthImpersonation},
		{config.ClusterAuthImpersonation, ""},
		{config.ClusterAuthImpersonation, config.ClusterAuthPassthrough},
		{config.ClusterAuthImpersonation, config.ClusterAuthKubeconfig},
	} {
		s.Run(modes[0]+" to "+modes[1], func() {
			previous := config.BaseDefault()
			previous.Port.SetForTest("8080")
			previous.ClusterAuthMode.SetForTest(modes[0])
			if modes[0] == config.ClusterAuthImpersonation {
				previous.ImpersonationTrustedProxies.SetForTest([]string{"127.0.0.1/32"})
			}
			next := fmt.Sprintf("port = \"8080\"\ncluster_auth_mode = %q\n", modes[1])
			if modes[1] == config.ClusterAuthImpersonation {
				next += "impersonation_trusted_proxies = [\"127.0.0.1/32\"]\n"
			}
			_, err := config.ReadToml(s.T().Context(), []byte(next), config.WithBaseDefault(), config.WithPrevious(previous))
			s.ErrorContains(err, "restart")
		})
	}
}

func TestImpersonationConfig(t *testing.T) {
	suite.Run(t, new(ImpersonationConfigSuite))
}
