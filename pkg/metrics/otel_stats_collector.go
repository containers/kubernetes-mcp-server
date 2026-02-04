package metrics

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"k8s.io/klog/v2"
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
// It provides a simple in-memory stats collector for the /stats endpoint
// and a Prometheus exporter for the /metrics endpoint.
type OtelStatsCollector struct {
	// OTel metric instruments
	toolCallCounter       metric.Int64Counter
	toolCallErrorCounter  metric.Int64Counter
	toolDurationHistogram metric.Float64Histogram
	httpRequestCounter    metric.Int64Counter
	serverInfoGauge       metric.Int64Gauge

	// Meter provider for shutdown
	provider *sdkmetric.MeterProvider

	// In-memory reader for querying metrics on-demand
	reader *sdkmetric.ManualReader

	// Prometheus HTTP handler for /metrics endpoint
	prometheusHandler http.Handler

	// Server start time for uptime calculation
	startTime time.Time
}

// CollectorConfig contains configuration for the OtelStatsCollector.
type CollectorConfig struct {
	MeterName      string
	ServiceName    string
	ServiceVersion string
	// Telemetry is the optional telemetry configuration.
	// If nil, env vars will be used for OTLP configuration.
	Telemetry *config.TelemetryConfig
}

// createMetricsExporter creates an OTLP metrics exporter.
// If cfg is provided and enabled, uses config values; otherwise falls back to env vars.
// Returns nil if:
//   - OTEL_METRICS_EXPORTER is set to "none" (env var always takes precedence)
//   - No endpoint is configured (neither in config nor env vars)
//
// When nil is returned, metrics will only be collected in-memory for the /stats endpoint.
func createMetricsExporter(ctx context.Context, cfg *config.TelemetryConfig) (sdkmetric.Exporter, error) {
	if strings.ToLower(os.Getenv("OTEL_METRICS_EXPORTER")) == "none" {
		klog.V(2).Info("OTLP metrics export disabled via OTEL_METRICS_EXPORTER=none")
		return nil, nil
	}

	// use config if provided and enabled, otherwise env vars
	var protocol string
	if cfg != nil && cfg.IsEnabled() {
		protocol = strings.ToLower(cfg.GetProtocol())
	} else {
		endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		if endpoint == "" {
			return nil, nil // No export configured
		}
		protocol = strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"))
	}

	switch protocol {
	case "http/protobuf", "http":
		klog.V(2).Infof("Using HTTP/protobuf OTLP metrics exporter (protocol=%s)", protocol)
		return otlpmetrichttp.New(ctx)

	case "grpc", "":
		if protocol == "" {
			klog.V(2).Info("Using gRPC OTLP metrics exporter (default)")
		} else {
			klog.V(2).Info("Using gRPC OTLP metrics exporter")
		}
		return otlpmetricgrpc.New(ctx)

	default:
		klog.V(1).Infof("Unknown OTEL_EXPORTER_OTLP_PROTOCOL '%s' for metrics, defaulting to gRPC", protocol)
		return otlpmetricgrpc.New(ctx)
	}
}

// NewOtelStatsCollector creates a new OtelStatsCollector with ManualReader.
// If OTEL_EXPORTER_OTLP_ENDPOINT is set, metrics will also be exported to OTLP.
func NewOtelStatsCollector(meterName string) (*OtelStatsCollector, error) {
	return NewOtelStatsCollectorWithConfig(CollectorConfig{
		MeterName:      meterName,
		ServiceName:    "kubernetes-mcp-server",
		ServiceVersion: "unknown",
	})
}

