package telemetry

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

// tracingEnabled indicates whether OpenTelemetry tracing is active.
// This is set to true only when OTEL_EXPORTER_OTLP_ENDPOINT is configured
// and the tracer provider is successfully initialized.
var tracingEnabled atomic.Bool

// Enabled returns true if OpenTelemetry tracing is active.
// Middleware should check this before doing any tracing work to avoid
// unnecessary overhead when tracing is disabled.
func Enabled() bool {
	return tracingEnabled.Load()
}

// InitTracerWithConfig initializes the OpenTelemetry tracer provider using the provided config.
// Env overrides are already resolved onto cfg at load time.
// Check telemetry.Enabled() to determine if tracing is active.
func InitTracerWithConfig(ctx context.Context, cfg *config.TelemetryConfig, serviceName, serviceVersion string) (func(), error) {
	logger := klogutil.FromContext(ctx)

	if cfg == nil || !cfg.IsEnabled() {
		logger.V(2).Info("Telemetry not enabled, tracing disabled")
		return func() {}, nil
	}

	exporter, err := createExporterWithConfig(ctx, cfg)
	if err != nil {
		klogutil.LogInfo(logger.V(1), "Failed to create OTLP exporter, tracing disabled", klogutil.Err(err))
		return func() {}, nil
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		logger.V(1).Info("Failed to create resource, tracing disabled")
		return func() {}, nil
	}

	sampler := getSamplerFromConfig(ctx, cfg)

	bsp := trace.NewBatchSpanProcessor(
		exporter,
		trace.WithBatchTimeout(5*time.Second),
		trace.WithMaxQueueSize(2048),
		trace.WithMaxExportBatchSize(512),
	)

	tp := trace.NewTracerProvider(
		trace.WithSpanProcessor(bsp),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracingEnabled.Store(true)
	logger.V(1).Info("OpenTelemetry tracing initialized successfully", "telemetry.exporter.endpoint", cfg.Endpoint.Get())

	cleanup := func() {
		tracingEnabled.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			logger.Error(err, "Failed to shutdown tracer provider")
		}
		logger.V(1).Info("OpenTelemetry tracer provider shutdown complete")
	}

	return cleanup, nil
}

// getSamplerFromConfig reads the sampler configuration from TelemetryConfig.
func getSamplerFromConfig(ctx context.Context, cfg *config.TelemetryConfig) trace.Sampler {
	samplerType := cfg.TracesSampler.Get()
	logger := klogutil.FromContext(ctx)

	ratio := 1.0 // Default to 100% sampling
	if arg := cfg.TracesSamplerArg.Get(); arg != nil {
		parsed := *arg
		if parsed < 0.0 || parsed > 1.0 {
			logger.V(1).Info("traces_sampler_arg out of range [0.0, 1.0], using default",
				"config.key", "traces_sampler_arg",
				"config.provided", parsed,
				"config.default", 1.0,
			)
		} else {
			ratio = parsed
		}
	}

	switch samplerType {
	case "always_on":
		logger.V(2).Info("Using AlwaysSample sampler")
		return trace.AlwaysSample()

	case "always_off":
		logger.V(2).Info("Using NeverSample sampler")
		return trace.NeverSample()

	case "traceidratio":
		logger.V(2).Info("Using TraceIDRatioBased sampler", "telemetry.sampler.ratio", ratio)
		return trace.TraceIDRatioBased(ratio)

	case "parentbased_always_on":
		logger.V(2).Info("Using ParentBased(AlwaysSample) sampler")
		return trace.ParentBased(trace.AlwaysSample())

	case "parentbased_always_off":
		logger.V(2).Info("Using ParentBased(NeverSample) sampler")
		return trace.ParentBased(trace.NeverSample())

	case "parentbased_traceidratio":
		logger.V(2).Info("Using ParentBased(TraceIDRatioBased) sampler", "telemetry.sampler.ratio", ratio)
		return trace.ParentBased(trace.TraceIDRatioBased(ratio))

	case "":
		// Default: ParentBased(AlwaysSample) for development
		logger.V(2).Info("Using default ParentBased(AlwaysSample) sampler")
		return trace.ParentBased(trace.AlwaysSample())

	default:
		logger.V(1).Info("Unknown traces_sampler, using default",
			"config.key", "traces_sampler",
			"config.provided", samplerType,
			"config.default", "ParentBased(AlwaysSample)",
		)
		return trace.ParentBased(trace.AlwaysSample())
	}
}

// createExporterWithConfig creates an OTLP trace exporter using the provided config.
func createExporterWithConfig(ctx context.Context, cfg *config.TelemetryConfig) (*otlptrace.Exporter, error) {
	protocol := strings.ToLower(cfg.Protocol.Get())
	endpoint := cfg.Endpoint.Get()
	logger := klogutil.FromContext(ctx)

	switch protocol {
	case "http/protobuf", "http":
		logger.V(2).Info("Using HTTP/protobuf OTLP exporter", "telemetry.exporter.protocol", protocol)
		return otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))

	case "grpc", "":
		if protocol == "" {
			logger.V(2).Info("Using gRPC OTLP exporter (default)")
		} else {
			logger.V(2).Info("Using gRPC OTLP exporter")
		}
		return otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(endpoint))

	default:
		logger.V(1).Info("Unknown protocol, falling back to default",
			"config.key", "protocol",
			"config.provided", protocol,
			"config.default", "grpc",
		)
		return otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(endpoint))
	}
}
