package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type MetricsSuite struct {
	suite.Suite
}

func (s *MetricsSuite) TestNew() {
	s.Run("creates metrics with stats collector enabled", func() {
		m, err := New(Config{
			TracerName:     "test",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
		})

		s.NoError(err)
		s.NotNil(m)
		s.NotNil(m.stats)
		s.Len(m.collectors, 1) // Stats collector
	})

	s.Run("creates metrics with telemetry config", func() {
		s.T().Setenv("OTEL_METRICS_EXPORTER", "")
		s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

		cfg := &config.TelemetryConfig{
			Endpoint: "http://localhost:4317",
			Protocol: "grpc",
		}

		m, err := New(Config{
			TracerName:     "test",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
			Telemetry:      cfg,
		})

		s.NoError(err)
		s.NotNil(m)
		s.NotNil(m.stats)
	})
}

func (s *MetricsSuite) TestRecordHTTPRequest() {
	s.Run("fans out to all collectors", func() {
		m, err := New(Config{
			TracerName:     "test",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
		})
		s.Require().NoError(err)
		ctx := context.Background()

		// Record an HTTP request
		m.RecordHTTPRequest(ctx, "GET", "/test", 200, 50*time.Millisecond)

		// Verify it was recorded in stats collector
		stats := m.GetStats()
		s.Equal(int64(1), stats.TotalHTTPRequests)
		s.Equal(int64(1), stats.HTTPRequestsByPath["/test"])
		s.Equal(int64(1), stats.HTTPRequestsByMethod["GET"])
		s.Equal(int64(1), stats.HTTPRequestsByStatus["2xx"])
	})
}

func (s *MetricsSuite) TestGetStats() {
	s.Run("returns stats from stats collector", func() {
		m, err := New(Config{
			TracerName:     "test",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
		})
		s.Require().NoError(err)
		ctx := context.Background()

		m.RecordToolCall(ctx, "tool_a", 100*time.Millisecond, nil)
		m.RecordHTTPRequest(ctx, "GET", "/test", 200, 50*time.Millisecond)

		stats := m.GetStats()
		s.Equal(int64(1), stats.TotalToolCalls)
		s.Equal(int64(1), stats.TotalHTTPRequests)
	})
}

func TestMetrics(t *testing.T) {
	suite.Run(t, new(MetricsSuite))
}