// NewOtelStatsCollectorWithConfig creates a new OtelStatsCollector with full configuration.
func NewOtelStatsCollectorWithConfig(cfg CollectorConfig) (*OtelStatsCollector, error) {
	ctx := context.Background()

	// Create an in-memory manual reader for stats collection (/stats endpoint)
	reader := sdkmetric.NewManualReader()

	// Create a custom Prometheus registry for the /metrics endpoint
	promRegistry := promclient.NewRegistry()

	// Create Prometheus exporter with custom registry
	prometheusExporter, err := prometheus.New(
		prometheus.WithRegisterer(promRegistry),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	// Create HTTP handler for Prometheus metrics
	prometheusHandler := promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})

	opts := []sdkmetric.Option{
		sdkmetric.WithReader(reader),
		sdkmetric.WithReader(prometheusExporter),
	}

	// Optionally add OTLP exporter if endpoint is configured
	exporter, err := createMetricsExporter(ctx, cfg.Telemetry)
	if err != nil {
		// Use Warning if telemetry was explicitly configured, V(1) otherwise
		if cfg.Telemetry != nil && cfg.Telemetry.IsEnabled() {
			klog.Warningf("Failed to create OTLP metrics exporter, OTLP export disabled: %v", err)
		} else {
			klog.V(1).Infof("Failed to create OTLP metrics exporter, OTLP export disabled: %v", err)
		}
	} else if exporter != nil {
		attrs := []attribute.KeyValue{
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		}
		if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
			attrs = append(attrs, semconv.K8SNamespaceName(ns))
		}
		res, err := resource.New(ctx,
			resource.WithAttributes(attrs...),
		)
		if err != nil {
			klog.V(1).Infof("Failed to create resource for metrics, using default: %v", err)
		} else {
			opts = append(opts, sdkmetric.WithResource(res))
		}

		periodicReader := sdkmetric.NewPeriodicReader(
			exporter,
			sdkmetric.WithInterval(30*time.Second),
		)
		opts = append(opts, sdkmetric.WithReader(periodicReader))
		klog.V(1).Info("OTLP metrics export enabled")
	}

	provider := sdkmetric.NewMeterProvider(opts...)

	meter := provider.Meter(cfg.MeterName)

	// Create metric instruments with k8s_mcp prefix for clear identification
	// in multi-MCP-server environments.
	toolCallCounter, err := meter.Int64Counter(
		"k8s_mcp.tool.calls",
		metric.WithDescription("Total number of MCP tool calls"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tool call counter: %w", err)
	}

	toolCallErrorCounter, err := meter.Int64Counter(
		"k8s_mcp.tool.errors",
		metric.WithDescription("Total number of MCP tool call errors"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tool error counter: %w", err)
	}

	toolDurationHistogram, err := meter.Float64Histogram(
		"k8s_mcp.tool.duration",
		metric.WithDescription("Duration of MCP tool calls in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tool duration histogram: %w", err)
	}

	httpRequestCounter, err := meter.Int64Counter(
		"k8s_mcp.http.requests",
		metric.WithDescription("Total number of HTTP requests to the MCP server"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request counter: %w", err)
	}

	serverInfoGauge, err := meter.Int64Gauge(
		"k8s_mcp.server.info",
		metric.WithDescription("Kubernetes MCP server version information"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create server info gauge: %w", err)
	}

	collector := &OtelStatsCollector{
		toolCallCounter:       toolCallCounter,
		toolCallErrorCounter:  toolCallErrorCounter,
		toolDurationHistogram: toolDurationHistogram,
		httpRequestCounter:    httpRequestCounter,
		serverInfoGauge:       serverInfoGauge,
		provider:              provider,
		reader:                reader,
		prometheusHandler:     prometheusHandler,
		startTime:             time.Now(),
	}

	// Record server info gauge with version attributes
	collector.serverInfoGauge.Record(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("version", cfg.ServiceVersion),
			attribute.String("go_version", runtime.Version()),
		),
	)

	return collector, nil
}

// Shutdown gracefully shuts down the meter provider, flushing any pending metrics.
func (c *OtelStatsCollector) Shutdown(ctx context.Context) error {
	return c.provider.Shutdown(ctx)
}

// PrometheusHandler returns the HTTP handler for the /metrics endpoint.
// This handler serves metrics in Prometheus text format.
func (c *OtelStatsCollector) PrometheusHandler() http.Handler {
	return c.prometheusHandler
}

// RecordToolCall implements the Collector interface.
func (c *OtelStatsCollector) RecordToolCall(ctx context.Context, name string, duration time.Duration, err error) {
	toolNameAttr := metric.WithAttributes(attribute.String("tool.name", name))

	// Record tool call with tool name as attribute
	c.toolCallCounter.Add(ctx, 1, toolNameAttr)

	// Record duration in seconds
	c.toolDurationHistogram.Record(ctx, duration.Seconds(), toolNameAttr)

	// Record errors
	if err != nil {
		c.toolCallErrorCounter.Add(ctx, 1, toolNameAttr)
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
// Thread-safety is handled by the OTel SDK's ManualReader.
func (c *OtelStatsCollector) GetStats() *Statistics {
	// Collect current metrics from the manual reader
	var rm metricdata.ResourceMetrics
	if err := c.reader.Collect(context.Background(), &rm); err != nil {
		klog.V(1).Infof("Failed to collect metrics for stats endpoint: %v", err)
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
	case "k8s_mcp.tool.calls":
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

	case "k8s_mcp.tool.errors":
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

	case "k8s_mcp.http.requests":
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
