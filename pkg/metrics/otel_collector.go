package metrics

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// GenAIOperationExecuteTool is the operation name for tool execution operations
	genAIOperationExecuteTool = "execute_tool"

	// ErrorTypeOther is used for general errors that don't fit other categories
	errorTypeOther = "_OTHER"
)

// OtelCollector wraps OpenTelemetry tracing to implement the Collector interface.
// It creates spans for tool calls and HTTP requests.
type OtelCollector struct {
	tracer trace.Tracer
}

// NewOtelCollector creates a new OtelCollector using the global OTel tracer provider.
func NewOtelCollector(tracerName string) *OtelCollector {
	return &OtelCollector{
		tracer: otel.Tracer(tracerName),
	}
}

// RecordToolCall implements the Collector interface by creating an OTel span for the tool call.
func (o *OtelCollector) RecordToolCall(ctx context.Context, name string, duration time.Duration, err error) {
	// Create span for tool call
	spanName := fmt.Sprintf("tools/call %s", name)
	_, span := o.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("gen_ai.tool.name", name),
			attribute.String("gen_ai.operation.name", genAIOperationExecuteTool),
		),
		// Set start time to retroactively match when tool actually started
		trace.WithTimestamp(time.Now().Add(-duration)),
	)
	defer span.End()

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("error.type", errorTypeOther))
	}
}

// RecordHTTPRequest implements the Collector interface by creating an OTel span for the HTTP request.
func (o *OtelCollector) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	// Create span for HTTP request
	spanName := fmt.Sprintf("%s %s", method, path)
	_, span := o.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.route", path),
			attribute.Int("http.status_code", statusCode),
		),
		// Set start time to retroactively match when request actually started
		trace.WithTimestamp(time.Now().Add(-duration)),
	)
	defer span.End()

	// Set error status for 4xx and 5xx responses
	if statusCode >= 400 {
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", statusCode))
	}
}
