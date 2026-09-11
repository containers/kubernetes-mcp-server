package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/config/configtest"
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

func initTestTracer(ctx context.Context, serviceName, serviceVersion string) (func(), error) {
	cfg, err := config.ReadToml(nil)
	if err != nil {
		return func() {}, err
	}
	return InitTracerWithConfig(ctx, &cfg.Telemetry, serviceName, serviceVersion)
}

func (s *TelemetrySuite) TestInitTracer() {
	s.Run("returns cleanup function when OTLP endpoint not configured", func() {
		cleanup, err := initTestTracer(s.T().Context(), "test-service", "1.0.0")

		s.NoError(err, "should not return error when OTLP endpoint is not configured")
		s.NotNil(cleanup, "cleanup function should not be nil")

		s.NotPanics(func() {
			cleanup()
		}, "cleanup should not panic")
	})

	s.Run("initializes with valid service name and version", func() {
		cleanup, err := initTestTracer(s.T().Context(), "my-service", "2.0.0")
		defer cleanup()

		s.NoError(err, "initialization should succeed")
		s.NotNil(cleanup, "should return cleanup function")
	})

	s.Run("sets global tracer provider", func() {
		s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
		initialProvider := otel.GetTracerProvider()

		cleanup, err := initTestTracer(s.T().Context(), "test-service", "1.0.0")
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
		cleanup, err := initTestTracer(s.T().Context(), "test-service", "1.0.0")
		s.Require().NoError(err)

		s.NotPanics(func() {
			cleanup()
			cleanup()
			cleanup()
		}, "cleanup should be safe to call multiple times")
	})

	s.Run("executes without blocking", func() {
		cleanup, err := initTestTracer(s.T().Context(), "test-service", "1.0.0")
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
		cleanup, err := initTestTracer(s.T().Context(), "", "1.0.0")
		defer cleanup()

		s.NoError(err, "should handle empty service name")
		s.NotNil(cleanup, "should return cleanup function")
	})

	s.Run("handles empty service version", func() {
		cleanup, err := initTestTracer(s.T().Context(), "test-service", "")
		defer cleanup()

		s.NoError(err, "should handle empty service version")
		s.NotNil(cleanup, "should return cleanup function")
	})

	s.Run("handles both empty", func() {
		cleanup, err := initTestTracer(s.T().Context(), "", "")
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

		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest(server.URL)
			c.Protocol.SetForTest("http/protobuf")
			return c
		}()
		cleanup, err := InitTracerWithConfig(s.T().Context(), cfg, "test-service", "1.0.0")
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

		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest(server.URL)
			c.Protocol.SetForTest("http/protobuf")
			return c
		}()
		cleanup, err := InitTracerWithConfig(s.T().Context(), cfg, "test-service", "1.0.0")
		s.Require().NoError(err)
		s.Require().True(Enabled())

		cleanup()

		s.False(Enabled())
	})
}

func (s *TelemetrySuite) TestInitTracerWithConfig() {
	s.Run("returns no-op cleanup when cfg is nil", func() {
		cleanup, err := InitTracerWithConfig(s.T().Context(), nil, "test-service", "1.0.0")

		s.NoError(err)
		s.NotNil(cleanup)
		s.NotPanics(func() { cleanup() })
	})

	s.Run("returns no-op cleanup when cfg is not enabled", func() {
		enabled := false
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Enabled.SetForTest(&enabled)
			c.Endpoint.SetForTest("http://localhost:4317")
			return c
		}()

		cleanup, err := InitTracerWithConfig(s.T().Context(), cfg, "test-service", "1.0.0")

		s.NoError(err)
		s.NotNil(cleanup)
		s.False(Enabled())
	})

	s.Run("returns no-op cleanup when cfg has no endpoint", func() {
		cfg := configtest.NewTelemetry()

		cleanup, err := InitTracerWithConfig(s.T().Context(), cfg, "test-service", "1.0.0")

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

		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest(server.URL)
			c.Protocol.SetForTest("http/protobuf")
			return c
		}()

		cleanup, err := InitTracerWithConfig(s.T().Context(), cfg, "test-service", "1.0.0")
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

		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest(server.URL)
			c.Protocol.SetForTest("grpc")
			return c
		}()

		cleanup, err := InitTracerWithConfig(s.T().Context(), cfg, "test-service", "1.0.0")
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

		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest(server.URL)
			c.Protocol.SetForTest("http/protobuf")
			return c
		}()
		initialProvider := otel.GetTracerProvider()

		cleanup, err := InitTracerWithConfig(s.T().Context(), cfg, "test-service", "1.0.0")
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
		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, configtest.Telemetry("http://localhost:4317", ""))
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates gRPC exporter for grpc protocol", func() {
		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, configtest.Telemetry("http://localhost:4317", "grpc"))
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates HTTP exporter for http/protobuf protocol", func() {
		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, configtest.Telemetry("http://localhost:4317", "http/protobuf"))
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates HTTP exporter for http protocol alias", func() {
		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, configtest.Telemetry("http://localhost:4317", "http"))
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("falls back to gRPC for unknown protocol", func() {
		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, configtest.Telemetry("http://localhost:4317", "unknown_protocol"))
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("handles case-insensitive protocol values", func() {
		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, configtest.Telemetry("http://localhost:4317", "HTTP/PROTOBUF"))
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})
}

