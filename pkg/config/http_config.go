package config

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
}
