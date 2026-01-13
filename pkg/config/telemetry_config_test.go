package config

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type TelemetryConfigSuite struct {
	suite.Suite
}

func TestTelemetryConfig(t *testing.T) {
	suite.Run(t, new(TelemetryConfigSuite))
}

func (s *TelemetryConfigSuite) TestIsEnabled() {
	boolPtr := func(b bool) *bool { return &b }

	s.Run("returns false when endpoint is empty and enabled is nil", func() {
		cfg := &TelemetryConfig{}
		s.False(cfg.IsEnabled())
	})

	s.Run("returns true when endpoint is set and enabled is nil", func() {
		cfg := &TelemetryConfig{Endpoint: "http://localhost:4317"}
		s.True(cfg.IsEnabled())
	})

	s.Run("returns false when enabled is explicitly false", func() {
		cfg := &TelemetryConfig{
			Enabled:  boolPtr(false),
			Endpoint: "http://localhost:4317",
		}
		s.False(cfg.IsEnabled())
	})

	s.Run("returns true when enabled is true and endpoint is set", func() {
		cfg := &TelemetryConfig{
			Enabled:  boolPtr(true),
			Endpoint: "http://localhost:4317",
		}
		s.True(cfg.IsEnabled())
	})

	s.Run("returns false when enabled is true but endpoint is empty", func() {
		cfg := &TelemetryConfig{
			Enabled: boolPtr(true),
		}
		s.False(cfg.IsEnabled())
	})

	s.Run("env var overrides empty config endpoint", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env-endpoint:4317")
		cfg := &TelemetryConfig{}
		s.True(cfg.IsEnabled())
	})

	s.Run("explicit disable overrides env var", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env-endpoint:4317")
		cfg := &TelemetryConfig{Enabled: boolPtr(false)}
		s.False(cfg.IsEnabled())
	})
}

func (s *TelemetryConfigSuite) TestGetEndpoint() {
	s.Run("returns config value when no env var", func() {
		cfg := &TelemetryConfig{Endpoint: "http://config-endpoint:4317"}
		s.Equal("http://config-endpoint:4317", cfg.GetEndpoint())
	})

	s.Run("returns empty when no config and no env var", func() {
		cfg := &TelemetryConfig{}
		s.Equal("", cfg.GetEndpoint())
	})

	s.Run("env var takes precedence over config", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env-endpoint:4317")
		cfg := &TelemetryConfig{Endpoint: "http://config-endpoint:4317"}
		s.Equal("http://env-endpoint:4317", cfg.GetEndpoint())
	})
}

func (s *TelemetryConfigSuite) TestGetProtocol() {
	s.Run("returns config value when no env var", func() {
		cfg := &TelemetryConfig{Protocol: "http/protobuf"}
		s.Equal("http/protobuf", cfg.GetProtocol())
	})

	s.Run("returns empty when no config and no env var", func() {
		cfg := &TelemetryConfig{}
		s.Equal("", cfg.GetProtocol())
	})

	s.Run("env var takes precedence over config", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
		cfg := &TelemetryConfig{Protocol: "http/protobuf"}
		s.Equal("grpc", cfg.GetProtocol())
	})
}

func (s *TelemetryConfigSuite) TestGetTracesSampler() {
	s.Run("returns config value when no env var", func() {
		cfg := &TelemetryConfig{TracesSampler: "always_on"}
		s.Equal("always_on", cfg.GetTracesSampler())
	})

	s.Run("returns empty when no config and no env var", func() {
		cfg := &TelemetryConfig{}
		s.Equal("", cfg.GetTracesSampler())
	})

	s.Run("env var takes precedence over config", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "always_off")
		cfg := &TelemetryConfig{TracesSampler: "always_on"}
		s.Equal("always_off", cfg.GetTracesSampler())
	})
}

func (s *TelemetryConfigSuite) TestGetTracesSamplerArg() {
	floatPtr := func(f float64) *float64 { return &f }

	s.Run("returns config value as string when no env var", func() {
		cfg := &TelemetryConfig{TracesSamplerArg: floatPtr(0.5)}
		s.Equal("0.5", cfg.GetTracesSamplerArg())
	})

	s.Run("returns empty when config is nil and no env var", func() {
		cfg := &TelemetryConfig{}
		s.Equal("", cfg.GetTracesSamplerArg())
	})

	s.Run("returns 0 when config is 0.0 (valid 0% sampling)", func() {
		cfg := &TelemetryConfig{TracesSamplerArg: floatPtr(0.0)}
		s.Equal("0", cfg.GetTracesSamplerArg())
	})

	s.Run("env var takes precedence over config", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "0.1")
		cfg := &TelemetryConfig{TracesSamplerArg: floatPtr(0.5)}
		s.Equal("0.1", cfg.GetTracesSamplerArg())
	})

	s.Run("formats float correctly without trailing zeros", func() {
		cfg := &TelemetryConfig{TracesSamplerArg: floatPtr(1.0)}
		s.Equal("1", cfg.GetTracesSamplerArg())
	})
}
