package config

import "fmt"

// DefaultRateLimitBurst is the default burst size used when rate_limit_rps is
// set but rate_limit_burst is not specified (zero value).
const DefaultRateLimitBurst = 10

// HTTPConfig contains HTTP server configuration options for security.
type HTTPConfig struct {
	// ReadHeaderTimeout is the amount of time allowed to read request headers.
	// This is the primary defense against Slowloris attacks.
	ReadHeaderTimeout Duration `toml:"read_header_timeout,omitempty"`

	// MaxBodyBytes is the maximum size of request body in bytes.
	// MCP payloads (tools/call with Kubernetes manifests) can be large,
	// so the default is 16MB to accommodate CRDs and ConfigMaps.
	// Type is int64 to match http.MaxBytesReader signature.
	MaxBodyBytes int64 `toml:"max_body_bytes,omitzero"`

	// RateLimitRPS is the maximum number of requests per second per session.
	// When set to 0 (default), rate limiting is disabled.
	RateLimitRPS float64 `toml:"rate_limit_rps,omitzero"`

	// RateLimitBurst is the maximum burst size for rate limiting per session.
	// Allows short bursts of requests above the rate limit.
	// Only effective when rate_limit_rps > 0.
	// When zero, the rate limiting middleware applies DefaultRateLimitBurst.
	RateLimitBurst int `toml:"rate_limit_burst,omitzero"`
}

// Validate checks HTTPConfig for invalid values.
// It rejects negative RateLimitRPS and negative RateLimitBurst.
func (c *HTTPConfig) Validate() error {
	if c.RateLimitRPS < 0 {
		return fmt.Errorf("rate_limit_rps must not be negative (got %v)", c.RateLimitRPS)
	}
	if c.RateLimitBurst < 0 {
		return fmt.Errorf("rate_limit_burst must not be negative (got %d)", c.RateLimitBurst)
	}
	return nil
}
