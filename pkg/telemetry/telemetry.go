package telemetry

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"k8s.io/klog/v2"
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

// getSamplerFromEnv reads the sampler configuration from environment variables.
// It supports the following OTEL_TRACES_SAMPLER values:
//   - "always_on": Sample all traces
//   - "always_off": Don't sample any traces
//   - "traceidratio": Sample based on trace ID ratio (requires OTEL_TRACES_SAMPLER_ARG)
//   - "parentbased_always_on": Respect parent span sampling decision, default to always_on
//   - "parentbased_traceidratio": Respect parent span sampling decision, default to ratio
//   - "" (empty/not set): Use default ParentBased(AlwaysSample)
func getSamplerFromEnv(ctx context.Context) trace.Sampler {
	samplerType := os.Getenv("OTEL_TRACES_SAMPLER")
	samplerArg := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	logger := klog.FromContext(ctx)

	// Parse sampler argument (ratio) if provided
	ratio := 1.0 // Default to 100% sampling
	if samplerArg != "" {
		parsed, err := strconv.ParseFloat(samplerArg, 64)
		if err != nil {
			logger.V(1).Info("Invalid OTEL_TRACES_SAMPLER_ARG, falling back to default",
				"config.env_var", "OTEL_TRACES_SAMPLER_ARG",
				"config.provided_value", samplerArg,
				"config.default_value", 1.0,
				"exception.message", err.Error(),
			)
		} else if parsed < 0.0 || parsed > 1.0 {
			logger.V(1).Info("OTEL_TRACES_SAMPLER_ARG out of range [0.0, 1.0], falling back to default",
				"config.env_var", "OTEL_TRACES_SAMPLER_ARG",
				"config.provided_value", parsed,
				"config.default_value", 1.0,
			)
		} else {
			ratio = parsed
		}
	}

	// Select sampler based on type
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
		logger.V(1).Info("Unknown OTEL_TRACES_SAMPLER, using default",
			"config.env_var", "OTEL_TRACES_SAMPLER",
			"config.provided", samplerType,
			"config.default", "ParentBased(AlwaysSample)",
		)
		return trace.ParentBased(trace.AlwaysSample())
	}
}

// createExporter creates an OTLP trace exporter based on the OTEL_EXPORTER_OTLP_PROTOCOL env var.
// Supported protocols:
//   - "grpc": Use gRPC protocol (default)
//   - "http/protobuf": Use HTTP with protobuf encoding
//   - "http": Alias for "http/protobuf"
func createExporter(ctx context.Context) (*otlptrace.Exporter, error) {
	protocol := strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"))
	logger := klog.FromContext(ctx)

	switch protocol {
	case "http/protobuf", "http":
		logger.V(2).Info("Using HTTP/protobuf OTLP exporter", "telemetry.exporter.protocol", protocol)
		return otlptracehttp.New(ctx)

	case "grpc", "":
		// Default to gRPC
		if protocol == "" {
			logger.V(2).Info("Using gRPC OTLP exporter (default)")
		} else {
			logger.V(2).Info("Using gRPC OTLP exporter")
		}
		return otlptracegrpc.New(ctx)

	default:
		logger.V(1).Info("Unknown OTEL_EXPORTER_OTLP_PROTOCOL falling back to default",
			"config.env_var", "OTEL_EXPORTER_OTLP_PROTOCOL",
			"config.provided", protocol,
			"config.default", "grpc",
		)
		return otlptracegrpc.New(ctx)
	}
}

// InitTracer initializes the OpenTelemetry tracer provider.
// Tracing is only enabled if OTEL_EXPORTER_OTLP_ENDPOINT is set.
// Check telemetry.Enabled() to determine if tracing is active.
func InitTracer(ctx context.Context, serviceName, serviceVersion string) (func(), error) {
	logger := klog.FromContext(ctx)
	// Check if OTLP endpoint is configured - if not, skip all tracing setup
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		logger.V(2).Info("OTEL_EXPORTER_OTLP_ENDPOINT not set, tracing disabled")
		return func() {}, nil
	}

	// Create OTLP exporter based on protocol configuration
	// Endpoint is configured via OTEL_EXPORTER_OTLP_ENDPOINT env var
	// Protocol is configured via OTEL_EXPORTER_OTLP_PROTOCOL env var
	exporter, err := createExporter(ctx)
	if err != nil {
		logger.V(1).Info("Failed to create OTLP exporter, tracing disabled", "exception.message", err.Error())
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
		logger.V(1).Info("Failed to create resource, tracing disabled", "exception.message", err.Error())
		return func() {}, nil
	}

	// Configure tracer provider with sampler from environment
	sampler := getSamplerFromEnv(ctx)

	// Create batch span processor with production settings
	// - BatchTimeout: Maximum time to wait before exporting a batch (5 seconds)
	// - MaxQueueSize: Maximum number of spans queued before dropping (2048)
	// - MaxExportBatchSize: Maximum spans per export batch (512)
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

	// Set up text map propagator for distributed tracing context propagation
	// This enables trace context to be extracted from and injected into carriers (e.g., HTTP headers, MCP metadata)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, // W3C Trace Context propagator
		propagation.Baggage{},      // W3C Baggage propagator
	))

	// Mark tracing as enabled so middleware knows to create spans
	tracingEnabled.Store(true)
	logger.V(1).Info("OpenTelemetry tracing initialized successfully")

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

// InitTracerWithConfig initializes the OpenTelemetry tracer provider using the provided config.
// The config values can be overridden by environment variables.
// Check telemetry.Enabled() to determine if tracing is active.
func InitTracerWithConfig(ctx context.Context, cfg *config.TelemetryConfig, serviceName, serviceVersion string) (func(), error) {
	logger := klog.FromContext(ctx)

	if cfg == nil || !cfg.IsEnabled() {
		logger.V(2).Info("Telemetry not enabled, tracing disabled")
		return func() {}, nil
	}

	exporter, err := createExporterWithConfig(ctx, cfg)
	if err != nil {
		logger.V(1).Info("Failed to create OTLP exporter, tracing disabled", "exception.message", err.Error())
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
	logger.V(1).Info("OpenTelemetry tracing initialized successfully", "telemetry.exporter.endpoint", cfg.GetEndpoint())

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
// Environment variables take precedence over config values.
func getSamplerFromConfig(ctx context.Context, cfg *config.TelemetryConfig) trace.Sampler {
	samplerType := cfg.GetTracesSampler()
	samplerArg := cfg.GetTracesSamplerArg()
	logger := klog.FromContext(ctx)

	ratio := 1.0 // Default to 100% sampling
	if samplerArg != "" {
		parsed, err := strconv.ParseFloat(samplerArg, 64)
		if err != nil {
			logger.V(1).Info("Invalid traces_sampler_arg, using",
				"config.key", "traces_sampler_arg",
				"config.provided", samplerArg,
				"config.default", 1.0,
				"exception.message", err.Error(),
			)
		} else if parsed < 0.0 || parsed > 1.0 {
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
// Environment variables take precedence over config values.
func createExporterWithConfig(ctx context.Context, cfg *config.TelemetryConfig) (*otlptrace.Exporter, error) {
	protocol := strings.ToLower(cfg.GetProtocol())
	endpoint := cfg.GetEndpoint()
	logger := klog.FromContext(ctx)

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
