// Package tlsutil provides shared TLS configuration utilities for parsing TLS
// settings and building crypto/tls.Config values. Callers should pass values
// from config.GetTLSMinVersionConfig / GetTLSCipherSuitesConfig.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// tlsVersionMap maps version strings to tls.Version constants
var tlsVersionMap = map[string]uint16{
	"1.0": tls.VersionTLS10,
	"1.1": tls.VersionTLS11,
	"1.2": tls.VersionTLS12,
	"1.3": tls.VersionTLS13,
}

// tlsCipherSuiteMap maps cipher suite names to their IDs.
var tlsCipherSuiteMap = func() map[string]uint16 {
	m := make(map[string]uint16)
	for _, suite := range tls.CipherSuites() {
		m[suite.Name] = suite.ID
	}
	return m
}()

// ParseTLSVersion parses a TLS version string (e.g., "1.2", "1.3") to a tls.Version constant.
// Returns tls.VersionTLS12 as default if version is empty.
func ParseTLSVersion(version string) (uint16, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return tls.VersionTLS12, nil
	}
	if v, ok := tlsVersionMap[version]; ok {
		return v, nil
	}
	validVersions := make([]string, 0, len(tlsVersionMap))
	for k := range tlsVersionMap {
		validVersions = append(validVersions, k)
	}
	slices.Sort(validVersions)
	return 0, fmt.Errorf("invalid TLS version %q: valid values are %s", version, strings.Join(validVersions, ", "))
}

// ParseTLSCipherSuites parses a slice of cipher suite names to their uint16 IDs.
// Returns nil if suites is empty or contains only empty strings (Go will use its default cipher suites).
func ParseTLSCipherSuites(suites []string) ([]uint16, error) {
	if len(suites) == 0 {
		return nil, nil
	}
	result := make([]uint16, 0, len(suites))
	var errs []error
	for _, name := range suites {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if id, ok := tlsCipherSuiteMap[name]; ok {
			result = append(result, id)
		} else {
			errs = append(errs, fmt.Errorf("unknown cipher suite %q", name))
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid cipher suites: %w", errors.Join(errs...))
	}
	// Return nil if no valid cipher suites were found (all were empty strings)
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// BuildTLSConfig creates a tls.Config from TLS settings returned by
// config.GetTLSMinVersionConfig / GetTLSCipherSuitesConfig.
// Options (e.g., RootCAs, InsecureSkipVerify) are applied last.
func BuildTLSConfig(minVersion string, cipherSuites []string, opts ...TLSConfigOption) (*tls.Config, error) {
	parsedMinVersion, err := ParseTLSVersion(minVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TLS min version: %w", err)
	}

	parsedCipherSuites, err := ParseTLSCipherSuites(cipherSuites)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TLS cipher suites: %w", err)
	}

	tlsConfig := &tls.Config{
		MinVersion:   parsedMinVersion,
		CipherSuites: parsedCipherSuites,
	}

	// Apply options
	for _, opt := range opts {
		opt(tlsConfig)
	}

	return tlsConfig, nil
}

// TLSConfigOption is a function that modifies a tls.Config.
type TLSConfigOption func(*tls.Config)

// WithRootCAs sets custom root CAs on the TLS config.
func WithRootCAs(pool *x509.CertPool) TLSConfigOption {
	return func(cfg *tls.Config) {
		cfg.RootCAs = pool
	}
}

// WithInsecureSkipVerify sets InsecureSkipVerify on the TLS config.
func WithInsecureSkipVerify(skip bool) TLSConfigOption {
	return func(cfg *tls.Config) {
		cfg.InsecureSkipVerify = skip
	}
}
