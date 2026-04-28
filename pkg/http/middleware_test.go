package http

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/telemetry"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// cfgStateWithTrustProxy returns a *config.StaticConfigState initialized with
// only TrustProxyHeaders set — used by tests that drive RequestMiddleware.
func cfgStateWithTrustProxy(trustProxy bool) *config.StaticConfigState {
	return config.NewStaticConfigState(&config.StaticConfig{TrustProxyHeaders: trustProxy})
}

// cfgStateWithMaxBody returns a *config.StaticConfigState initialized with
// only HTTP.MaxBodyBytes set — used by tests that drive MaxBodyMiddleware.
func cfgStateWithMaxBody(maxBytes int64) *config.StaticConfigState {
	return config.NewStaticConfigState(&config.StaticConfig{HTTP: config.HTTPConfig{MaxBodyBytes: maxBytes}})
}

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

		middleware := RequestMiddleware(cfgStateWithTrustProxy(false))(handler)

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

		middleware := RequestMiddleware(cfgStateWithTrustProxy(false))(handler)

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

		middleware := RequestMiddleware(cfgStateWithTrustProxy(false))(handler)

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

		middleware := RequestMiddleware(cfgStateWithTrustProxy(false))(intermediateHandler)

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

type MaxBodyMiddlewareSuite struct {
	suite.Suite
}

func (s *MaxBodyMiddlewareSuite) TestMaxBodyMiddleware() {
	s.Run("allows requests under limit", func() {
		handler := MaxBodyMiddleware(cfgStateWithMaxBody(100))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			s.Require().NoError(err)
			s.Equal("small body", string(body))
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("small body"))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		s.Equal(http.StatusOK, rr.Code)
	})

	s.Run("rejects requests exceeding limit", func() {
		handlerCalled := false
		handler := MaxBodyMiddleware(cfgStateWithMaxBody(10))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			// Attempt to read the body - this should fail
			_, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))

		largeBody := strings.Repeat("x", 100)
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(largeBody))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		s.True(handlerCalled, "handler should be called")
		s.Equal(http.StatusRequestEntityTooLarge, rr.Code)
	})

	s.Run("skips GET requests", func() {
		handlerCalled := false
		handler := MaxBodyMiddleware(cfgStateWithMaxBody(10))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		s.True(handlerCalled)
		s.Equal(http.StatusOK, rr.Code)
	})

	s.Run("skips HEAD requests", func() {
		handlerCalled := false
		handler := MaxBodyMiddleware(cfgStateWithMaxBody(10))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodHead, "/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		s.True(handlerCalled)
		s.Equal(http.StatusOK, rr.Code)
	})

	s.Run("skips OPTIONS requests", func() {
		handlerCalled := false
		handler := MaxBodyMiddleware(cfgStateWithMaxBody(10))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		s.True(handlerCalled)
		s.Equal(http.StatusOK, rr.Code)
	})

	s.Run("skips when maxBytes is zero", func() {
		handler := MaxBodyMiddleware(cfgStateWithMaxBody(0))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			s.Require().NoError(err)
			s.Equal("large body that would exceed any limit", string(body))
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("large body that would exceed any limit"))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		s.Equal(http.StatusOK, rr.Code)
	})

	s.Run("applies to PUT requests", func() {
		handler := MaxBodyMiddleware(cfgStateWithMaxBody(10))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))

		largeBody := strings.Repeat("x", 100)
		req := httptest.NewRequest(http.MethodPut, "/test", strings.NewReader(largeBody))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		s.Equal(http.StatusRequestEntityTooLarge, rr.Code)
	})

	s.Run("applies to PATCH requests", func() {
		handler := MaxBodyMiddleware(cfgStateWithMaxBody(10))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))

		largeBody := strings.Repeat("x", 100)
		req := httptest.NewRequest(http.MethodPatch, "/test", strings.NewReader(largeBody))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		s.Equal(http.StatusRequestEntityTooLarge, rr.Code)
	})

	// Regression for issue #1106: changes stored in cfgState must be observed
	// on the NEXT request without rebuilding the middleware.
	s.Run("picks up max_body_bytes change via cfgState.Store", func() {
		cfgState := config.NewStaticConfigState(&config.StaticConfig{HTTP: config.HTTPConfig{MaxBodyBytes: 0}})

		handler := MaxBodyMiddleware(cfgState)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(strings.Repeat("x", 100)))
		rr1 := httptest.NewRecorder()
		handler.ServeHTTP(rr1, req1)
		s.Equal(http.StatusOK, rr1.Code, "pre-reload: max_body_bytes=0 must allow any body size")

		cfgState.Store(&config.StaticConfig{HTTP: config.HTTPConfig{MaxBodyBytes: 10}})

		req2 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(strings.Repeat("x", 100)))
		rr2 := httptest.NewRecorder()
		handler.ServeHTTP(rr2, req2)
		s.Equal(http.StatusRequestEntityTooLarge, rr2.Code,
			"post-reload: max_body_bytes=10 must reject oversized body")
	})
}

