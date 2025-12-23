package tokenexchange

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	// AuthStyleParams sends client_id and client_secret in the request body
	AuthStyleParams = "params"
	// AuthStyleHeader sends client credentials as HTTP Basic Authentication header
	AuthStyleHeader = "header"
)

// TargetTokenExchangeConfig holds per-target token exchange configuration
// This is used by providers that support per-target token exchange to
// keep configuration consistent between providers
type TargetTokenExchangeConfig struct {
	// TokenURL is the token endpoint for the target
	TokenURL string `toml:"token_url"`
	// ClientID is the OAuth client ID for the target
	ClientID string `toml:"client_id"`
	// ClientSecret is the OAuth client secret for the target
	ClientSecret string `toml:"client_secret"`
	// Audience is the target audience for the exchanged token
	Audience string `toml:"audience"`
	// SubjectTokenType specifies the token type for the subject token
	// For same-realm: "urn:ietf:params:oauth:token-type:access_token"
	// For cross-realm: "urn:ietf:params:oauth:token-type:jwt"
	SubjectTokenType string `toml:"subject_token_type"`
	// SubjectIssuer is the IDP alias for cross-realm token exchange
	// Only required when exchanging tokens across Keycloak realms
	SubjectIssuer string `toml:"subject_issuer,omitempty"`
	// Scopes are optional scopes to request during token exchange
	Scopes []string `toml:"scopes,omitempty"`
	// CAFile is the path to a CA certificate file for TLS verification
	// Used when the token endpoint uses a certificate signed by a private CA
	CAFile string `toml:"ca_file,omitempty"`
	// AuthStyle specifies how client credentials are sent to the token endpoint
	// "params" (default): client_id/secret in request body
	// "header": HTTP Basic Authentication header
	AuthStyle string `toml:"auth_style,omitempty"`

	// client is a http client configured to work with the IdP for this target
	client *http.Client `toml:"-"`
}

// Validate checks that the configuration values are valid
func (c *TargetTokenExchangeConfig) Validate() error {
	if c.AuthStyle != "" && c.AuthStyle != AuthStyleParams && c.AuthStyle != AuthStyleHeader {
		return fmt.Errorf("invalid auth_style %q: must be %q or %q", c.AuthStyle, AuthStyleParams, AuthStyleHeader)
	}
	return nil
}

func (c *TargetTokenExchangeConfig) HTTPCLient() (*http.Client, error) {
	if c.client != nil {
		return c.client, nil
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()

	if c.CAFile != "" {
		tlsConfig, err := buildTlsConfigForCaFile(c.CAFile)
		if err != nil {
			return nil, err
		}

		transport.TLSClientConfig = tlsConfig
	}

	c.client = &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	return c.client, nil
}

func buildTlsConfigForCaFile(caFile string) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA file '%s': %w", caFile, err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parce CA certificate from '%s'", caFile)
	}

	tlsConfig.RootCAs = caCertPool

	return tlsConfig, nil
}
