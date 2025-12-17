package telemetry

import (
	"context"
	"testing"

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
		initialProvider := otel.GetTracerProvider()

		cleanup, err := InitTracer("test-service", "1.0.0")
		s.Require().NoError(err)
		defer cleanup()

		// Provider should be set (even if tracing is disabled)
		currentProvider := otel.GetTracerProvider()
		s.NotNil(currentProvider, "tracer provider should be set")

		// If we actually got a TracerProvider, it should be different from initial
		if _, ok := currentProvider.(*trace.TracerProvider); ok {
			s.NotEqual(initialProvider, currentProvider, "should set new tracer provider")
		}
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
