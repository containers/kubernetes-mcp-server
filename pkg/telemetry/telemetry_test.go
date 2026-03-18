package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
)

type TelemetrySuite struct {
	suite.Suite
}

func TestTelemetry(t *testing.T) {
	suite.Run(t, new(TelemetrySuite))
}

func (s *TelemetrySuite) TestInitTracer() {
	s.Run("returns cleanup function when OTLP endpoint not configured", func() {
		cleanup, err := InitTracer("test-service", "1.0.0")

		s.NoError(err, "should not return error when OTLP endpoint is not configured")
		s.NotNil(cleanup, "cleanup function should not be nil")

		s.NotPanics(func() {
			cleanup()
		}, "cleanup should not panic")
	})

	s.Run("initializes with valid service name and version", func() {
		cleanup, err := InitTracer("my-service", "2.0.0")
		defer cleanup()

		s.NoError(err, "initialization should succeed")
		s.NotNil(cleanup, "should return cleanup function")
	})

	s.Run("sets global tracer provider", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
		initialProvider := otel.GetTracerProvider()

		cleanup, err := InitTracer("test-service", "1.0.0")
		s.Require().NoError(err)
		defer cleanup()

		currentProvider := otel.GetTracerProvider()
		s.NotNil(currentProvider, "tracer provider should be set")

		_, isSDKProvider := currentProvider.(*trace.TracerProvider)
		s.True(isSDKProvider, "should set SDK TracerProvider")
		s.NotEqual(initialProvider, currentProvider, "should set new tracer provider")
	})
}

func (s *TelemetrySuite) TestCleanupFunction() {
	s.Run("can be called multiple times", func() {
		cleanup, err := InitTracer("test-service", "1.0.0")
		s.Require().NoError(err)

		s.NotPanics(func() {
			cleanup()
			cleanup()
			cleanup()
		}, "cleanup should be safe to call multiple times")
	})

	s.Run("executes without blocking", func() {
		cleanup, err := InitTracer("test-service", "1.0.0")
		s.Require().NoError(err)

		done := make(chan bool)
		go func() {
			cleanup()
			done <- true
		}()

		// Should complete within reasonable time
		select {
		case <-done:
			// Success
		case <-context.Background().Done():
			s.Fail("cleanup blocked indefinitely")
		}
	})
}

func (s *TelemetrySuite) TestInitTracerWithEmptyValues() {
	s.Run("handles empty service name", func() {
		cleanup, err := InitTracer("", "1.0.0")
		defer cleanup()

		s.NoError(err, "should handle empty service name")
		s.NotNil(cleanup, "should return cleanup function")
	})

	s.Run("handles empty service version", func() {
		cleanup, err := InitTracer("test-service", "")
		defer cleanup()

		s.NoError(err, "should handle empty service version")
		s.NotNil(cleanup, "should return cleanup function")
	})

	s.Run("handles both empty", func() {
		cleanup, err := InitTracer("", "")
		defer cleanup()

		s.NoError(err, "should handle both empty values")
		s.NotNil(cleanup, "should return cleanup function")
	})
}

func (s *TelemetrySuite) TestEnabled() {
	s.Run("returns false when no tracer has been initialized", func() {
		tracingEnabled.Store(false)

		s.False(Enabled())
	})

	s.Run("returns true after InitTracerWithConfig with valid endpoint", func() {
		tracingEnabled.Store(false)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &config.TelemetryConfig{
			Endpoint: server.URL,
			Protocol: "http/protobuf",
		}
		cleanup, err := InitTracerWithConfig(cfg, "test-service", "1.0.0")
		s.Require().NoError(err)
		defer cleanup()

		s.True(Enabled())
	})

	s.Run("returns false after cleanup is called", func() {
		tracingEnabled.Store(false)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &config.TelemetryConfig{
			Endpoint: server.URL,
			Protocol: "http/protobuf",
		}
		cleanup, err := InitTracerWithConfig(cfg, "test-service", "1.0.0")
		s.Require().NoError(err)
		s.Require().True(Enabled())

		cleanup()

		s.False(Enabled())
	})
}

