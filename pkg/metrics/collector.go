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

	// RecordInFlightRequest adds delta (+1 when an MCP request starts, -1 when it returns)
	// to the number of MCP requests currently being handled for the given method.
	RecordInFlightRequest(ctx context.Context, method string, delta int64)
}
