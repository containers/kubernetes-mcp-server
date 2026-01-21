package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"k8s.io/klog/v2"
)

// Config contains configuration for the metrics system.
type Config struct {
	TracerName     string
	ServiceName    string
	ServiceVersion string
	// Telemetry is the optional telemetry configuration.
	// If nil, env vars will be used for OTLP configuration.
	Telemetry *config.TelemetryConfig
}

// Metrics coordinates multiple metric collectors.
// It implements the Collector interface and fans out calls to all registered collectors.
type Metrics struct {
	collectors []Collector
	stats      *OtelStatsCollector
}

// New creates a new Metrics instance with configured collectors.
// If OTEL_EXPORTER_OTLP_ENDPOINT is set, metrics will also be exported to OTLP.
func New(cfg Config) (*Metrics, error) {
	m := &Metrics{
		collectors: []Collector{},
	}

	// Stats collector - always enabled for /stats endpoint
	// Also exports to OTLP if OTEL_EXPORTER_OTLP_ENDPOINT is set or Telemetry config is provided
	stats, err := NewOtelStatsCollectorWithConfig(CollectorConfig{
		MeterName:      cfg.TracerName,
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Telemetry:      cfg.Telemetry,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create stats collector: %w", err)
	}
	m.stats = stats
	m.collectors = append(m.collectors, m.stats)
	klog.V(1).Info("Stats collector enabled")

	return m, nil
}

// RecordToolCall implements the Collector interface.
// It fans out the call to all registered collectors.
func (m *Metrics) RecordToolCall(ctx context.Context, name string, duration time.Duration, err error) {
	for _, c := range m.collectors {
		c.RecordToolCall(ctx, name, duration, err)
	}
}

// RecordHTTPRequest implements the Collector interface.
// It fans out the call to all registered collectors.
func (m *Metrics) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	for _, c := range m.collectors {
		c.RecordHTTPRequest(ctx, method, path, statusCode, duration)
	}
}

// GetStats returns the current statistics from the StatsCollector.
// This is used by the /stats HTTP endpoint.
func (m *Metrics) GetStats() *Statistics {
	return m.stats.GetStats()
}

// Shutdown gracefully shuts down the metrics system, flushing any pending metrics.
func (m *Metrics) Shutdown(ctx context.Context) error {
	return m.stats.Shutdown(ctx)
}

// PrometheusHandler returns the HTTP handler for the /metrics endpoint.
// This handler serves metrics in Prometheus text format
func (m *Metrics) PrometheusHandler() http.Handler {
	return m.stats.PrometheusHandler()
}
