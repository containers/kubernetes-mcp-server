// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package instrumentation

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Middleware holds necessary metrics to instrument an http.Server
// and provides necessary behaviors.
type Middleware interface {
	// NewHandler wraps the given HTTP handler for instrumentation.
	NewHandler(handlerName string, handler http.Handler) http.HandlerFunc
}

type nopInstrumentationMiddleware struct{}

func (ins nopInstrumentationMiddleware) NewHandler(handlerName string, handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}
}

// NewNopMiddleware provides a Middleware which does nothing.
func NewNopMiddleware() Middleware {
	return nopInstrumentationMiddleware{}
}

type defaultInstrumentationMiddleware struct {
	metrics *defaultMetrics
}

// NewMiddleware provides default Middleware.
// Passing nil as buckets uses the default buckets.
func NewMiddleware(reg prometheus.Registerer, buckets []float64) Middleware {
	return &defaultInstrumentationMiddleware{
		metrics: newDefaultMetrics(reg, buckets, []string{}),
	}
}

// NewHandler wraps the given HTTP handler for instrumentation. It
// registers four metric collectors (if not already done) and reports HTTP
// metrics to the (newly or already) registered collectors: http_requests_total
// (CounterVec), http_request_duration_seconds (Histogram),
// http_request_size_bytes (Summary), http_response_size_bytes (Summary).
// Each has a constant label named "handler" with the provided handlerName as value.
func (ins *defaultInstrumentationMiddleware) NewHandler(handlerName string, handler http.Handler) http.HandlerFunc {
	baseLabels := prometheus.Labels{"handler": handlerName}
	return httpInstrumentationHandler(baseLabels, ins.metrics, handler)
}

func httpInstrumentationHandler(baseLabels prometheus.Labels, metrics *defaultMetrics, next http.Handler) http.HandlerFunc {
	return promhttp.InstrumentHandlerRequestSize(
		metrics.requestSize.MustCurryWith(baseLabels),
		instrumentHandlerInFlight(
			metrics.inflightHTTPRequests.MustCurryWith(baseLabels),
			promhttp.InstrumentHandlerCounter(
				metrics.requestsTotal.MustCurryWith(baseLabels),
				promhttp.InstrumentHandlerResponseSize(
					metrics.responseSize.MustCurryWith(baseLabels),
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						now := time.Now()

						wd := &responseWriterDelegator{w: w}
						next.ServeHTTP(wd, r)

						requestLabels := prometheus.Labels{"code": wd.Status(), "method": strings.ToLower(r.Method)}
						observer := metrics.requestDuration.MustCurryWith(baseLabels).With(requestLabels)
						requestDuration := time.Since(now).Seconds()

						observer.Observe(requestDuration)
					}),
				),
			),
		),
	)
}

// responseWriterDelegator implements http.ResponseWriter and extracts the statusCode.
// It also implements optional interfaces (Flusher, Hijacker, Pusher) to support
// streaming (SSE), WebSockets, and HTTP/2 push.
type responseWriterDelegator struct {
	w          http.ResponseWriter
	written    bool
	statusCode int
}

func (wd *responseWriterDelegator) Header() http.Header {
	return wd.w.Header()
}

func (wd *responseWriterDelegator) Write(bytes []byte) (int, error) {
	return wd.w.Write(bytes)
}

func (wd *responseWriterDelegator) WriteHeader(statusCode int) {
	wd.written = true
	wd.statusCode = statusCode
	wd.w.WriteHeader(statusCode)
}

func (wd *responseWriterDelegator) StatusCode() int {
	if !wd.written {
		return http.StatusOK
	}
	return wd.statusCode
}

func (wd *responseWriterDelegator) Status() string {
	return fmt.Sprintf("%d", wd.StatusCode())
}

// Flush implements http.Flusher.
// This is required for SSE (Server-Sent Events) streaming to work.
func (wd *responseWriterDelegator) Flush() {
	if f, ok := wd.w.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker.
// This is required for WebSocket connections and other protocols that need
// to take over the underlying connection.
func (wd *responseWriterDelegator) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := wd.w.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("http.Hijacker not implemented by underlying ResponseWriter")
}

// Push implements http.Pusher.
// This is required for HTTP/2 server push support.
func (wd *responseWriterDelegator) Push(target string, opts *http.PushOptions) error {
	if p, ok := wd.w.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return fmt.Errorf("http.Pusher not implemented by underlying ResponseWriter")
}

// instrumentHandlerInFlight is responsible for counting the amount of
// in-flight HTTP requests (requests being processed by the handler) at a given
// moment in time.
// This is used instead of prometheus/client_golang/promhttp.InstrumentHandlerInFlight
// to be able to have the HTTP method as a label.
func instrumentHandlerInFlight(vec *prometheus.GaugeVec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gauge := vec.With(prometheus.Labels{"method": r.Method})
		gauge.Inc()
		defer gauge.Dec()
		next.ServeHTTP(w, r)
	})
}
