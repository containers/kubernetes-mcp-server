package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type OtelStatsCollectorSuite struct {
	suite.Suite
	collector *OtelStatsCollector
}

func (s *OtelStatsCollectorSuite) SetupTest() {
	collector, err := NewOtelStatsCollector("test-meter")
	s.Require().NoError(err)
	s.collector = collector
}

func (s *OtelStatsCollectorSuite) TearDownTest() {
	if s.collector != nil {
		_ = s.collector.Shutdown(context.Background())
	}
}

func (s *OtelStatsCollectorSuite) TestRecordToolCall() {
	s.Run("records successful tool calls", func() {
		ctx := context.Background()
		s.collector.RecordToolCall(ctx, "test_tool", 100*time.Millisecond, nil)
		s.collector.RecordToolCall(ctx, "test_tool", 200*time.Millisecond, nil)
		s.collector.RecordToolCall(ctx, "another_tool", 50*time.Millisecond, nil)

		stats := s.collector.GetStats()
		s.Equal(int64(3), stats.TotalToolCalls, "Should have 3 total tool calls")
		s.Equal(int64(2), stats.ToolCallsByName["test_tool"], "Should have 2 calls for test_tool")
		s.Equal(int64(1), stats.ToolCallsByName["another_tool"], "Should have 1 call for another_tool")
		s.Equal(int64(0), stats.ToolCallErrors, "Should have no errors")
	})

	s.Run("records tool call errors", func() {
		ctx := context.Background()
		collector, err := NewOtelStatsCollector("test-meter-errors")
		s.Require().NoError(err)

		collector.RecordToolCall(ctx, "failing_tool", 100*time.Millisecond, nil)
		collector.RecordToolCall(ctx, "failing_tool", 200*time.Millisecond, &TestError{msg: "test error"})

		stats := collector.GetStats()
		s.Equal(int64(2), stats.TotalToolCalls, "Should have 2 total tool calls")
		s.Equal(int64(1), stats.ToolCallErrors, "Should have 1 error")
		s.Equal(int64(1), stats.ToolErrorsByName["failing_tool"], "Should have 1 error for failing_tool")
	})
}

func (s *OtelStatsCollectorSuite) TestRecordHTTPRequest() {
	s.Run("records HTTP requests by status class", func() {
		ctx := context.Background()
		s.collector.RecordHTTPRequest(ctx, "GET", "/api/v1", 200, 50*time.Millisecond)
		s.collector.RecordHTTPRequest(ctx, "POST", "/api/v1", 201, 100*time.Millisecond)
		s.collector.RecordHTTPRequest(ctx, "GET", "/api/v2", 404, 30*time.Millisecond)
		s.collector.RecordHTTPRequest(ctx, "POST", "/api/v1", 500, 200*time.Millisecond)

		stats := s.collector.GetStats()
		s.Equal(int64(4), stats.TotalHTTPRequests, "Should have 4 total HTTP requests")
		s.Equal(int64(2), stats.HTTPRequestsByStatus["2xx"], "Should have 2 successful requests")
		s.Equal(int64(1), stats.HTTPRequestsByStatus["4xx"], "Should have 1 client error")
		s.Equal(int64(1), stats.HTTPRequestsByStatus["5xx"], "Should have 1 server error")
	})

	s.Run("records HTTP requests by method", func() {
		ctx := context.Background()
		collector, err := NewOtelStatsCollector("test-meter-http")
		s.Require().NoError(err)

		collector.RecordHTTPRequest(ctx, "GET", "/api/v1", 200, 50*time.Millisecond)
		collector.RecordHTTPRequest(ctx, "GET", "/api/v2", 200, 60*time.Millisecond)
		collector.RecordHTTPRequest(ctx, "POST", "/api/v1", 201, 100*time.Millisecond)

		stats := collector.GetStats()
		s.Equal(int64(2), stats.HTTPRequestsByMethod["GET"], "Should have 2 GET requests")
		s.Equal(int64(1), stats.HTTPRequestsByMethod["POST"], "Should have 1 POST request")
	})

	s.Run("records HTTP requests by path", func() {
		ctx := context.Background()
		collector, err := NewOtelStatsCollector("test-meter-http-path")
		s.Require().NoError(err)

		collector.RecordHTTPRequest(ctx, "GET", "/api/v1", 200, 50*time.Millisecond)
		collector.RecordHTTPRequest(ctx, "GET", "/api/v1", 200, 60*time.Millisecond)
		collector.RecordHTTPRequest(ctx, "POST", "/api/v2", 201, 100*time.Millisecond)

		stats := collector.GetStats()
		s.Equal(int64(2), stats.HTTPRequestsByPath["/api/v1"], "Should have 2 requests to /api/v1")
		s.Equal(int64(1), stats.HTTPRequestsByPath["/api/v2"], "Should have 1 request to /api/v2")
	})
}

func (s *OtelStatsCollectorSuite) TestGetStats() {
	s.Run("returns uptime and start time", func() {
		stats := s.collector.GetStats()
		s.NotNil(stats, "Stats should not be nil")
		s.True(stats.UptimeSeconds >= 0, "Uptime should be non-negative")
		s.True(stats.StartTime > 0, "Start time should be set")
	})

	s.Run("initializes all maps", func() {
		stats := s.collector.GetStats()
		s.NotNil(stats.ToolCallsByName, "ToolCallsByName should be initialized")
		s.NotNil(stats.ToolErrorsByName, "ToolErrorsByName should be initialized")
		s.NotNil(stats.HTTPRequestsByPath, "HTTPRequestsByPath should be initialized")
		s.NotNil(stats.HTTPRequestsByStatus, "HTTPRequestsByStatus should be initialized")
		s.NotNil(stats.HTTPRequestsByMethod, "HTTPRequestsByMethod should be initialized")
	})
}