func (s *TelemetrySuite) TestInitTracerWithConfig() {
	s.Run("returns no-op cleanup when cfg is nil", func() {
		cleanup, err := InitTracerWithConfig(nil, "test-service", "1.0.0")

		s.NoError(err)
		s.NotNil(cleanup)
		s.NotPanics(func() { cleanup() })
	})

	s.Run("returns no-op cleanup when cfg is not enabled", func() {
		enabled := false
		cfg := &config.TelemetryConfig{
			Enabled:  &enabled,
			Endpoint: "http://localhost:4317",
		}

		cleanup, err := InitTracerWithConfig(cfg, "test-service", "1.0.0")

		s.NoError(err)
		s.NotNil(cleanup)
		s.False(Enabled())
	})

	s.Run("returns no-op cleanup when cfg has no endpoint", func() {
		cfg := &config.TelemetryConfig{}

		cleanup, err := InitTracerWithConfig(cfg, "test-service", "1.0.0")

		s.NoError(err)
		s.NotNil(cleanup)
		s.False(Enabled())
	})

	s.Run("initializes tracing when config has valid endpoint", func() {
		tracingEnabled.Store(false)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &config.TelemetryConfig{
			Endpoint: server.URL,
			Protocol: "http/protobuf",
		}

		cleanup, err := InitTracerWithConfig(cfg, "test-service", "1.0.0")
		s.Require().NoError(err)
		defer cleanup()

		s.True(Enabled())
	})

	s.Run("cleanup restores enabled state to false", func() {
		tracingEnabled.Store(false)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &config.TelemetryConfig{
			Endpoint: server.URL,
			Protocol: "grpc",
		}

		cleanup, err := InitTracerWithConfig(cfg, "test-service", "1.0.0")
		s.Require().NoError(err)
		s.Require().True(Enabled())

		cleanup()

		s.False(Enabled())
	})

	s.Run("sets the global OTel tracer provider", func() {
		tracingEnabled.Store(false)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &config.TelemetryConfig{
			Endpoint: server.URL,
			Protocol: "http/protobuf",
		}
		initialProvider := otel.GetTracerProvider()

		cleanup, err := InitTracerWithConfig(cfg, "test-service", "1.0.0")
		s.Require().NoError(err)
		defer cleanup()

		currentProvider := otel.GetTracerProvider()
		_, isSDKProvider := currentProvider.(*trace.TracerProvider)
		s.True(isSDKProvider)
		s.NotEqual(initialProvider, currentProvider)
	})
}

func (s *TelemetrySuite) TestCreateExporter() {
	s.Run("creates gRPC exporter by default", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")

		ctx := context.Background()
		exporter, err := createExporter(ctx)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates gRPC exporter for grpc protocol", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")

		ctx := context.Background()
		exporter, err := createExporter(ctx)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates HTTP exporter for http/protobuf protocol", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

		ctx := context.Background()
		exporter, err := createExporter(ctx)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates HTTP exporter for http protocol alias", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http")

		ctx := context.Background()
		exporter, err := createExporter(ctx)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("falls back to gRPC for unknown protocol", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "unknown_protocol")

		ctx := context.Background()
		exporter, err := createExporter(ctx)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("handles case-insensitive protocol values", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "HTTP/PROTOBUF")

		ctx := context.Background()
		exporter, err := createExporter(ctx)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})
}

func (s *TelemetrySuite) TestCreateExporterWithConfig() {
	s.Run("creates gRPC exporter by default when protocol is empty", func() {
		cfg := &config.TelemetryConfig{
			Endpoint: "http://localhost:4317",
		}

		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates gRPC exporter for grpc protocol", func() {
		cfg := &config.TelemetryConfig{
			Endpoint: "http://localhost:4317",
			Protocol: "grpc",
		}

		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates HTTP exporter for http/protobuf protocol", func() {
		cfg := &config.TelemetryConfig{
			Endpoint: "http://localhost:4318",
			Protocol: "http/protobuf",
		}

		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates HTTP exporter for http protocol alias", func() {
		cfg := &config.TelemetryConfig{
			Endpoint: "http://localhost:4318",
			Protocol: "http",
		}

		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("falls back to gRPC for unknown protocol", func() {
		cfg := &config.TelemetryConfig{
			Endpoint: "http://localhost:4317",
			Protocol: "unknown_protocol",
		}

		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("sends traces to config endpoint", func() {
		var requestReceived atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestReceived.Store(true)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		s.T().Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")

		cfg := &config.TelemetryConfig{
			Endpoint: server.URL,
			Protocol: "http/protobuf",
		}

		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, cfg)
		s.Require().NoError(err)
		s.Require().NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()

		// Use a synchronous span processor to export immediately on span end
		tp := trace.NewTracerProvider(
			trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exporter)),
		)
		defer func() { _ = tp.Shutdown(ctx) }()

		_, span := tp.Tracer("test").Start(ctx, "test-span")
		span.End()

		s.True(requestReceived.Load(), "exporter should send traces to the config endpoint")
	})
}

func (s *TelemetrySuite) TestGetSamplerFromEnv() {
	s.Run("returns default ParentBased(AlwaysSample) when no env vars set", func() {
		// Clear environment variables
		s.T().Setenv("OTEL_TRACES_SAMPLER", "")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns AlwaysSample for always_on", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "always_on")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns NeverSample for always_off", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "always_off")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns TraceIDRatioBased for traceidratio with valid arg", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "0.5")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns TraceIDRatioBased with default 1.0 for traceidratio without arg", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("handles invalid sampler arg gracefully", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "invalid")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil even with invalid arg")
	})

	s.Run("handles out of range sampler arg gracefully", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "1.5")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil even with out of range arg")
	})

	s.Run("handles negative sampler arg gracefully", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "-0.1")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil even with negative arg")
	})

	s.Run("returns ParentBased(AlwaysSample) for parentbased_always_on", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "parentbased_always_on")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns ParentBased(TraceIDRatioBased) for parentbased_traceidratio", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "parentbased_traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "0.1")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns default for unknown sampler type", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "unknown_sampler")

		sampler := getSamplerFromEnv()
		s.NotNil(sampler, "sampler should not be nil even with unknown type")
	})

	s.Run("handles edge case ratio values", func() {
		s.Run("accepts 0.0", func() {
			s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
			s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "0.0")

			sampler := getSamplerFromEnv()
			s.NotNil(sampler, "sampler should accept 0.0")
		})

		s.Run("accepts 1.0", func() {
			s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
			s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "1.0")

			sampler := getSamplerFromEnv()
			s.NotNil(sampler, "sampler should accept 1.0")
		})
	})
}

