package metrics

import (
	"context"
	"time"
)

// Collector defines the interface for collecting metrics from various sources.
// Implementations can export metrics to different backends (OTel, Prometheus, in-memory stats, etc.).
type Collector interface {
	// RecordToolCall records metrics for an MCP tool call execution.
	RecordToolCall(ctx context.Context, name string, duration time.Duration, err error)

	// RecordHTTPRequest records metrics for an HTTP request.
	RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration)
}
