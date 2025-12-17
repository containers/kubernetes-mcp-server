package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
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
