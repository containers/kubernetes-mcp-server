package config

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"

	"k8s.io/klog/v2"
)

// ValidateURLRequiresTLS validates that a URL uses a secure scheme when TLS is required.
// Returns nil if the URL is empty. Returns an error if the URL does not use a secure scheme.
// This provides Layer 1 (config-time) validation for fail-fast feedback.
func ValidateURLRequiresTLS(urlStr string, fieldName string) error {
	if urlStr == "" {
		return nil
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", fieldName, err)
	}
	if !isSecureHTTPScheme(u.Scheme) {
		return fmt.Errorf("require_tls is enabled but %s uses %q scheme (secure scheme required)", fieldName, u.Scheme)
	}
	return nil
}

// ValidateURLsRequireTLS validates multiple URLs use a secure scheme.
// The map keys are field names, values are the URLs to validate.
// All URLs are validated and errors are combined.
// Keys are sorted for deterministic error ordering.
func ValidateURLsRequireTLS(urls map[string]string) error {
	var errs []error
	for _, fieldName := range slices.Sorted(maps.Keys(urls)) {
		if err := ValidateURLRequiresTLS(urls[fieldName], fieldName); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("TLS validation failed: %w", errors.Join(errs...))
}

// TLSEnforcingTransport wraps an http.RoundTripper and rejects non-HTTPS requests
// when RequireTLS returns true. This provides Layer 2 (runtime) enforcement as
// defense-in-depth, catching any URLs that might have been missed during config validation.
// The RequireTLS function is called per-request, allowing dynamic config changes (e.g., SIGHUP).
type TLSEnforcingTransport struct {
	Base       http.RoundTripper
	RequireTLS func() bool
}

func (t *TLSEnforcingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.RequireTLS != nil && t.RequireTLS() && !isSecureHTTPScheme(req.URL.Scheme) {
		klog.V(1).Infof("require_tls: blocked request to %s", req.URL.Host)
		return nil, fmt.Errorf("require_tls is enabled but request to %s uses %q scheme (secure scheme required)",
			req.URL.Host, req.URL.Scheme)
	}
	return t.Base.RoundTrip(req)
}

// isSecureHTTPScheme returns true for TLS-encrypted HTTP protocols.
// Currently only http/https and ws/wss are used in this codebase.
func isSecureHTTPScheme(scheme string) bool {
	switch scheme {
	case "https", "wss":
		return true
	default:
		return false
	}
}

// NewTLSEnforcingTransport creates a transport that enforces HTTPS when requireTLS returns true.
// The requireTLS function is called on each request, allowing dynamic configuration changes.
func NewTLSEnforcingTransport(base http.RoundTripper, requireTLS func() bool) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &TLSEnforcingTransport{
		Base:       base,
		RequireTLS: requireTLS,
	}
}

// NewTLSEnforcingClient creates an HTTP client that enforces HTTPS when requireTLS returns true.
// The requireTLS function is called on each request, allowing dynamic configuration changes.
func NewTLSEnforcingClient(base *http.Client, requireTLS func() bool) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	transport := base.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{
		Transport:     NewTLSEnforcingTransport(transport, requireTLS),
		CheckRedirect: base.CheckRedirect,
		Jar:           base.Jar,
		Timeout:       base.Timeout,
	}
}
