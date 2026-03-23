package tokenexchange

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	// AuthStyleParams sends client_id and client_secret in the request body
	AuthStyleParams = "params"
	// AuthStyleHeader sends client credentials as HTTP Basic Authentication header
	AuthStyleHeader = "header"
	// AuthStyleAssertion sends a signed JWT client assertion (RFC 7523)
	AuthStyleAssertion = "assertion"
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
	// "assertion": JWT client assertion (RFC 7523)
	AuthStyle string `toml:"auth_style,omitempty"`
	// ClientCertFile is the path to the client certificate PEM file
	// Used with AuthStyleAssertion for JWT client assertion authentication
	ClientCertFile string `toml:"client_cert_file,omitempty"`
	// ClientKeyFile is the path to the client private key PEM file
	// Used with AuthStyleAssertion for JWT client assertion authentication
	ClientKeyFile string `toml:"client_key_file,omitempty"`
	// AssertionLifetime is the validity duration for generated JWT assertions
	// Defaults to 5 minutes if not specified
	AssertionLifetime time.Duration `toml:"assertion_lifetime,omitempty"`

	// client is a http client configured to work with the IdP for this target
	client *http.Client `toml:"-"`
	// cachedAssertion stores the most recently generated JWT assertion
	cachedAssertion string `toml:"-"`
	// cachedAssertionExpiry is when the cached assertion expires
	cachedAssertionExpiry time.Time `toml:"-"`
	// assertionMutex protects assertion caching from race conditions
	assertionMutex sync.Mutex `toml:"-"`
	// clientMutex protects HTTP client creation from race conditions
	clientMutex sync.Mutex `toml:"-"`
}

// Validate checks that the configuration values are valid
func (c *TargetTokenExchangeConfig) Validate() error {
	switch c.AuthStyle {
	case "", AuthStyleParams, AuthStyleHeader:
		// valid
	case AuthStyleAssertion:
		if c.ClientCertFile == "" {
			return fmt.Errorf("client_cert_file is required when auth_style is %q", AuthStyleAssertion)
		}
		if c.ClientKeyFile == "" {
			return fmt.Errorf("client_key_file is required when auth_style is %q", AuthStyleAssertion)
		}
	default:
		return fmt.Errorf("invalid auth_style %q: must be %q, %q, or %q", c.AuthStyle, AuthStyleParams, AuthStyleHeader, AuthStyleAssertion)
	}
	return nil
}

func (c *TargetTokenExchangeConfig) HTTPClient() (*http.Client, error) {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()

	if c.client != nil {
		return c.client, nil
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()

	// Always set MinVersion for security, regardless of CAFile
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if c.CAFile != "" {
		caCert, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file '%s': %w", c.CAFile, err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate from '%s'", c.CAFile)
		}

		tlsConfig.RootCAs = caCertPool
	}

	transport.TLSClientConfig = tlsConfig

	c.client = &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	return c.client, nil
}