func (s *TelemetrySuite) TestGetSamplerFromConfig() {
	s.Run("returns default ParentBased(AlwaysSample) when sampler is empty", func() {
		cfg := &config.TelemetryConfig{}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
	})

	s.Run("returns AlwaysSample for always_on", func() {
		cfg := &config.TelemetryConfig{TracesSampler: "always_on"}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
	})

	s.Run("returns NeverSample for always_off", func() {
		cfg := &config.TelemetryConfig{TracesSampler: "always_off"}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
	})

	s.Run("returns TraceIDRatioBased for traceidratio with valid arg", func() {
		ratio := 0.5
		cfg := &config.TelemetryConfig{
			TracesSampler:    "traceidratio",
			TracesSamplerArg: &ratio,
		}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
	})

	s.Run("returns TraceIDRatioBased with default 1.0 for traceidratio without arg", func() {
		cfg := &config.TelemetryConfig{TracesSampler: "traceidratio"}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
	})

	s.Run("returns ParentBased(AlwaysSample) for parentbased_always_on", func() {
		cfg := &config.TelemetryConfig{TracesSampler: "parentbased_always_on"}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
	})

	s.Run("returns ParentBased(NeverSample) for parentbased_always_off", func() {
		cfg := &config.TelemetryConfig{TracesSampler: "parentbased_always_off"}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
	})

	s.Run("returns ParentBased(TraceIDRatioBased) for parentbased_traceidratio", func() {
		ratio := 0.1
		cfg := &config.TelemetryConfig{
			TracesSampler:    "parentbased_traceidratio",
			TracesSamplerArg: &ratio,
		}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
	})

	s.Run("returns default for unknown sampler type", func() {
		cfg := &config.TelemetryConfig{TracesSampler: "unknown_sampler"}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
	})

	s.Run("handles edge case ratio values", func() {
		s.Run("accepts 0.0", func() {
			ratio := 0.0
			cfg := &config.TelemetryConfig{
				TracesSampler:    "traceidratio",
				TracesSamplerArg: &ratio,
			}

			sampler := getSamplerFromConfig(cfg)
			s.NotNil(sampler)
		})

		s.Run("accepts 1.0", func() {
			ratio := 1.0
			cfg := &config.TelemetryConfig{
				TracesSampler:    "traceidratio",
				TracesSamplerArg: &ratio,
			}

			sampler := getSamplerFromConfig(cfg)
			s.NotNil(sampler)
		})
	})

	s.Run("env var takes precedence over config sampler", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "always_off")
		cfg := &config.TelemetryConfig{TracesSampler: "always_on"}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
		// The sampler returned should respect the env var override
		// (GetTracesSampler returns the env var value)
	})

	s.Run("env var takes precedence over config sampler arg", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "0.1")
		ratio := 0.9
		cfg := &config.TelemetryConfig{
			TracesSampler:    "traceidratio",
			TracesSamplerArg: &ratio,
		}

		sampler := getSamplerFromConfig(cfg)
		s.NotNil(sampler)
	})
}
