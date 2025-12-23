package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type HTTPTraceContextPropagationSuite struct {
	suite.Suite
}

func (s *HTTPTraceContextPropagationSuite) SetupTest() {
	// Set up a global text map propagator for tests
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
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

		s.True(capturedContext.IsValid(), "Expected valid span context to be extracted")
		s.True(capturedContext.IsRemote(), "Expected span context to be marked as remote")

		expectedTraceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
		s.Equal(expectedTraceID, capturedContext.TraceID())

		expectedSpanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
		s.Equal(expectedSpanID, capturedContext.SpanID())
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

		// Should not have extracted any trace context
		s.False(capturedContext.IsValid(), "Expected no trace context for request without headers")
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
