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

func boolPtr(b bool) *bool { return &b }

func (s *TelemetryConfigSuite) TestIsEnabled() {
	s.Run("returns false when endpoint is empty and enabled is nil", func() {
		cfg := New()
		s.False(cfg.Telemetry.IsEnabled())
	})

	s.Run("returns true when endpoint is set and enabled is nil", func() {
		cfg := New()
		cfg.Telemetry.Endpoint.SetForTest("http://localhost:4317")
		s.True(cfg.Telemetry.IsEnabled())
	})

	s.Run("returns false when enabled is explicitly false", func() {
		cfg := New()
		cfg.Telemetry.Enabled.SetForTest(boolPtr(false))
		cfg.Telemetry.Endpoint.SetForTest("http://localhost:4317")
		s.False(cfg.Telemetry.IsEnabled())
	})

	s.Run("returns true when enabled is true and endpoint is set", func() {
		cfg := New()
		cfg.Telemetry.Enabled.SetForTest(boolPtr(true))
		cfg.Telemetry.Endpoint.SetForTest("http://localhost:4317")
		s.True(cfg.Telemetry.IsEnabled())
	})

	s.Run("returns false when enabled is true but endpoint is empty", func() {
		cfg := New()
		cfg.Telemetry.Enabled.SetForTest(boolPtr(true))
		s.False(cfg.Telemetry.IsEnabled())
	})

	s.Run("env var overrides empty config endpoint", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env-endpoint:4317")
		cfg, err := ReadToml(nil)
		s.Require().NoError(err)
		s.True(cfg.Telemetry.IsEnabled())
	})

	s.Run("explicit disable overrides env var", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env-endpoint:4317")
		cfg, err := ReadToml([]byte(`
[telemetry]
enabled = false
`))
		s.Require().NoError(err)
		s.False(cfg.Telemetry.IsEnabled())
	})
}

func (s *TelemetryConfigSuite) TestGetEndpoint() {
	s.Run("returns config value when no env var", func() {
		cfg := New()
		cfg.Telemetry.Endpoint.SetForTest("http://config-endpoint:4317")
		s.Equal("http://config-endpoint:4317", cfg.Telemetry.Endpoint.Get())
	})

	s.Run("returns empty when no config and no env var", func() {
		cfg := New()
		s.Equal("", cfg.Telemetry.Endpoint.Get())
	})

	s.Run("env var takes precedence over config", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env-endpoint:4317")
		cfg, err := ReadToml([]byte(`
[telemetry]
endpoint = "http://config-endpoint:4317"
`))
		s.Require().NoError(err)
		s.Equal("http://env-endpoint:4317", cfg.Telemetry.Endpoint.Get())
	})
}

func (s *TelemetryConfigSuite) TestGetProtocol() {
	s.Run("returns config value when no env var", func() {
		cfg := New()
		cfg.Telemetry.Protocol.SetForTest("http/protobuf")
		s.Equal("http/protobuf", cfg.Telemetry.Protocol.Get())
	})

	s.Run("defaults to grpc when not set", func() {
		cfg := New()
		s.Equal("grpc", cfg.Telemetry.Protocol.Get())
	})

	s.Run("env var takes precedence over config", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
		cfg, err := ReadToml([]byte(`
[telemetry]
protocol = "http/protobuf"
`))
		s.Require().NoError(err)
		s.Equal("grpc", cfg.Telemetry.Protocol.Get())
	})
}

func (s *TelemetryConfigSuite) TestGetTracesSampler() {
	s.Run("returns config value when no env var", func() {
		cfg := New()
		cfg.Telemetry.TracesSampler.SetForTest("always_on")
		s.Equal("always_on", cfg.Telemetry.TracesSampler.Get())
	})

	s.Run("returns empty when no config and no env var", func() {
		cfg := New()
		s.Equal("", cfg.Telemetry.TracesSampler.Get())
	})

	s.Run("env var takes precedence over config", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "always_off")
		cfg, err := ReadToml([]byte(`
[telemetry]
traces_sampler = "always_on"
`))
		s.Require().NoError(err)
		s.Equal("always_off", cfg.Telemetry.TracesSampler.Get())
	})
}

func (s *TelemetryConfigSuite) TestGetTracesSamplerArg() {
	floatPtr := func(f float64) *float64 { return &f }

	s.Run("returns config value when no env var", func() {
		cfg := New()
		cfg.Telemetry.TracesSamplerArg.SetForTest(floatPtr(0.5))
		s.Require().NotNil(cfg.Telemetry.TracesSamplerArg.Get())
		s.Equal(0.5, *cfg.Telemetry.TracesSamplerArg.Get())
	})

	s.Run("returns nil when unset", func() {
		cfg := New()
		s.Nil(cfg.Telemetry.TracesSamplerArg.Get())
	})

	s.Run("returns 0 when config is 0.0 (valid 0% sampling)", func() {
		cfg := New()
		cfg.Telemetry.TracesSamplerArg.SetForTest(floatPtr(0.0))
		s.Require().NotNil(cfg.Telemetry.TracesSamplerArg.Get())
		s.Equal(0.0, *cfg.Telemetry.TracesSamplerArg.Get())
	})

	s.Run("env var takes precedence over config", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "0.1")
		cfg, err := ReadToml([]byte(`
[telemetry]
traces_sampler_arg = 0.5
`))
		s.Require().NoError(err)
		s.Require().NotNil(cfg.Telemetry.TracesSamplerArg.Get())
		s.Equal(0.1, *cfg.Telemetry.TracesSamplerArg.Get())
	})
}
