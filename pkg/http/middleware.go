package http

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

// httpTracer is the tracer used for HTTP request spans
var httpTracer = otel.Tracer("kubernetes-mcp-server/http")

// Middleware decorates an http.Handler. It is the shape returned by
// RequestMiddleware, AuthorizationMiddleware, and MaxBodyMiddleware so they
// can be composed via chain.
type Middleware func(http.Handler) http.Handler

// chain composes middlewares into a single handler, applied in the order
// listed: the first middleware is the outermost (runs first on inbound,
// last on outbound). Reads top-down like the request flow.
func chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// getClientIP extracts the client IP address from the request.
// When trustProxy is true, it checks X-Forwarded-For and X-Real-IP headers first
// (for proxied requests), then falls back to RemoteAddr.
func getClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		// Check X-Forwarded-For header (may contain comma-separated list)
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Take the first IP in the list (original client)
			if idx := strings.Index(xff, ","); idx != -1 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}

		// Check X-Real-IP header
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// getHTTPRoute returns a normalized route for the request path.
// This helps reduce cardinality in traces by grouping similar paths.
func getHTTPRoute(path string) string {
	// Known routes for this server
	switch path {
	case "/healthz", "/mcp", "/sse", "/message", "/stats":
		return path
	}
	// Check for well-known prefix
	if strings.HasPrefix(path, "/.well-known/") {
		return "/.well-known/*"
	}
	return path
}

// RequestMiddleware creates OpenTelemetry spans for HTTP requests.
// The trust_proxy_headers config flag is read per request from cfgState so
// SIGHUP-reloaded values take effect immediately. When enabled, X-Forwarded-*
// and X-Real-IP headers are used for client IP and scheme detection. Only
// enable when behind a trusted reverse proxy.
func RequestMiddleware(cfgState *config.StaticConfigState) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip tracing for health checks
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			trustProxy := cfgState.Load().TrustProxyHeaders

			// Skip all tracing work if telemetry is not enabled
			if !telemetry.Enabled() {
				lrw := &loggingResponseWriter{
					ResponseWriter: w,
					statusCode:     http.StatusOK,
				}
				start := time.Now()
				next.ServeHTTP(lrw, r)
				klog.V(5).Infof("%s %s %d %v", r.Method, r.URL.Path, lrw.statusCode, time.Since(start))
				return
			}

			// Extract trace context from HTTP headers using OpenTelemetry propagator
			// This enables distributed tracing for HTTP requests
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Determine HTTP route for span naming
			route := getHTTPRoute(r.URL.Path)
			spanName := fmt.Sprintf("%s %s", r.Method, route)

			// Build attributes following OpenTelemetry HTTP semantic conventions
			attrs := []attribute.KeyValue{
				attribute.String("http.request.method", r.Method),
				attribute.String("url.path", r.URL.Path),
				attribute.String("url.scheme", getScheme(r, trustProxy)),
				attribute.String("server.address", r.Host),
				attribute.String("network.protocol.name", "http"),
				attribute.String("network.protocol.version", r.Proto),
				attribute.String("client.address", getClientIP(r, trustProxy)),
			}

			if route != r.URL.Path {
				attrs = append(attrs, attribute.String("http.route", route))
			}

			// Note: url.query is intentionally not included as it may contain sensitive data

			if ua := r.UserAgent(); ua != "" {
				attrs = append(attrs, attribute.String("user_agent.original", ua))
			}

			if r.ContentLength > 0 {
				attrs = append(attrs, attribute.Int64("http.request.body.size", r.ContentLength))
			}

			ctx, span := httpTracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attrs...),
			)
			defer span.End()

			r = r.WithContext(ctx)

			start := time.Now()

			lrw := &loggingResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(lrw, r)

			duration := time.Since(start)

			// Add response attributes to span
			span.SetAttributes(
				attribute.Int("http.response.status_code", lrw.statusCode),
			)

			// Set span status and error type based on response code
			if lrw.statusCode >= 500 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", lrw.statusCode))
				span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%d", lrw.statusCode)))
			} else if lrw.statusCode >= 400 {
				// 4xx errors are client errors, not server errors
				span.SetStatus(codes.Unset, "")
				span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%d", lrw.statusCode)))
			} else {
				span.SetStatus(codes.Ok, "")
			}

			klog.V(5).Infof("%s %s %d %v", r.Method, r.URL.Path, lrw.statusCode, duration)
		})
	}
}

// getScheme returns the URL scheme (http or https) for the request.
// When trustProxy is true, it checks the X-Forwarded-Proto header first.
func getScheme(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			return proto
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	if !lrw.headerWritten {
		lrw.statusCode = code
		lrw.headerWritten = true
		lrw.ResponseWriter.WriteHeader(code)
	}
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	if !lrw.headerWritten {
		lrw.statusCode = http.StatusOK
		lrw.headerWritten = true
	}
	return lrw.ResponseWriter.Write(b)
}

func (lrw *loggingResponseWriter) Flush() {
	if flusher, ok := lrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (lrw *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := lrw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// MaxBodyMiddleware limits the size of incoming request bodies.
// It wraps the request body with http.MaxBytesReader to enforce the limit.
// Requests exceeding the limit receive a 413 Request Entity Too Large response.
// The max_body_bytes limit is read per request from cfgState so SIGHUP-reloaded
// values take effect immediately.
func MaxBodyMiddleware(cfgState *config.StaticConfigState) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip for methods that typically don't have bodies
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			maxBytes := cfgState.Load().HTTP.MaxBodyBytes
			// Skip if maxBytes is 0 or negative (disabled)
			if maxBytes <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
