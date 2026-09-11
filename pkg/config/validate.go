package config

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/tlsutil"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// Validate validates config-level invariants that must hold at both startup and
// on SIGHUP reload.
func (c *Config) Validate(ctx context.Context) error {
	c.CertificateAuthority.replaceValue(strings.TrimSpace(c.CertificateAuthority.Get()))
	c.TLSCert.replaceValue(strings.TrimSpace(c.TLSCert.Get()))
	c.TLSKey.replaceValue(strings.TrimSpace(c.TLSKey.Get()))
	c.normalizeTokenExchange()

	if c.Port.Get() == "" && c.RequireOAuth.Get() {
		return fmt.Errorf("require_oauth is not supported in stdio mode (port is empty); set port for HTTP or disable require_oauth")
	}

	var optErr error
	walkOptions(c, func(o option, path string) {
		if optErr != nil {
			return
		}
		if err := o.validateValue(); err != nil {
			optErr = err
		}
	})
	if optErr != nil {
		return optErr
	}

	if err := toolsets.Validate(c.Toolsets.Get()); err != nil {
		return err
	}
	if c.MetricsPort.Get() != "" && c.Port.Get() == "" {
		return fmt.Errorf("metrics_port requires port to be set (metrics port is only supported in HTTP mode)")
	}
	if c.MetricsPort.Get() != "" && c.MetricsPort.Get() == c.Port.Get() {
		return fmt.Errorf("metrics_port must be different from port")
	}
	if c.ClusterProviderStrategy.Get() != "" && len(c.providerStrategies) > 0 {
		if !slices.Contains(c.providerStrategies, c.ClusterProviderStrategy.Get()) {
			return fmt.Errorf("invalid cluster_provider_strategy: %s, valid values are: %s", c.ClusterProviderStrategy.Get(), strings.Join(c.providerStrategies, ", "))
		}
	}
	if !c.RequireOAuth.Get() && (c.OAuthAudience.Get() != "" || c.AuthorizationURL.Get() != "" || c.ServerURL.Get() != "" || c.CertificateAuthority.Get() != "") {
		return fmt.Errorf("oauth_audience, authorization_url, server_url and certificate_authority are only valid if require_oauth is enabled")
	}
	if c.AuthorizationURL.Get() != "" {
		u, err := url.Parse(c.AuthorizationURL.Get())
		if err != nil {
			return err
		}
		if u.Scheme != "https" && u.Scheme != "http" {
			return fmt.Errorf("authorization_url must be a valid URL")
		}
		if u.Scheme == "http" {
			klogutil.LogWarn(
				klogutil.FromContext(ctx),
				"authorization_url is using insecure scheme, this is not recommended production use",
				klogutil.Field("url.scheme", "http"),
			)
		}
	}
	if err := c.validateSkipJWTVerification(ctx); err != nil {
		return err
	}
	if (c.TLSCert.Get() != "" && c.TLSKey.Get() == "") || (c.TLSCert.Get() == "" && c.TLSKey.Get() != "") {
		return fmt.Errorf("both tls_cert and tls_key must be provided together")
	}
	if err := c.ValidateRequireTLS(); err != nil {
		return err
	}
	if err := c.ValidateClusterAuthMode(); err != nil {
		return err
	}
	if err := c.validateTokenExchange(); err != nil {
		return err
	}
	if c.TLSCert.Get() != "" && c.Port.Get() == "" {
		return fmt.Errorf("tls_cert and tls_key require port to be set (TLS is only supported in HTTP mode)")
	}
	if c.RequireTLS.Get() && c.Port.Get() != "" {
		if c.TLSCert.Get() == "" || c.TLSKey.Get() == "" {
			return fmt.Errorf("require_tls is enabled but TLS certificates are not configured (set tls_cert and tls_key)")
		}
	}
	return nil
}

func validateListOutput(v string) error {
	if output.FromString(v) == nil {
		return fmt.Errorf("invalid output name: %s, valid names are: %s", v, strings.Join(output.Names, ", "))
	}
	return nil
}

