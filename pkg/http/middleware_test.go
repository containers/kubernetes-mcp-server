package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/telemetry"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type HTTPTraceContextPropagationSuite struct {
	suite.Suite
	cleanupTelemetry func()
}

func (s *HTTPTraceContextPropagationSuite) SetupTest() {
	// Enable telemetry for tests by setting the OTLP endpoint
	s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")

	// Initialize telemetry (exporter may fail but tracingEnabled will be set)
	cleanup, _ := telemetry.InitTracer("test", "1.0.0")
	s.cleanupTelemetry = cleanup

	// Set up a global text map propagator for tests
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

func (s *HTTPTraceContextPropagationSuite) TearDownTest() {
	if s.cleanupTelemetry != nil {
		s.cleanupTelemetry()
	}
	// Note: s.T().Setenv automatically restores the original value after the test
}

func (s *HTTPTraceContextPropagationSuite) TestRequestMiddlewareExtractsTraceContext() {
	s.Run("extracts trace context from HTTP headers", func() {
		// Create a test handler that captures the context
		var capturedContext trace.SpanContext
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedContext = trace.SpanContextFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		middleware := RequestMiddleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
		req.Header.Set("tracestate", "rojo=00f067aa0ba902b7")

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		s.True(capturedContext.IsValid(), "Expected valid span context")

		// The middleware creates a new child span, so trace ID should match parent but span ID will be different
		expectedTraceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
		s.Equal(expectedTraceID, capturedContext.TraceID(), "Trace ID should be propagated from parent")

		// Span ID will be different since middleware creates a new span
		parentSpanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
		s.NotEqual(parentSpanID, capturedContext.SpanID(), "Span ID should be new child span, not parent")
	})

	s.Run("handles requests without trace context", func() {
		var capturedContext trace.SpanContext
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedContext = trace.SpanContextFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		middleware := RequestMiddleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		// Middleware creates a new root span when no parent context exists
		s.True(capturedContext.IsValid(), "Expected valid span context from middleware-created span")
	})

	s.Run("skips trace extraction for healthz endpoint", func() {
		var handlerCalled bool
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := RequestMiddleware(handler)

		req := httptest.NewRequest("GET", "/healthz", nil)
		req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		s.True(handlerCalled, "Handler should be called for healthz")
		s.Equal(http.StatusOK, rr.Code)
	})

	s.Run("propagates context through request chain", func() {
		var innerContext trace.SpanContext
		innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			innerContext = trace.SpanContextFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		// Add an intermediate handler
		intermediateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify context is available here too
			spanContext := trace.SpanContextFromContext(r.Context())
			s.True(spanContext.IsValid(), "Context should be valid in intermediate handler")
			innerHandler.ServeHTTP(w, r)
		})

		middleware := RequestMiddleware(intermediateHandler)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		// Verify context was propagated all the way through
		s.True(innerContext.IsValid(), "Context should propagate to inner handler")
		expectedTraceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
		s.Equal(expectedTraceID, innerContext.TraceID())
	})
}

func TestHTTPTraceContextPropagation(t *testing.T) {
	suite.Run(t, new(HTTPTraceContextPropagationSuite))
}
