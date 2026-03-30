package config

// HTTPConfig contains HTTP server configuration options for timeouts and size limits.
type HTTPConfig struct {
	// ReadTimeout is the maximum duration for reading the entire request,
	// including the body. A zero or negative value means no timeout.
	ReadTimeout Duration `toml:"read_timeout,omitempty"`

	// IdleTimeout is the maximum duration to wait for the next request when keep-alives are enabled.
	// A zero or negative value means no timeout.
	IdleTimeout Duration `toml:"idle_timeout,omitempty"`

	// ReadHeaderTimeout is the amount of time allowed to read request headers.
	// This is the primary defense against Slowloris attacks.
	ReadHeaderTimeout Duration `toml:"read_header_timeout,omitempty"`

	// MaxHeaderBytes is the maximum size of request headers in bytes.
	// Type is int to match http.Server.MaxHeaderBytes.
	MaxHeaderBytes int `toml:"max_header_bytes,omitzero"`

	// MaxBodyBytes is the maximum size of request body in bytes.
	// Enforced via middleware using http.MaxBytesReader.
	// Type is int64 to match http.MaxBytesReader signature.
	MaxBodyBytes int64 `toml:"max_body_bytes,omitzero"`
}
