package metrics

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Statistics represents the aggregated metrics data exposed by the stats endpoint.
type Statistics struct {
	// Tool call metrics
	TotalToolCalls   int64            `json:"total_tool_calls"`
	ToolCallErrors   int64            `json:"tool_call_errors"`
	ToolCallsByName  map[string]int64 `json:"tool_calls_by_name"`
	ToolErrorsByName map[string]int64 `json:"tool_errors_by_name"`

	// HTTP request metrics
	TotalHTTPRequests    int64            `json:"total_http_requests"`
	HTTPRequestsByPath   map[string]int64 `json:"http_requests_by_path"`
	HTTPRequestsByStatus map[string]int64 `json:"http_requests_by_status"`
	HTTPRequestsByMethod map[string]int64 `json:"http_requests_by_method"`

	// Uptime
	UptimeSeconds int64 `json:"uptime_seconds"`
	StartTime     int64 `json:"start_time_unix"`
}

// OtelStatsCollector collects metrics using OpenTelemetry SDK with ManualReader.
// It provides a simple in-memory stats collector for the /stats endpoint.
type OtelStatsCollector struct {
	// OTel metric instruments
	toolCallCounter      metric.Int64Counter
	toolCallErrorCounter metric.Int64Counter
	httpRequestCounter   metric.Int64Counter

	// In-memory reader for querying metrics on-demand
	reader *sdkmetric.ManualReader

	// Server start time for uptime calculation
	startTime time.Time

	// Mutex for thread-safe access to reader
	mu sync.RWMutex
}

// NewOtelStatsCollector creates a new OtelStatsCollector with ManualReader.
func NewOtelStatsCollector(meterName string) *OtelStatsCollector {
	// Create an in-memory manual reader for stats collection
	reader := sdkmetric.NewManualReader()

	// Create a meter provider with the manual reader
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)

	meter := provider.Meter(meterName)

	// Create metric instruments following OTel semantic conventions
	toolCallCounter, _ := meter.Int64Counter(
		"mcp.tool.calls",
		metric.WithDescription("Total number of MCP tool calls"),
	)

	toolCallErrorCounter, _ := meter.Int64Counter(
		"mcp.tool.errors",
		metric.WithDescription("Total number of MCP tool call errors"),
	)

	httpRequestCounter, _ := meter.Int64Counter(
		"http.server.requests",
		metric.WithDescription("Total number of HTTP requests"),
	)

	return &OtelStatsCollector{
		toolCallCounter:      toolCallCounter,
		toolCallErrorCounter: toolCallErrorCounter,
		httpRequestCounter:   httpRequestCounter,
		reader:               reader,
		startTime:            time.Now(),
	}
}

// RecordToolCall implements the Collector interface.
func (c *OtelStatsCollector) RecordToolCall(ctx context.Context, name string, duration time.Duration, err error) {
	// Record tool call with tool name as attribute
	c.toolCallCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("tool.name", name),
	))

	// Record errors
	if err != nil {
		c.toolCallErrorCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("tool.name", name),
		))
	}
}

// RecordHTTPRequest implements the Collector interface.
func (c *OtelStatsCollector) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	// Determine status class (2xx, 3xx, 4xx, 5xx)
	statusClass := ""
	if statusCode >= 200 && statusCode < 300 {
		statusClass = "2xx"
	} else if statusCode >= 300 && statusCode < 400 {
		statusClass = "3xx"
	} else if statusCode >= 400 && statusCode < 500 {
		statusClass = "4xx"
	} else if statusCode >= 500 && statusCode < 600 {
		statusClass = "5xx"
	} else {
		statusClass = "other"
	}

	// Record HTTP request with attributes
	c.httpRequestCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("http.request.method", method),
		attribute.String("url.path", path),
		attribute.String("http.response.status_class", statusClass),
	))
}

// GetStats returns a snapshot of current statistics by reading from OTel metrics.
func (c *OtelStatsCollector) GetStats() *Statistics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Collect current metrics from the manual reader
	var rm metricdata.ResourceMetrics
	if err := c.reader.Collect(context.Background(), &rm); err != nil {
		// Return empty stats on error
		return &Statistics{
			ToolCallsByName:      make(map[string]int64),
			ToolErrorsByName:     make(map[string]int64),
			HTTPRequestsByPath:   make(map[string]int64),
			HTTPRequestsByStatus: make(map[string]int64),
			HTTPRequestsByMethod: make(map[string]int64),
			UptimeSeconds:        int64(time.Since(c.startTime).Seconds()),
			StartTime:            c.startTime.Unix(),
		}
	}

	stats := &Statistics{
		ToolCallsByName:      make(map[string]int64),
		ToolErrorsByName:     make(map[string]int64),
		HTTPRequestsByPath:   make(map[string]int64),
		HTTPRequestsByStatus: make(map[string]int64),
		HTTPRequestsByMethod: make(map[string]int64),
		UptimeSeconds:        int64(time.Since(c.startTime).Seconds()),
		StartTime:            c.startTime.Unix(),
	}

	// Process collected metrics
	for _, scopeMetrics := range rm.ScopeMetrics {
		for _, m := range scopeMetrics.Metrics {
			c.processMetric(m, stats)
		}
	}

	return stats
}

// processMetric extracts data from a single metric and updates the statistics.
func (c *OtelStatsCollector) processMetric(m metricdata.Metrics, stats *Statistics) {
	switch m.Name {
	case "mcp.tool.calls":
		if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
			for _, dp := range sum.DataPoints {
				value := dp.Value
				stats.TotalToolCalls += value

				// Extract tool name from attributes
				toolName := c.getAttributeValue(dp.Attributes, "tool.name")
				if toolName != "" {
					stats.ToolCallsByName[toolName] = value
				}
			}
		}

	case "mcp.tool.errors":
		if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
			for _, dp := range sum.DataPoints {
				value := dp.Value
				stats.ToolCallErrors += value

				// Extract tool name from attributes
				toolName := c.getAttributeValue(dp.Attributes, "tool.name")
				if toolName != "" {
					stats.ToolErrorsByName[toolName] = value
				}
			}
		}

	case "http.server.requests":
		if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
			for _, dp := range sum.DataPoints {
				value := dp.Value
				stats.TotalHTTPRequests += value

				// Extract attributes
				method := c.getAttributeValue(dp.Attributes, "http.request.method")
				path := c.getAttributeValue(dp.Attributes, "url.path")
				statusClass := c.getAttributeValue(dp.Attributes, "http.response.status_class")

				if method != "" {
					stats.HTTPRequestsByMethod[method] += value
				}
				if path != "" {
					stats.HTTPRequestsByPath[path] += value
				}
				if statusClass != "" {
					stats.HTTPRequestsByStatus[statusClass] += value
				}
			}
		}
	}
}

// getAttributeValue extracts a string value from attributes by key.
func (c *OtelStatsCollector) getAttributeValue(attrs attribute.Set, key string) string {
	val, ok := attrs.Value(attribute.Key(key))
	if !ok {
		return ""
	}
	return val.AsString()
}