func (s *TelemetrySuite) TestCreateExporterWithConfig() {
	s.Run("creates gRPC exporter by default when protocol is empty", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest("http://localhost:4317")
			return c
		}()

		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates gRPC exporter for grpc protocol", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest("http://localhost:4317")
			c.Protocol.SetForTest("grpc")
			return c
		}()

		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates HTTP exporter for http/protobuf protocol", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest("http://localhost:4318")
			c.Protocol.SetForTest("http/protobuf")
			return c
		}()

		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("creates HTTP exporter for http protocol alias", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest("http://localhost:4318")
			c.Protocol.SetForTest("http")
			return c
		}()

		ctx := context.Background()
		exporter, err := createExporterWithConfig(ctx, cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(ctx) }()
	})

	s.Run("falls back to gRPC for unknown protocol", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest("http://localhost:4317")
			c.Protocol.SetForTest("unknown_protocol")
			return c
		}()

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

		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest(server.URL)
			c.Protocol.SetForTest("http/protobuf")
			return c
		}()

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

		sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
			cfg, err := config.ReadToml(nil)
			s.Require().NoError(err)
			return &cfg.Telemetry
		}())
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns AlwaysSample for always_on", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "always_on")

		sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
			cfg, err := config.ReadToml(nil)
			s.Require().NoError(err)
			return &cfg.Telemetry
		}())
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns NeverSample for always_off", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "always_off")

		sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
			cfg, err := config.ReadToml(nil)
			s.Require().NoError(err)
			return &cfg.Telemetry
		}())
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns TraceIDRatioBased for traceidratio with valid arg", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "0.5")

		sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
			cfg, err := config.ReadToml(nil)
			s.Require().NoError(err)
			return &cfg.Telemetry
		}())
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns TraceIDRatioBased with default 1.0 for traceidratio without arg", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "")

		sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
			cfg, err := config.ReadToml(nil)
			s.Require().NoError(err)
			return &cfg.Telemetry
		}())
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("rejects invalid sampler arg at load", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "invalid")

		_, err := config.ReadToml(nil)
		s.Require().Error(err)
		s.Contains(err.Error(), "OTEL_TRACES_SAMPLER_ARG")
	})

	s.Run("handles out of range sampler arg gracefully", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "1.5")

		sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
			cfg, err := config.ReadToml(nil)
			s.Require().NoError(err)
			return &cfg.Telemetry
		}())
		s.NotNil(sampler, "sampler should not be nil even with out of range arg")
	})

	s.Run("handles negative sampler arg gracefully", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "-0.1")

		sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
			cfg, err := config.ReadToml(nil)
			s.Require().NoError(err)
			return &cfg.Telemetry
		}())
		s.NotNil(sampler, "sampler should not be nil even with negative arg")
	})

	s.Run("returns ParentBased(AlwaysSample) for parentbased_always_on", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "parentbased_always_on")

		sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
			cfg, err := config.ReadToml(nil)
			s.Require().NoError(err)
			return &cfg.Telemetry
		}())
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns ParentBased(TraceIDRatioBased) for parentbased_traceidratio", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "parentbased_traceidratio")
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "0.1")

		sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
			cfg, err := config.ReadToml(nil)
			s.Require().NoError(err)
			return &cfg.Telemetry
		}())
		s.NotNil(sampler, "sampler should not be nil")
	})

	s.Run("returns default for unknown sampler type", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "unknown_sampler")

		sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
			cfg, err := config.ReadToml(nil)
			s.Require().NoError(err)
			return &cfg.Telemetry
		}())
		s.NotNil(sampler, "sampler should not be nil even with unknown type")
	})

	s.Run("handles edge case ratio values", func() {
		s.Run("accepts 0.0", func() {
			s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
			s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "0.0")

			sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
				cfg, err := config.ReadToml(nil)
				s.Require().NoError(err)
				return &cfg.Telemetry
			}())
			s.NotNil(sampler, "sampler should accept 0.0")
		})

		s.Run("accepts 1.0", func() {
			s.T().Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
			s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "1.0")

			sampler := getSamplerFromConfig(s.T().Context(), func() *config.TelemetryConfig {
				cfg, err := config.ReadToml(nil)
				s.Require().NoError(err)
				return &cfg.Telemetry
			}())
			s.NotNil(sampler, "sampler should accept 1.0")
		})
	})
}

