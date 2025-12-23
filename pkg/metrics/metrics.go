package metrics

import (
	"context"
	"os"
	"time"

	"k8s.io/klog/v2"
)

// Config contains configuration for the metrics system.
type Config struct {
	// OTelEnabled enables OpenTelemetry tracing collector.
	// enabled when OTEL_EXPORTER_OTLP_ENDPOINT is set.
	OTelEnabled bool

	// TracerName is the name used for the OTel tracer.
	TracerName string
}

// Metrics coordinates multiple metric collectors.
// It implements the Collector interface and fans out calls to all registered collectors.
type Metrics struct {
	collectors []Collector
	stats      *OtelStatsCollector
}

// New creates a new Metrics instance with configured collectors.
func New(config Config) *Metrics {
	m := &Metrics{
		collectors: []Collector{},
	}

	// Stats collector - always enabled for /stats endpoint
	m.stats = NewOtelStatsCollector(config.TracerName)
	m.collectors = append(m.collectors, m.stats)
	klog.V(1).Info("OTel stats collector enabled")

	// OTel collector - optional, enabled via config
	if config.OTelEnabled {
		otelCollector := NewOtelCollector(config.TracerName)
		m.collectors = append(m.collectors, otelCollector)
		klog.V(1).Info("OpenTelemetry collector enabled")
	}

	return m
}

// NewFromEnvironment creates a new Metrics instance with configuration derived from environment variables.
func NewFromEnvironment(tracerName string) *Metrics {
	config := Config{
		OTelEnabled: os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "",
		TracerName:  tracerName,
	}
	return New(config)
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