func TestMaxBodyMiddleware(t *testing.T) {
	suite.Run(t, new(MaxBodyMiddlewareSuite))
}

// TrustProxyHeadersSuite verifies that RequestMiddleware only honors
// X-Forwarded-* and X-Real-IP headers when trust_proxy_headers is enabled.
// Assertions read url.scheme and client.address from OpenTelemetry span
// attributes captured by an in-memory recorder bound to httpTracer for the
// lifetime of each test.
type TrustProxyHeadersSuite struct {
	suite.Suite
	spanRecorder     *tracetest.SpanRecorder
	origHTTPTracer   trace.Tracer
	cleanupTelemetry func()
}

func (s *TrustProxyHeadersSuite) SetupTest() {
	// Bind httpTracer directly to a fresh in-memory recorder. Swapping the
	// package-level tracer sidesteps OTel's one-shot delegate locking
	// (sync.Once on the global proxy tracer), so this suite doesn't need a
	// TestMain and doesn't interfere with other suites in this package.
	s.spanRecorder = tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(s.spanRecorder),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	s.origHTTPTracer = httpTracer
	httpTracer = tp.Tracer("test")

	// RequestMiddleware skips span creation when telemetry.Enabled() is false,
	// so flip the flag on by initializing the tracer with an OTLP endpoint.
	s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
	cleanup, err := telemetry.InitTracer("test", "1.0.0")
	s.Require().NoError(err, "Expected telemetry.InitTracer to succeed")
	s.cleanupTelemetry = cleanup
}

func (s *TrustProxyHeadersSuite) TearDownTest() {
	httpTracer = s.origHTTPTracer
	if s.cleanupTelemetry != nil {
		s.cleanupTelemetry()
	}
}

// runRequest drives a request through RequestMiddleware(trustProxy) and
// returns the attributes of the span that was ended by the middleware.
// The recorder is reset before each invocation so this helper is safe to call
// from nested s.Run subtests (SetupTest only runs per top-level test method).
func (s *TrustProxyHeadersSuite) runRequest(trustProxy bool, mutate func(*http.Request)) map[string]attribute.Value {
	s.T().Helper()
	s.spanRecorder.Reset()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RequestMiddleware(cfgStateWithTrustProxy(trustProxy))(handler)

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.RemoteAddr = "192.168.1.1:443"
	if mutate != nil {
		mutate(req)
	}

	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	ended := s.spanRecorder.Ended()
	s.Require().Len(ended, 1, "expected exactly one span to be recorded")
	attrs := make(map[string]attribute.Value, len(ended[0].Attributes()))
	for _, kv := range ended[0].Attributes() {
		attrs[string(kv.Key)] = kv.Value
	}
	return attrs
}

func (s *TrustProxyHeadersSuite) TestClientAddress() {
	s.Run("trust_proxy=true", func() {
		s.Run("uses first X-Forwarded-For IP", func() {
			attrs := s.runRequest(true, func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "10.0.0.1")
			})
			s.Equal("10.0.0.1", attrs["client.address"].AsString())
		})
		s.Run("takes first entry from comma-separated X-Forwarded-For", func() {
			attrs := s.runRequest(true, func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2, 10.0.0.3")
			})
			s.Equal("10.0.0.1", attrs["client.address"].AsString())
		})
		s.Run("trims whitespace around X-Forwarded-For entry", func() {
			attrs := s.runRequest(true, func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", " 10.0.0.1 ")
			})
			s.Equal("10.0.0.1", attrs["client.address"].AsString())
		})
		s.Run("falls back to X-Real-IP when X-Forwarded-For absent", func() {
			attrs := s.runRequest(true, func(r *http.Request) {
				r.Header.Set("X-Real-IP", "10.0.0.2")
			})
			s.Equal("10.0.0.2", attrs["client.address"].AsString())
		})
		s.Run("X-Forwarded-For wins over X-Real-IP when both present", func() {
			attrs := s.runRequest(true, func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "10.0.0.1")
				r.Header.Set("X-Real-IP", "10.0.0.2")
			})
			s.Equal("10.0.0.1", attrs["client.address"].AsString())
		})
	})

	s.Run("trust_proxy=false", func() {
		s.Run("ignores X-Forwarded-For and X-Real-IP", func() {
			attrs := s.runRequest(false, func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "10.0.0.1")
				r.Header.Set("X-Real-IP", "10.0.0.2")
			})
			s.Equal("192.168.1.1", attrs["client.address"].AsString(),
				"proxy headers must be ignored when trust_proxy_headers is disabled")
		})
		s.Run("uses RemoteAddr host when no proxy headers set", func() {
			attrs := s.runRequest(false, nil)
			s.Equal("192.168.1.1", attrs["client.address"].AsString())
		})
		s.Run("falls back to RemoteAddr when SplitHostPort fails", func() {
			attrs := s.runRequest(false, func(r *http.Request) {
				r.RemoteAddr = "bad-remote-addr"
			})
			s.Equal("bad-remote-addr", attrs["client.address"].AsString())
		})
	})
}