func validateMetricsPortNumber(v string) error {
	if v == "" {
		return nil
	}
	p, err := strconv.Atoi(v)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("metrics_port must be a valid port number (1-65535), got %q", v)
	}
	return nil
}

func validateClusterAuthModeValue(mode string) error {
	if mode != "" && mode != api.ClusterAuthPassthrough && mode != api.ClusterAuthKubeconfig {
		return fmt.Errorf("invalid cluster_auth_mode %q: must be %q or %q", mode, api.ClusterAuthPassthrough, api.ClusterAuthKubeconfig)
	}
	return nil
}

func validateConfirmationFallback(fb string) error {
	if fb != "" && fb != "allow" && fb != "deny" {
		return fmt.Errorf("invalid confirmation_fallback %q: must be \"allow\" or \"deny\"", fb)
	}
	return nil
}

func validateConfirmationRules(rules []api.ConfirmationRule) error {
	var ruleErrors []error
	for i, rule := range rules {
		if ruleErr := rule.Validate(); ruleErr != nil {
			ruleErrors = append(ruleErrors, fmt.Errorf("confirmation_rules[%d]: %w", i, ruleErr))
		}
	}
	if len(ruleErrors) > 0 {
		return fmt.Errorf("invalid confirmation rules:\n%w", errors.Join(ruleErrors...))
	}
	return nil
}

func validateExistingFile(field string) func(string) error {
	return func(path string) error {
		return requireExistingFile(field, strings.TrimSpace(path))
	}
}

func validateTLSMinVersion(v string) error {
	_, err := tlsutil.ParseTLSVersion(v)
	return err
}

func validateTLSCipherSuites(v []string) error {
	_, err := tlsutil.ParseTLSCipherSuites(v)
	return err
}

func requireExistingFile(field, path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s must be a valid file path: %w", field, err)
	}
	return nil
}

func (c *Config) validateSkipJWTVerification(ctx context.Context) error {
	if !c.RequireOAuth.Get() || c.AuthorizationURL.Get() != "" {
		return nil
	}
	if c.SkipJWTVerification.Get() {
		klogutil.LogWarn(klogutil.FromContext(ctx),
			"skip_jwt_verification is enabled with no authorization_url: bearer tokens will be forwarded without any local validation. "+
				"The cluster (or a trusted upstream) is the sole authority. Only use this when cluster_auth_mode=passthrough and the cluster validates tokens directly.")
		return nil
	}
	return fmt.Errorf("require_oauth is enabled but authorization_url is not configured: " +
		"JWTs cannot be cryptographically verified without an OIDC provider. " +
		"Set authorization_url to an OIDC issuer, or set skip_jwt_verification=true " +
		"if the server is behind a trusted reverse proxy that verifies tokens")
}

