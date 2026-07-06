package tokenexchange

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/tlsutil"
)

const (
	// AuthStyleParams sends client_id and client_secret in the request body
	AuthStyleParams = "params"
	// AuthStyleHeader sends client credentials as HTTP Basic Authentication header
	AuthStyleHeader = "header"
	// AuthStyleAssertion sends a signed JWT client assertion (RFC 7523)
	AuthStyleAssertion = "assertion"
	// AuthStyleFederated reads a JWT from an external identity provider token file
	// and sends it as a client assertion (workload identity federation)
	AuthStyleFederated = "federated"
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
	// FederatedTokenFile is the path to a file containing a JWT from an external
	// identity provider (e.g., SPIRE JWT-SVID). Used with AuthStyleFederated.
	// The file is re-read on each token request to support token rotation.
	FederatedTokenFile string `toml:"federated_token_file,omitempty"`
	// TLSMinVersion is the effective global TLS min version (from GetTLSMinVersionConfig).
	TLSMinVersion string `toml:"-"`
	// TLSCipherSuites is the effective global TLS cipher suites (from GetTLSCipherSuitesConfig).
	TLSCipherSuites []string `toml:"-"`

	// client is a http client configured to work with the IdP for this target
	client *http.Client `toml:"-"`
	// clientCAFile tracks which CAFile was used to build the cached client
	clientCAFile string `toml:"-"`
	// clientTLSMinVersion tracks the TLS min version used to build the cached client
	clientTLSMinVersion string `toml:"-"`
	// clientTLSCipherSuites tracks the TLS cipher suites used to build the cached client
	clientTLSCipherSuites []string `toml:"-"`
	// cachedAssertion stores the most recently generated JWT assertion
	cachedAssertion string `toml:"-"`
	// cachedAssertionExpiry is when the cached assertion expires
	cachedAssertionExpiry time.Time `toml:"-"`
	// assertionMutex protects assertion caching from race conditions
	assertionMutex sync.Mutex `toml:"-"`
	// clientMutex protects HTTP client creation from race conditions
	clientMutex sync.Mutex `toml:"-"`

	// requireTLS, when set, returns true while TLS is required. The transport
	// consults it live per request via currentRequireTLS.
	requireTLS func() bool `toml:"-"`
}

// SetRequireTLS installs the TLS enforcer. HTTPClient() always wraps its
// transport to read it live per request, so this may be (re)set at any time —
// before or after the client is memoized, e.g. on a SIGHUP toggle.
func (c *TargetTokenExchangeConfig) SetRequireTLS(enforcer func() bool) {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()
	c.requireTLS = enforcer
}

// currentRequireTLS reports whether TLS is required now, reading the latest
// enforcer under clientMutex.
func (c *TargetTokenExchangeConfig) currentRequireTLS() bool {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()
	return c.requireTLS != nil && c.requireTLS()
}

// tlsEnforcingTransport rejects non-HTTPS requests when RequireTLS returns true.
// Mirrors pkg/config.TLSEnforcingTransport (duplicated to avoid an import cycle),
// but RequireTLS is wired to currentRequireTLS for live, per-request evaluation.
type tlsEnforcingTransport struct {
	Base       http.RoundTripper
	RequireTLS func() bool
}

func (t *tlsEnforcingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.RequireTLS != nil && t.RequireTLS() && !isSecureHTTPScheme(req.URL.Scheme) {
		klogutil.FromContext(req.Context()).V(1).Info("require_tls: blocked request to host", "host", req.URL.Host)
		return nil, fmt.Errorf("require_tls is enabled but request to %s uses %q scheme (secure scheme required)",
			req.URL.Host, req.URL.Scheme)
	}
	return t.Base.RoundTrip(req)
}

func isSecureHTTPScheme(scheme string) bool {
	switch scheme {
	case "https", "wss":
		return true
	default:
		return false
	}
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
	case AuthStyleFederated:
		if c.FederatedTokenFile == "" {
			return fmt.Errorf("federated_token_file is required when auth_style is %q", AuthStyleFederated)
		}
	default:
		return fmt.Errorf("invalid auth_style %q: must be %q, %q, %q, or %q", c.AuthStyle, AuthStyleParams, AuthStyleHeader, AuthStyleAssertion, AuthStyleFederated)
	}
	return nil
}

// HTTPClient returns a memoized *http.Client configured to talk to the IdP.
// The client is rebuilt (and the previous one's idle connections closed) when
// CAFile changes, so a CA rotation takes effect on the next call. Concurrent
// calls are safe, but CAFile itself must not be mutated concurrently with this
// method: writes to CAFile are not guarded by clientMutex, so a caller that
// reuses a TargetTokenExchangeConfig across CA changes must set CAFile and call
// HTTPClient from the same goroutine (the provider builds a fresh config per
// cache-key change, which satisfies this).
func (c *TargetTokenExchangeConfig) HTTPClient() (*http.Client, error) {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()

	if c.client != nil && c.clientCAFile == c.CAFile &&
		c.clientTLSMinVersion == c.TLSMinVersion &&
		slices.Equal(c.clientTLSCipherSuites, c.TLSCipherSuites) {
		return c.client, nil
	}

	baseTransport := http.DefaultTransport.(*http.Transport).Clone()

	// Build TLS options for outbound client
	var tlsOpts []tlsutil.TLSConfigOption

	if c.CAFile != "" {
		caCert, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file '%s': %w", c.CAFile, err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate from '%s'", c.CAFile)
		}

		tlsOpts = append(tlsOpts, tlsutil.WithRootCAs(caCertPool))
	}

	// Build TLS config from stored min version and cipher suites.
	tlsConfig, err := tlsutil.BuildTLSConfig(c.TLSMinVersion, c.TLSCipherSuites, tlsOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS config: %w", err)
	}

	baseTransport.TLSClientConfig = tlsConfig

	// Always wrap so require_tls is enforced live per request via
	// currentRequireTLS, regardless of whether an enforcer was set before this
	// client was memoized. currentRequireTLS is a no-op until SetRequireTLS
	// installs one, at the cost of a clientMutex lock per request.
	transport := &tlsEnforcingTransport{Base: baseTransport, RequireTLS: c.currentRequireTLS}

	if c.client != nil {
		c.client.CloseIdleConnections()
	}
	c.client = &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
	c.clientCAFile = c.CAFile
	c.clientTLSMinVersion = c.TLSMinVersion
	c.clientTLSCipherSuites = append([]string(nil), c.TLSCipherSuites...)

	return c.client, nil
}

// CloseIdleConnections closes any idle keep-alive connections held by the
// memoized HTTP client. It is a no-op if no client has been built yet. Callers
// that discard a TargetTokenExchangeConfig (e.g. when a reload produces a fresh
// config) should call this first so the old client's connections are released
// promptly instead of lingering until garbage collection.
func (c *TargetTokenExchangeConfig) CloseIdleConnections() {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
}
