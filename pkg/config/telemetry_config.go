package config

import (
	"os"
	"strconv"
)

// TelemetryConfig contains OpenTelemetry configuration options.
// Values can be set via TOML config file or environment variables.
// Environment variables take precedence over TOML config values.
type TelemetryConfig struct {
	// Enabled explicitly enables or disables telemetry.
	// If nil (not set), telemetry is auto-enabled when Endpoint is configured.
	// If explicitly set to false, telemetry is disabled even if env vars are set.
	Enabled *bool `toml:"enabled,omitempty"`

	// Endpoint is the OTLP endpoint URL (e.g., "http://localhost:4317").
	// Can be overridden by OTEL_EXPORTER_OTLP_ENDPOINT env var.
	Endpoint string `toml:"endpoint,omitempty"`

	// Protocol specifies the OTLP protocol: "grpc" (default) or "http/protobuf".
	// Can be overridden by OTEL_EXPORTER_OTLP_PROTOCOL env var.
	Protocol string `toml:"protocol,omitempty"`

	// TracesSampler specifies the trace sampling strategy.
	// Supported values: "always_on", "always_off", "traceidratio",
	// "parentbased_always_on", "parentbased_traceidratio".
	// Can be overridden by OTEL_TRACES_SAMPLER env var.
	TracesSampler string `toml:"traces_sampler,omitempty"`

	// TracesSamplerArg is the sampling ratio for ratio-based samplers (0.0 to 1.0).
	// Can be overridden by OTEL_TRACES_SAMPLER_ARG env var.
	TracesSamplerArg *float64 `toml:"traces_sampler_arg,omitempty"`
}

// IsEnabled returns true if telemetry should be enabled.
// Logic:
//   - If Enabled is explicitly set to false, return false (explicit disable)
//   - If Enabled is explicitly set to true, return true only if endpoint is available
//   - If Enabled is nil (not set), return true if endpoint is available (auto-enable)
func (c *TelemetryConfig) IsEnabled() bool {
	// Explicit disable takes precedence
	if c.Enabled != nil && !*c.Enabled {
		return false
	}

	endpoint := c.GetEndpoint()
	return endpoint != ""
}

// GetEndpoint returns the OTLP endpoint.
// Environment variable OTEL_EXPORTER_OTLP_ENDPOINT takes precedence over config.
func (c *TelemetryConfig) GetEndpoint() string {
	if envVal := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); envVal != "" {
		return envVal
	}
	return c.Endpoint
}

// GetProtocol returns the OTLP protocol.
// Environment variable OTEL_EXPORTER_OTLP_PROTOCOL takes precedence over config.
func (c *TelemetryConfig) GetProtocol() string {
	if envVal := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"); envVal != "" {
		return envVal
	}
	return c.Protocol
}

// GetTracesSampler returns the trace sampler type.
// Environment variable OTEL_TRACES_SAMPLER takes precedence over config.
func (c *TelemetryConfig) GetTracesSampler() string {
	if envVal := os.Getenv("OTEL_TRACES_SAMPLER"); envVal != "" {
		return envVal
	}
	return c.TracesSampler
}

// GetTracesSamplerArg returns the trace sampler argument as a string.
// Environment variable OTEL_TRACES_SAMPLER_ARG takes precedence over config.
func (c *TelemetryConfig) GetTracesSamplerArg() string {
	if envVal := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); envVal != "" {
		return envVal
	}

	// nil means not set, return empty string
	if c.TracesSamplerArg == nil {
		return ""
	}
	// Return the value even if it's 0.0 (valid ratio for 0% sampling)
	return strconv.FormatFloat(*c.TracesSamplerArg, 'f', -1, 64)
}
