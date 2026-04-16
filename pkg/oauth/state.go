package oauth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"slices"
	"sync/atomic"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/coreos/go-oidc/v3/oidc"
)

// Snapshot is an immutable point-in-time capture of OAuth-related state.
// It is swapped atomically via State so all consumers see a consistent view.
type Snapshot struct {
	OIDCProvider                     *oidc.Provider
	HTTPClient                       *http.Client
	AuthorizationURL                 string
	CertificateAuthority             string
	OAuthScopes                      []string
	DisableDynamicClientRegistration bool
}

// HasProviderConfigChanged reports whether the fields that require OIDC provider
// and HTTP client recreation have changed between two snapshots.
func (s *Snapshot) HasProviderConfigChanged(other *Snapshot) bool {
	if s == nil || other == nil {
		return s != other
	}
	return s.AuthorizationURL != other.AuthorizationURL ||
		s.CertificateAuthority != other.CertificateAuthority
}

// HasWellKnownConfigChanged reports whether any WellKnown-serving fields changed.
func (s *Snapshot) HasWellKnownConfigChanged(other *Snapshot) bool {
	if s == nil || other == nil {
		return s != other
	}
	if s.HasProviderConfigChanged(other) {
		return true
	}
	if s.DisableDynamicClientRegistration != other.DisableDynamicClientRegistration {
		return true
	}
	if !slices.Equal(s.OAuthScopes, other.OAuthScopes) {
		return true
	}
	return false
}

// State holds the current OAuth snapshot and allows atomic, lock-free reads.
type State struct {
	ref atomic.Pointer[Snapshot]
}

// NewState creates a new State initialized with the given snapshot.
func NewState(snap *Snapshot) *State {
	s := &State{}
	s.ref.Store(snap)
	return s
}

// Load returns the current snapshot. Safe for concurrent use.
func (s *State) Load() *Snapshot {
	return s.ref.Load()
}

// Store atomically replaces the current snapshot.
func (s *State) Store(snap *Snapshot) {
	s.ref.Store(snap)
}

// SnapshotFromConfig extracts OAuth-relevant fields from a StaticConfig and
// pairs them with the corresponding OIDC provider and HTTP client.
func SnapshotFromConfig(cfg *config.StaticConfig, provider *oidc.Provider, httpClient *http.Client) *Snapshot {
	return &Snapshot{
		OIDCProvider:                     provider,
		HTTPClient:                       httpClient,
		AuthorizationURL:                 cfg.AuthorizationURL,
		CertificateAuthority:             cfg.CertificateAuthority,
		OAuthScopes:                      cfg.OAuthScopes,
		DisableDynamicClientRegistration: cfg.DisableDynamicClientRegistration,
	}
}

// CreateOIDCProviderAndClient builds an OIDC provider and HTTP client from config.
// Returns (nil, nil, nil) when AuthorizationURL is empty (OAuth not configured).
func CreateOIDCProviderAndClient(cfg *config.StaticConfig) (*oidc.Provider, *http.Client, error) {
	if cfg.AuthorizationURL == "" {
		return nil, nil, nil
	}

	ctx := context.Background()
	var httpClient *http.Client

	if cfg.CertificateAuthority != "" {
		caCert, err := os.ReadFile(cfg.CertificateAuthority)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read CA certificate from %s: %w", cfg.CertificateAuthority, err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, nil, fmt.Errorf("failed to append CA certificate from %s to pool", cfg.CertificateAuthority)
		}
		if caCertPool.Equal(x509.NewCertPool()) {
			caCertPool = nil
		}
		var transport http.RoundTripper = &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    caCertPool,
			},
		}
		transport = config.NewTLSEnforcingTransport(transport, cfg.IsRequireTLS)
		httpClient = &http.Client{Transport: transport}
	} else {
		httpClient = config.NewTLSEnforcingClient(nil, cfg.IsRequireTLS)
	}

	ctx = oidc.ClientContext(ctx, httpClient)
	provider, err := oidc.NewProvider(ctx, cfg.AuthorizationURL)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to setup OIDC provider: %w", err)
	}

	return provider, httpClient, nil
}
