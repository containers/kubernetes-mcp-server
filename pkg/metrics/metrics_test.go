package metrics

import (
	"context"
	"testing"
	"time"

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
}

func (s *MetricsSuite) TestRecordToolCall() {
	s.Run("fans out to all collectors", func() {
		m, err := New(Config{
			TracerName:     "test",
			ServiceName:    "test-service",
			ServiceVersion: "1.0.0",
		})
		s.Require().NoError(err)
		ctx := context.Background()

		// Record a tool call
		m.RecordToolCall(ctx, "test_tool", 100*time.Millisecond, nil)

		// Verify it was recorded in stats collector
		stats := m.GetStats()
		s.Equal(int64(1), stats.TotalToolCalls)
		s.Equal(int64(1), stats.ToolCallsByName["test_tool"])
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
