package telemetry

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/bridges/otellogr"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// NewLogProvider creates an OTLP-backed LoggerProvider that batches and
// exports log records. Returns nil when telemetry is disabled or no
// endpoint is configured. Errors are intended to be non-fatal.
//
// This function does not use klog and is safe to call before the global
// klog logger is wired.
func NewLogProvider(ctx context.Context, cfg *config.TelemetryConfig, serviceName, serviceVersion string) (*sdklog.LoggerProvider, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil
	}

	if strings.ToLower(cfg.LogsExporter.Get()) == "none" {
		return nil, nil
	}

	exporter, err := createLogExporter(ctx, cfg)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, err
	}

	return sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter,
			sdklog.WithExportTimeout(5*time.Second),
		)),
		sdklog.WithResource(res),
	), nil
}

// NewLogSink creates a logr.LogSink that bridges klog output to the given
// OTel LoggerProvider. The provider can be a *sdklog.LoggerProvider for
// production or any log.LoggerProvider implementation (e.g.
// logtest.Recorder) for testing — the bridge treats them identically.
func NewLogSink(serviceName, serviceVersion string, provider log.LoggerProvider) logr.LogSink {
	return otellogr.NewLogSink(serviceName,
		otellogr.WithLoggerProvider(provider),
		otellogr.WithVersion(serviceVersion),
	)
}

// createLogExporter creates an OTLP log exporter using the same endpoint and
// protocol configuration as traces and metrics.
func createLogExporter(ctx context.Context, cfg *config.TelemetryConfig) (sdklog.Exporter, error) {
	protocol := strings.ToLower(cfg.Protocol.Get())
	endpoint := cfg.Endpoint.Get()

	switch protocol {
	case "http/protobuf", "http":
		return otlploghttp.New(ctx, otlploghttp.WithEndpointURL(endpoint))
	default:
		return otlploggrpc.New(ctx, otlploggrpc.WithEndpointURL(endpoint))
	}
}
