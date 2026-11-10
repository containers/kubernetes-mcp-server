package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/config/configtest"
	"github.com/stretchr/testify/suite"
)

type MetricsSuite struct {
	suite.Suite
}

func (s *MetricsSuite) TestNew() {
	s.Run("creates metrics with stats collector enabled", func() {
		m, err := New(s.T().Context(), Config{
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

		cfg := func() *config.TelemetryConfig {
			c := configtest.NewTelemetry()
			c.Endpoint.SetForTest("http://localhost:4317")
			c.Protocol.SetForTest("grpc")
			return c
		}()

		m, err := New(s.T().Context(), Config{
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
		m, err := New(s.T().Context(), Config{
			TracerName:     "test",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
		})
		s.Require().NoError(err)
		ctx := context.Background()

		// Record an HTTP request
		m.RecordHTTPRequest(ctx, "GET", "/test", 200, 50*time.Millisecond)

		// Verify it was recorded in stats collector
		stats := m.GetStats(s.T().Context())
		s.Equal(int64(1), stats.TotalHTTPRequests)
		s.Equal(int64(1), stats.HTTPRequestsByPath["/test"])
		s.Equal(int64(1), stats.HTTPRequestsByMethod["GET"])
		s.Equal(int64(1), stats.HTTPRequestsByStatus["2xx"])
	})
}

func (s *MetricsSuite) TestRecordInFlightRequest() {
	s.Run("fans out to all collectors", func() {
		m, err := New(s.T().Context(), Config{
			TracerName:     "test",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
		})
		s.Require().NoError(err)
		ctx := context.Background()

		m.RecordInFlightRequest(ctx, "tools/call", 1)
		m.RecordInFlightRequest(ctx, "tools/call", 1)
		m.RecordInFlightRequest(ctx, "tools/call", -1)

		s.Equal(int64(1), m.GetStats(s.T().Context()).RequestsInFlight["tools/call"])

		rec := httptest.NewRecorder()
		m.PrometheusHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		s.Regexp(regexp.MustCompile(`(?m)^k8s_mcp_requests_in_flight\{[^}]*mcp_method_name="tools/call"[^}]*\} 1$`), rec.Body.String())
	})
}

func (s *MetricsSuite) TestGetStats() {
	s.Run("returns stats from stats collector", func() {
		m, err := New(s.T().Context(), Config{
			TracerName:     "test",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
		})
		s.Require().NoError(err)
		ctx := context.Background()

		m.RecordToolCall(ctx, "tool_a", 100*time.Millisecond, nil)
		m.RecordHTTPRequest(ctx, "GET", "/test", 200, 50*time.Millisecond)

		stats := m.GetStats(s.T().Context())
		s.Equal(int64(1), stats.TotalToolCalls)
		s.Equal(int64(1), stats.TotalHTTPRequests)
	})
}

func TestMetrics(t *testing.T) {
	suite.Run(t, new(MetricsSuite))
}
