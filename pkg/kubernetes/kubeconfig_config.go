package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

// KubeConfigProviderConfig holds per-target token exchange configuration for
// the kubeconfig cluster provider. It is registered under
// cluster_provider_configs and keyed by kubeconfig context name:
//
//	[cluster_provider_configs.kubeconfig.targets."spoke-context"]
//	audience = "spoke-cluster"
//	scopes = ["mcp:spoke"]
//
// Every target may carry a complete token exchange configuration. Fields left
// unset fall back to the global [token_exchange] block, and the token URL
// additionally falls back to the endpoint discovered from authorization_url.
// A target that sets everything it needs works without a global block at all,
// so different contexts can exchange against different identity providers.
type KubeConfigProviderConfig struct {
	Targets map[string]KubeConfigTargetConfig `toml:"targets,omitempty"`
}

// KubeConfigTargetConfig mirrors the global [token_exchange] block for one
// kubeconfig context. Unset fields inherit the global value.
type KubeConfigTargetConfig struct {
	// Strategy selects the token exchange implementation (e.g. rfc8693).
	Strategy string `toml:"strategy,omitempty"`
	// TokenURL is the explicit token-exchange endpoint for this target.
	TokenURL string `toml:"token_url,omitempty"`
	// Audience is the audience requested during token exchange.
	Audience string `toml:"audience,omitempty"`
	// Scopes are the scopes requested during token exchange.
	Scopes []string `toml:"scopes,omitempty"`
	// SubjectTokenType is an RFC 8693 subject_token_type override.
	SubjectTokenType string `toml:"subject_token_type,omitempty"`
	// RequestedTokenType is an RFC 8693 requested_token_type override.
	RequestedTokenType string `toml:"requested_token_type,omitempty"`
	// SubjectIssuer is the Keycloak subject_issuer for cross-realm exchange.
	SubjectIssuer string `toml:"subject_issuer,omitempty"`
	// CertificateAuthority is a CA bundle for the target's token endpoint.
	// Inherits the top-level certificate_authority when unset.
	CertificateAuthority string `toml:"certificate_authority,omitempty"`
	// ClientAuth replaces the global [token_exchange.client_auth] block as a
	// whole when any of its fields is set; credentials are never mixed.
	ClientAuth KubeConfigTargetClientAuth `toml:"client_auth,omitempty"`
}

// KubeConfigTargetClientAuth mirrors [token_exchange.client_auth].
type KubeConfigTargetClientAuth struct {
	Method          string `toml:"method,omitempty"`
	ClientID        string `toml:"client_id,omitempty"`
	ClientSecret    string `toml:"client_secret,omitempty"`
	CertificateFile string `toml:"certificate_file,omitempty"`
	PrivateKeyFile  string `toml:"private_key_file,omitempty"`
	TokenFile       string `toml:"token_file,omitempty"`
}

var _ config.ExtendedConfig = (*KubeConfigProviderConfig)(nil)

func (c *KubeConfigProviderConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("kubeconfig provider config is nil")
	}
	names := make([]string, 0, len(c.Targets))
	for name := range c.Targets {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		if err := c.Targets[name].validate(); err != nil {
			errs = append(errs, fmt.Errorf("target %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

func (t KubeConfigTargetConfig) validate() error {
	var errs []error
	if t.Strategy != "" {
		strategies := tokenexchange.GetRegisteredStrategies()
		if !slices.Contains(strategies, t.Strategy) {
			errs = append(errs, fmt.Errorf("invalid strategy %q: valid values are: %s", t.Strategy, strings.Join(strategies, ", ")))
		}
	}
	if t.TokenURL != "" {
		u, err := url.Parse(t.TokenURL)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("invalid token_url: %w", err))
		case u.Scheme != "https" && u.Scheme != "http":
			errs = append(errs, fmt.Errorf("token_url must use the http or https scheme, got %q", u.Scheme))
		case u.Host == "":
			errs = append(errs, fmt.Errorf("token_url must include a host"))
		}
	}
	if err := t.ClientAuth.validate(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// isSet reports whether any client_auth field was configured for the target.
func (a KubeConfigTargetClientAuth) isSet() bool {
	return a.Method != "" || a.ClientID != "" || a.ClientSecret != "" ||
		a.CertificateFile != "" || a.PrivateKeyFile != "" || a.TokenFile != ""
}

// validate applies the same rules as the global token_exchange.client_auth block.
func (a KubeConfigTargetClientAuth) validate() error {
	if !a.isSet() {
		return nil
	}
	if a.Method == "" {
		// A bare client_id is a public client and needs no method.
		if a.ClientID != "" && a.ClientSecret == "" && a.CertificateFile == "" && a.PrivateKeyFile == "" && a.TokenFile == "" {
			return nil
		}
		return fmt.Errorf("client_auth.method is required when client authentication fields are configured")
	}
	var errs []error
	if a.ClientID == "" {
		errs = append(errs, fmt.Errorf("client_auth.client_id is required when method is %q", a.Method))
	}
	switch config.TokenExchangeClientAuthMethod(a.Method) {
	case config.TokenExchangeClientAuthMethodSecretBasic, config.TokenExchangeClientAuthMethodSecretPost:
		if a.ClientSecret == "" {
			errs = append(errs, fmt.Errorf("client_auth.client_secret is required when method is %q", a.Method))
		}
	case config.TokenExchangeClientAuthMethodPrivateKey:
		errs = append(errs, requireClientAuthFile("certificate_file", a.CertificateFile), requireClientAuthFile("private_key_file", a.PrivateKeyFile))
	case config.TokenExchangeClientAuthMethodJWTFile:
		errs = append(errs, requireClientAuthFile("token_file", a.TokenFile))
	default:
		errs = append(errs, fmt.Errorf("invalid client_auth.method %q: must be %s, %s, %s, or %s", a.Method,
			config.TokenExchangeClientAuthMethodSecretBasic, config.TokenExchangeClientAuthMethodSecretPost,
			config.TokenExchangeClientAuthMethodPrivateKey, config.TokenExchangeClientAuthMethodJWTFile))
	}
	return errors.Join(errs...)
}

func requireClientAuthFile(name, path string) error {
	if path == "" {
		return fmt.Errorf("client_auth.%s is required", name)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("client_auth.%s %q: %w", name, path, err)
	}
	return nil
}

func kubeconfigProviderConfigParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (config.ExtendedConfig, error) {
	var cfg KubeConfigProviderConfig
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func init() {
	config.RegisterProviderConfig(config.ClusterProviderKubeConfig, kubeconfigProviderConfigParser)
}