func (c *Config) validateTokenExchange() error {
	if c.GetTokenExchangeConfig() == nil {
		return nil
	}
	if c.AuthorizationURL.Get() == "" {
		return fmt.Errorf("token exchange requires authorization_url to discover the token endpoint")
	}
	strategies := c.tokenExchangeStrategies
	if len(strategies) == 0 {
		strategies = tokenexchange.GetRegisteredStrategies()
	}
	if c.TokenExchange.Strategy.Get() == "" || !slices.Contains(strategies, c.TokenExchange.Strategy.Get()) {
		return fmt.Errorf("invalid token_exchange.strategy %q: valid values are: %s", c.TokenExchange.Strategy.Get(), strings.Join(strategies, ", "))
	}
	auth := c.TokenExchange.ClientAuth
	if !auth.configured() {
		return nil
	}
	if auth.Method.Get() == "" {
		if auth.ClientID.Get() != "" && auth.ClientSecret.Get() == "" && auth.CertificateFile.Get() == "" && auth.PrivateKeyFile.Get() == "" && auth.TokenFile.Get() == "" {
			return nil
		}
		return fmt.Errorf("token_exchange.client_auth.method is required when client authentication fields are configured")
	}
	if auth.ClientID.Get() == "" {
		return fmt.Errorf("token_exchange.client_auth.client_id is required when method is %q", auth.Method.Get())
	}
	switch api.TokenExchangeClientAuthMethod(auth.Method.Get()) {
	case api.TokenExchangeClientAuthMethodSecretBasic, api.TokenExchangeClientAuthMethodSecretPost:
		if auth.ClientSecret.Get() == "" {
			return fmt.Errorf("token_exchange.client_auth.client_secret is required when method is %q", auth.Method.Get())
		}
	case api.TokenExchangeClientAuthMethodPrivateKey:
		if err := validateTokenExchangeFile("certificate_file", auth.CertificateFile.Get()); err != nil {
			return err
		}
		if err := validateTokenExchangeFile("private_key_file", auth.PrivateKeyFile.Get()); err != nil {
			return err
		}
	case api.TokenExchangeClientAuthMethodJWTFile:
		if err := validateTokenExchangeFile("token_file", auth.TokenFile.Get()); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid token_exchange.client_auth.method %q: must be client_secret_basic, client_secret_post, private_key_jwt, or jwt_file", auth.Method.Get())
	}
	return nil
}

func (c *Config) ValidateRequireTLS() error {
	if !c.RequireTLS.Get() {
		return nil
	}
	return ValidateURLsRequireTLS(map[string]string{
		"authorization_url": c.AuthorizationURL.Get(),
		"server_url":        c.ServerURL.Get(),
	})
}

func (c *Config) ValidateClusterAuthMode() error {
	mode := c.ClusterAuthMode.Get()
	if mode == api.ClusterAuthKubeconfig && c.RequireOAuth.Get() {
		return fmt.Errorf("cluster_auth_mode %q is not compatible with require_oauth=true: all authenticated users would share a single cluster identity, breaking per-user audit trails; use passthrough or token exchange to preserve user identity on the cluster", api.ClusterAuthKubeconfig)
	}
	hasTokenExchange := c.GetTokenExchangeConfig() != nil
	if mode == api.ClusterAuthKubeconfig && hasTokenExchange {
		return fmt.Errorf("token_exchange is incompatible with cluster_auth_mode %q (exchanged token would be unused)", api.ClusterAuthKubeconfig)
	}
	if !c.RequireOAuth.Get() && hasTokenExchange {
		return fmt.Errorf("token exchange requires require_oauth=true (token exchange depends on OAuth-validated tokens)")
	}
	return nil
}

func (c *Config) normalizeTokenExchange() {
	te := &c.TokenExchange
	te.Strategy.replaceValue(strings.TrimSpace(te.Strategy.Get()))
	te.Audience.replaceValue(strings.TrimSpace(te.Audience.Get()))
	te.SubjectTokenType.replaceValue(strings.TrimSpace(te.SubjectTokenType.Get()))
	te.RequestedTokenType.replaceValue(strings.TrimSpace(te.RequestedTokenType.Get()))
	auth := &te.ClientAuth
	auth.Method.replaceValue(strings.TrimSpace(auth.Method.Get()))
	auth.ClientID.replaceValue(strings.TrimSpace(auth.ClientID.Get()))
	auth.ClientSecret.replaceValue(strings.TrimSpace(auth.ClientSecret.Get()))
	auth.CertificateFile.replaceValue(strings.TrimSpace(auth.CertificateFile.Get()))
	auth.PrivateKeyFile.replaceValue(strings.TrimSpace(auth.PrivateKeyFile.Get()))
	auth.TokenFile.replaceValue(strings.TrimSpace(auth.TokenFile.Get()))
}

func validateTokenExchangeFile(name, path string) error {
	if path == "" {
		return fmt.Errorf("token_exchange.client_auth.%s is required", name)
	}
	return requireExistingFile("token_exchange.client_auth."+name, path)
}
