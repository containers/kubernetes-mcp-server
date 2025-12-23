package telemetry

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"k8s.io/klog/v2"
)

// getSamplerFromEnv reads the sampler configuration from environment variables.
// It supports the following OTEL_TRACES_SAMPLER values:
//   - "always_on": Sample all traces
//   - "always_off": Don't sample any traces
//   - "traceidratio": Sample based on trace ID ratio (requires OTEL_TRACES_SAMPLER_ARG)
//   - "parentbased_always_on": Respect parent span sampling decision, default to always_on
//   - "parentbased_traceidratio": Respect parent span sampling decision, default to ratio
//   - "" (empty/not set): Use default ParentBased(AlwaysSample)
func getSamplerFromEnv() trace.Sampler {
	samplerType := os.Getenv("OTEL_TRACES_SAMPLER")
	samplerArg := os.Getenv("OTEL_TRACES_SAMPLER_ARG")

	// Parse sampler argument (ratio) if provided
	ratio := 1.0 // Default to 100% sampling
	if samplerArg != "" {
		parsed, err := strconv.ParseFloat(samplerArg, 64)
		if err != nil {
			klog.V(1).Infof("Invalid OTEL_TRACES_SAMPLER_ARG '%s', using default 1.0: %v", samplerArg, err)
		} else if parsed < 0.0 || parsed > 1.0 {
			klog.V(1).Infof("OTEL_TRACES_SAMPLER_ARG '%f' out of range [0.0, 1.0], using default 1.0", parsed)
		} else {
			ratio = parsed
		}
	}

	// Select sampler based on type
	switch samplerType {
	case "always_on":
		klog.V(2).Info("Using AlwaysSample sampler")
		return trace.AlwaysSample()

	case "always_off":
		klog.V(2).Info("Using NeverSample sampler")
		return trace.NeverSample()

	case "traceidratio":
		klog.V(2).Infof("Using TraceIDRatioBased sampler with ratio %.2f", ratio)
		return trace.TraceIDRatioBased(ratio)

	case "parentbased_always_on":
		klog.V(2).Info("Using ParentBased(AlwaysSample) sampler")
		return trace.ParentBased(trace.AlwaysSample())

	case "parentbased_traceidratio":
		klog.V(2).Infof("Using ParentBased(TraceIDRatioBased(%.2f)) sampler", ratio)
		return trace.ParentBased(trace.TraceIDRatioBased(ratio))

	case "":
		// Default: ParentBased(AlwaysSample) for development
		klog.V(2).Info("Using default ParentBased(AlwaysSample) sampler")
		return trace.ParentBased(trace.AlwaysSample())

	default:
		klog.V(1).Infof("Unknown OTEL_TRACES_SAMPLER '%s', using default ParentBased(AlwaysSample)", samplerType)
		return trace.ParentBased(trace.AlwaysSample())
	}
}

// InitTracer initializes the OpenTelemetry tracer provider
func InitTracer(serviceName, serviceVersion string) (func(), error) {
	ctx := context.Background()

	// Create OTLP exporter
	// Endpoint is configured via OTEL_EXPORTER_OTLP_ENDPOINT env var
	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		klog.V(1).Infof("Failed to create OTLP exporter, tracing disabled: %v", err)
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
		klog.V(1).Infof("Failed to create resource, tracing disabled: %v", err)
		return func() {}, nil
	}

	// Configure tracer provider with sampler from environment
	sampler := getSamplerFromEnv()
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
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

	klog.V(1).Info("OpenTelemetry tracing initialized successfully")

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			klog.Errorf("Failed to shutdown tracer provider: %v", err)
		}
		klog.V(1).Info("OpenTelemetry tracer provider shutdown complete")
	}

	return cleanup, nil
}