func (s *OtelStatsCollectorSuite) TestToolDurationHistogram() {
	s.Run("records tool call duration", func() {
		collector, err := NewOtelStatsCollectorWithConfig(CollectorConfig{
			MeterName:      "test-meter-duration",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
		})
		s.Require().NoError(err)

		ctx := context.Background()
		collector.RecordToolCall(ctx, "slow_tool", 500*time.Millisecond, nil)
		collector.RecordToolCall(ctx, "fast_tool", 10*time.Millisecond, nil)

		// Read metrics from the manual reader
		var rm metricdata.ResourceMetrics
		err = collector.reader.Collect(ctx, &rm)
		s.Require().NoError(err)

		// Find the duration histogram
		var foundHistogram bool
		for _, scopeMetrics := range rm.ScopeMetrics {
			for _, m := range scopeMetrics.Metrics {
				if m.Name == "k8s_mcp.tool.duration" {
					foundHistogram = true
					histogram, ok := m.Data.(metricdata.Histogram[float64])
					s.True(ok, "k8s_mcp.tool.duration should be a float64 histogram")
					s.Len(histogram.DataPoints, 2, "Should have 2 data points (one per tool)")

					// Verify data points have recorded values
					for _, dp := range histogram.DataPoints {
						s.Greater(dp.Count, uint64(0), "Histogram should have recorded at least one value")
						s.Greater(dp.Sum, float64(0), "Histogram sum should be greater than 0")
					}
				}
			}
		}
		s.True(foundHistogram, "k8s_mcp.tool.duration histogram should exist")
	})
}

func (s *OtelStatsCollectorSuite) TestServerInfoGauge() {
	s.Run("records server info with version labels", func() {
		collector, err := NewOtelStatsCollectorWithConfig(CollectorConfig{
			MeterName:      "test-meter-info",
			ServiceName:    "test-service",
			ServiceVersion: "1.2.3",
		})
		s.Require().NoError(err)

		ctx := context.Background()

		// Read metrics from the manual reader
		var rm metricdata.ResourceMetrics
		err = collector.reader.Collect(ctx, &rm)
		s.Require().NoError(err)

		// Find the server info gauge
		var foundGauge bool
		for _, scopeMetrics := range rm.ScopeMetrics {
			for _, m := range scopeMetrics.Metrics {
				if m.Name == "k8s_mcp.server.info" {
					foundGauge = true
					gauge, ok := m.Data.(metricdata.Gauge[int64])
					s.True(ok, "k8s_mcp.server.info should be an int64 gauge")
					s.Len(gauge.DataPoints, 1, "Should have 1 data point")

					if len(gauge.DataPoints) > 0 {
						dp := gauge.DataPoints[0]
						s.Equal(int64(1), dp.Value, "Gauge value should be 1")

						// Verify version attribute
						version, ok := dp.Attributes.Value("version")
						s.True(ok, "version attribute should exist")
						s.Equal("1.2.3", version.AsString(), "version should match config")

						// Verify go_version attribute
						goVersion, ok := dp.Attributes.Value("go_version")
						s.True(ok, "go_version attribute should exist")
						s.Equal(runtime.Version(), goVersion.AsString(), "go_version should match runtime")
					}
				}
			}
		}
		s.True(foundGauge, "k8s_mcp.server.info gauge should exist")
	})
}

func (s *OtelStatsCollectorSuite) TestPrometheusHandler() {
	s.Run("returns valid Prometheus handler", func() {
		collector, err := NewOtelStatsCollectorWithConfig(CollectorConfig{
			MeterName:      "test-meter-prom",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
		})
		s.Require().NoError(err)

		handler := collector.PrometheusHandler()
		s.NotNil(handler, "PrometheusHandler should not be nil")
	})

	s.Run("serves metrics in Prometheus format", func() {
		collector, err := NewOtelStatsCollectorWithConfig(CollectorConfig{
			MeterName:      "test-meter-prom-serve",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
		})
		s.Require().NoError(err)

		// Record some metrics
		ctx := context.Background()
		collector.RecordToolCall(ctx, "test_tool", 100*time.Millisecond, nil)
		collector.RecordHTTPRequest(ctx, "GET", "/api/v1", 200, 50*time.Millisecond)

		// Create a test request
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()

		// Serve the request
		handler := collector.PrometheusHandler()
		handler.ServeHTTP(rec, req)

		// Verify response
		s.Equal(http.StatusOK, rec.Code, "Should return 200 OK")

		body := rec.Body.String()
		s.Contains(body, "k8s_mcp_tool_calls", "Should contain k8s_mcp_tool_calls metric")
		s.Contains(body, "k8s_mcp_tool_duration", "Should contain k8s_mcp_tool_duration metric")
		s.Contains(body, "k8s_mcp_http_requests", "Should contain k8s_mcp_http_requests metric")
		s.Contains(body, "k8s_mcp_server_info", "Should contain k8s_mcp_server_info metric")
	})
}

// TestError is a simple error type for testing
type TestError struct {
	msg string
}

func (e *TestError) Error() string {
	return e.msg
}

func TestOtelStatsCollector(t *testing.T) {
	suite.Run(t, new(OtelStatsCollectorSuite))
}