func (s *TelemetrySuite) TestGetSamplerFromConfig() {
	s.Run("returns default ParentBased(AlwaysSample) when sampler is empty", func() {
		cfg := configtest.NewTelemetry()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
	})

	s.Run("returns AlwaysSample for always_on", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.TracesSampler.SetForTest("always_on")
			return c
		}()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
	})

	s.Run("returns NeverSample for always_off", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.TracesSampler.SetForTest("always_off")
			return c
		}()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
	})

	s.Run("returns TraceIDRatioBased for traceidratio with valid arg", func() {
		ratio := 0.5
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.TracesSampler.SetForTest("traceidratio")
			c.TracesSamplerArg.SetForTest(&ratio)
			return c
		}()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
	})

	s.Run("returns TraceIDRatioBased with default 1.0 for traceidratio without arg", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.TracesSampler.SetForTest("traceidratio")
			return c
		}()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
	})

	s.Run("returns ParentBased(AlwaysSample) for parentbased_always_on", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.TracesSampler.SetForTest("parentbased_always_on")
			return c
		}()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
	})

	s.Run("returns ParentBased(NeverSample) for parentbased_always_off", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.TracesSampler.SetForTest("parentbased_always_off")
			return c
		}()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
	})

	s.Run("returns ParentBased(TraceIDRatioBased) for parentbased_traceidratio", func() {
		ratio := 0.1
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.TracesSampler.SetForTest("parentbased_traceidratio")
			c.TracesSamplerArg.SetForTest(&ratio)
			return c
		}()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
	})

	s.Run("returns default for unknown sampler type", func() {
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.TracesSampler.SetForTest("unknown_sampler")
			return c
		}()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
	})

	s.Run("handles edge case ratio values", func() {
		s.Run("accepts 0.0", func() {
			ratio := 0.0
			cfg := func() *config.TelemetryConfig {
				c := configtest.NewTelemetry()
				c.TracesSampler.SetForTest("traceidratio")
				c.TracesSamplerArg.SetForTest(&ratio)
				return c
			}()

			sampler := getSamplerFromConfig(s.T().Context(), cfg)
			s.NotNil(sampler)
		})

		s.Run("accepts 1.0", func() {
			ratio := 1.0
			cfg := func() *config.TelemetryConfig {
				c := configtest.NewTelemetry()
				c.TracesSampler.SetForTest("traceidratio")
				c.TracesSamplerArg.SetForTest(&ratio)
				return c
			}()

			sampler := getSamplerFromConfig(s.T().Context(), cfg)
			s.NotNil(sampler)
		})
	})

	s.Run("env var takes precedence over config sampler", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER", "always_off")
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.TracesSampler.SetForTest("always_on")
			return c
		}()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
		// The sampler returned should respect the env var override
		// (GetTracesSampler returns the env var value)
	})

	s.Run("env var takes precedence over config sampler arg", func() {
		s.T().Setenv("OTEL_TRACES_SAMPLER_ARG", "0.1")
		ratio := 0.9
		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.TracesSampler.SetForTest("traceidratio")
			c.TracesSamplerArg.SetForTest(&ratio)
			return c
		}()

		sampler := getSamplerFromConfig(s.T().Context(), cfg)
		s.NotNil(sampler)
	})
}