func (s *TrustProxyHeadersSuite) TestURLScheme() {
	s.Run("trust_proxy=true", func() {
		s.Run("honors X-Forwarded-Proto=https", func() {
			attrs := s.runRequest(true, func(r *http.Request) {
				r.Header.Set("X-Forwarded-Proto", "https")
			})
			s.Equal("https", attrs["url.scheme"].AsString())
		})
		s.Run("defaults to http when X-Forwarded-Proto absent and no TLS", func() {
			attrs := s.runRequest(true, nil)
			s.Equal("http", attrs["url.scheme"].AsString())
		})
		s.Run("returns https when request has TLS", func() {
			attrs := s.runRequest(true, func(r *http.Request) {
				r.TLS = &tls.ConnectionState{}
			})
			s.Equal("https", attrs["url.scheme"].AsString())
		})
	})

	s.Run("trust_proxy=false", func() {
		s.Run("ignores X-Forwarded-Proto", func() {
			attrs := s.runRequest(false, func(r *http.Request) {
				r.Header.Set("X-Forwarded-Proto", "https")
			})
			s.Equal("http", attrs["url.scheme"].AsString(),
				"X-Forwarded-Proto must be ignored when trust_proxy_headers is disabled")
		})
		s.Run("returns https when request has TLS", func() {
			attrs := s.runRequest(false, func(r *http.Request) {
				r.TLS = &tls.ConnectionState{}
			})
			s.Equal("https", attrs["url.scheme"].AsString())
		})
	})
}

func TestTrustProxyHeaders(t *testing.T) {
	suite.Run(t, new(TrustProxyHeadersSuite))
}

// TestReloadObserved verifies the middleware observes config changes on the
// NEXT request after cfgState.Store — no wiring rebuild required. This is
// the regression lock for issue #1106: RequestMiddleware and MaxBodyMiddleware
// must read from *StaticConfigState per request so SIGHUP-reloaded values
// (trust_proxy_headers, max_body_bytes) take effect without a restart.
func (s *TrustProxyHeadersSuite) TestReloadObserved() {
	s.Run("RequestMiddleware picks up trust_proxy_headers flip via cfgState.Store", func() {
		s.spanRecorder.Reset()
		cfgState := config.NewStaticConfigState(&config.StaticConfig{TrustProxyHeaders: false})

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		middleware := RequestMiddleware(cfgState)(handler)

		// First request — trust_proxy=false: X-Forwarded-For must be ignored.
		req1 := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		req1.RemoteAddr = "192.168.1.1:443"
		req1.Header.Set("X-Forwarded-For", "10.0.0.1")
		middleware.ServeHTTP(httptest.NewRecorder(), req1)

		// Flip config — simulates a SIGHUP reload.
		cfgState.Store(&config.StaticConfig{TrustProxyHeaders: true})

		// Second request — trust_proxy=true: X-Forwarded-For must now be honored.
		req2 := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		req2.RemoteAddr = "192.168.1.1:443"
		req2.Header.Set("X-Forwarded-For", "10.0.0.1")
		middleware.ServeHTTP(httptest.NewRecorder(), req2)

		ended := s.spanRecorder.Ended()
		s.Require().Len(ended, 2, "expected two spans")

		firstAttrs := make(map[string]attribute.Value, len(ended[0].Attributes()))
		for _, kv := range ended[0].Attributes() {
			firstAttrs[string(kv.Key)] = kv.Value
		}
		secondAttrs := make(map[string]attribute.Value, len(ended[1].Attributes()))
		for _, kv := range ended[1].Attributes() {
			secondAttrs[string(kv.Key)] = kv.Value
		}

		s.Equal("192.168.1.1", firstAttrs["client.address"].AsString(),
			"pre-reload request must ignore X-Forwarded-For (trust_proxy_headers=false)")
		s.Equal("10.0.0.1", secondAttrs["client.address"].AsString(),
			"post-reload request must honor X-Forwarded-For (trust_proxy_headers=true)")
	})
}
